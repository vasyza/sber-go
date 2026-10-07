// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

var domainMoscow = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		panic("sber: Moscow timezone unavailable")
	}
	return loc
}()

// This is the canonical _strptime.TimeRE grammar for %d.%m.%YT%H:%M:%S,
// not the ASCII ISO grammar. Its Nd alternatives are deliberately asymmetric:
// month and many leading digits stay ASCII; T is case-insensitive. Datetime
// validation below still rejects leap seconds admitted by strptime's regex.
var domainBankDate = regexp.MustCompile(`(?i)^(3[0-1]|[1-2]\p{Nd}|0[1-9]|[1-9]| [1-9])\.(1[0-2]|0[1-9]|[1-9])\.(\p{Nd}{4})T(2[0-3]|[0-1]\p{Nd}|\p{Nd}):([0-5]\p{Nd}|\p{Nd}):(6[0-1]|[0-5]\p{Nd}|\p{Nd})$`)

// The pinned canonical Python3.12.3 uses Unicode15. Go1.27.1 uses Unicode17.
// These decimal-zero ranges were generated from the actual canonical runtime
// (bank-fixture-runtime.json); newer Go Nd digits must not widen source grammar.
// Each immutable table entry begins ten consecutive Nd decimal digits.
var domainBankDigitZeros = [...]rune{
	48, 1632, 1776, 1984, 2406, 2534, 2662, 2790, 2918, 3046,
	3174, 3302, 3430, 3558, 3664, 3792, 3872, 4160, 4240, 6112,
	6160, 6470, 6608, 6784, 6800, 6992, 7088, 7232, 7248, 42528,
	43216, 43264, 43472, 43504, 43600, 44016, 65296, 66720, 68912, 69734,
	69872, 69942, 70096, 70384, 70736, 70864, 71248, 71360, 71472, 71904,
	72016, 72784, 73040, 73120, 73552, 92768, 92864, 93008, 120782, 120792,
	120802, 120812, 120822, 123200, 123632, 124144, 125264, 130032,
}

func domainBankInt(value string) (int, bool) {
	value = strings.TrimPrefix(value, " ") // only the %d grammar permits it
	n := 0
	for _, r := range value {
		i := sort.Search(len(domainBankDigitZeros), func(i int) bool { return domainBankDigitZeros[i]+9 >= r })
		if i == len(domainBankDigitZeros) || r < domainBankDigitZeros[i] {
			return 0, false
		}
		n = n*10 + int(r-domainBankDigitZeros[i])
	}
	return n, true
}

func domainOperationDate(value any) string {
	if !domainTruthy(value) {
		return ""
	}
	raw := domainID(value)
	parts := domainBankDate.FindStringSubmatch(raw)
	if parts == nil {
		return raw
	}
	numbers := make([]int, 6)
	for i := range numbers {
		var valid bool
		numbers[i], valid = domainBankInt(parts[i+1])
		if !valid {
			return raw
		}
	}
	day, month, year, hour, minute, second := numbers[0], numbers[1], numbers[2], numbers[3], numbers[4], numbers[5]
	if year < 1 || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59 {
		return raw
	}
	wall := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	if wall.Year() != year || int(wall.Month()) != month || wall.Day() != day {
		return raw
	}
	moment, err := domainAttachMoscow(wall)
	if err != nil {
		return raw
	}
	return domainISOFormat(moment)
}

func domainISOFormat(moment time.Time) string {
	_, offset := moment.Zone()
	if offset%60 != 0 {
		return moment.Format("2006-01-02T15:04:05Z07:00:00")
	}
	return moment.Format(time.RFC3339)
}

// Native layouts do not define canonical source acceptance. Keep ISO date
// boundary selection / width validation separate from clock and offset scans.
var domainISOWeekDate = regexp.MustCompile(`^([0-9]{4})(?:-W([0-9]{2})(?:-([1-7]))?|W([0-9]{2})([1-7])?)$`)

func domainASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

// Resolve the source's basic/extended ISO week date boundary, including its
// documented numeric-separator ambiguity. Positions before the separator are
// ASCII; the separator itself is handled separately.
func domainISODateWidth(value string) int {
	if len(value) < 7 {
		return 0
	}
	if len(value) == 7 {
		return 7
	}
	if value[4] == '-' {
		if value[5] != 'W' {
			return 10
		}
		if len(value) > 8 && value[8] == '-' {
			if len(value) == 9 {
				return 0
			}
			if len(value) > 10 && domainASCIIDigit(value[10]) {
				return 8
			}
			return 10
		}
		return 8
	}
	if value[4] != 'W' {
		return 8
	}
	i := 7
	for i < len(value) && domainASCIIDigit(value[i]) {
		i++
	}
	if i < 9 {
		return i
	}
	if i%2 == 0 {
		return 7
	}
	return 8
}

func domainCanonicalWeekISO(value string) string {
	width := domainISODateWidth(value)
	if width == 0 || width > len(value) {
		return value
	}
	parts := domainISOWeekDate.FindStringSubmatch(value[:width])
	if parts == nil {
		return value
	}
	year, _ := strconv.Atoi(parts[1])
	weekText, dayText := parts[2], parts[3]
	if weekText == "" {
		weekText, dayText = parts[4], parts[5]
	}
	week, _ := strconv.Atoi(weekText)
	day := 1
	if dayText != "" {
		day, _ = strconv.Atoi(dayText)
	}
	if year < 1 || year > 9999 || week < 1 || week > 53 {
		return value
	}
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	weekday := (int(jan4.Weekday()) + 6) % 7
	wall := jan4.AddDate(0, 0, (week-1)*7+day-1-weekday)
	isoYear, isoWeek := wall.ISOWeek()
	if isoYear != year || isoWeek != week || wall.Year() < 1 || wall.Year() > 9999 {
		return value
	}
	return wall.Format("2006-01-02") + value[width:]
}

// Canonicalize only the validated ASCII calendar prefix and one codepoint
// separator; retain original compact/extended clocks for the source scanner.
func domainCanonicalBasicISO(value string) string {
	value = domainCanonicalWeekISO(value)
	if _, basic := domainISOInt(value, 0, 8); basic {
		value = value[:4] + "-" + value[4:6] + "-" + value[6:]
	}
	if len(value) > 10 {
		_, width := utf8.DecodeRuneInString(value[10:])
		value = value[:10] + "T" + value[10+width:]
	}
	return value
}

// Read fixed-width ASCII fields without native parser width permissiveness.
func domainISOInt(value string, start, width int) (int, bool) {
	if start < 0 || width < 0 || start+width > len(value) {
		return 0, false
	}
	v := 0
	for _, c := range []byte(value[start : start+width]) {
		if !domainASCIIDigit(c) {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	return v, true
}

// ParseSourceDate implements the independent canonical Python3.12 DATE
// scanner. Its entry point accepts UTF-8 byte lengths 7, 8 or 10; its basic
// calendar/week scanner does not require consuming residual bytes. In
// particular a valid DATE and DATETIME can denote different days. The result
// is a UTC-labeled civil date, not a UTC instant or a Moscow attachment.
func ParseSourceDate(value string) (time.Time, error) {
	invalid := func() (time.Time, error) { return time.Time{}, NewParseError("date") }
	if !utf8.ValidString(value) || (len(value) != 7 && len(value) != 8 && len(value) != 10) {
		return invalid()
	}
	year, ok := domainISOInt(value, 0, 4)
	if !ok || year < 1 || year > 9999 {
		return invalid()
	}
	pos := 4
	separated := domainISOByte(value, pos) == '-'
	if separated {
		pos++
	}
	if domainISOByte(value, pos) == 'W' {
		week, valid := domainISOInt(value, pos+1, 2)
		if !valid || week < 1 || week > 53 {
			return invalid()
		}
		pos += 3
		day := 1
		if pos < len(value) {
			if separated {
				if domainISOByte(value, pos) != '-' {
					return invalid()
				}
				pos++
			}
			day, valid = domainISOInt(value, pos, 1)
			if !valid || day < 1 || day > 7 {
				return invalid()
			}
		}
		jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
		weekday := (int(jan4.Weekday()) + 6) % 7
		wall := jan4.AddDate(0, 0, (week-1)*7+day-1-weekday)
		isoYear, isoWeek := wall.ISOWeek()
		if isoYear != year || isoWeek != week || wall.Year() < 1 || wall.Year() > 9999 {
			return invalid()
		}
		return wall, nil
	}
	month, ok := domainISOInt(value, pos, 2)
	if !ok || month < 1 || month > 12 {
		return invalid()
	}
	pos += 2
	if separated {
		if domainISOByte(value, pos) != '-' {
			return invalid()
		}
		pos++
	}
	day, ok := domainISOInt(value, pos, 2)
	if !ok || day < 1 || day > 31 {
		return invalid()
	}
	wall := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if wall.Year() != year || int(wall.Month()) != month || wall.Day() != day {
		return invalid()
	}
	return wall, nil
}

func domainISOByte(value string, pos int) byte {
	if pos < 0 || pos >= len(value) {
		return 0
	}
	return value[pos]
}

// Mirror the canonical 3.12 C clock scanner's return semantics, not a generic
// ISO standard or the newer Python fallback. An aware clock may have a dangling
// decimal marker, or residual text after six fractional digits; an offset or
// naive clock must finish. Fractions are fractional SECONDS even on HH/HHMM.
func domainISOClock(value string, end int) (fields [4]int, residual, ok bool) {
	pos, colon := 0, false
	for i := 0; i < 3; i++ {
		if pos+2 > end {
			return fields, false, false
		}
		n, valid := domainISOInt(value, pos, 2)
		if !valid {
			return fields, false, false
		}
		fields[i] = n
		pos += 2
		c := domainISOByte(value, pos)
		pos++
		if i == 0 {
			colon = c == ':'
		}
		if pos >= end {
			return fields, c != 0, true
		}
		if colon && c == ':' {
			continue
		}
		if c == '.' || c == ',' {
			break
		}
		if !colon {
			pos--
		} else {
			return fields, false, false
		}
	}
	count := end - pos
	if count < 0 {
		return fields, false, false
	}
	if count > 6 {
		count = 6
	}
	fraction, valid := domainISOInt(value, pos, count)
	if !valid {
		return fields, false, false
	}
	pos += count
	for i := count; i < 6; i++ {
		fraction *= 10
	}
	fields[3] = fraction
	for domainASCIIDigit(domainISOByte(value, pos)) {
		pos++
	}
	return fields, domainISOByte(value, pos) != 0, true
}

// SourceDateTime retains exact source civil fields and microsecond UTC offsets
// in each immutable value. Native Go zones have only integral seconds: Native
// returns UTC for fractional offsets rather than faking or rounding that zone.
// CivilTime is a UTC-labeled CIVIL clock, not the instant; UTCOffset is exact.
// Naive inputs attach Moscow with fold=0, including retained imaginary clocks.
// The zero value is invalid. Copies retain metadata without global registries.
type SourceDateTime struct {
	civil    time.Time
	native   time.Time
	offset   time.Duration
	valid    bool
	dateOnly bool
	aware    bool
}

func (d SourceDateTime) Valid() bool { return d.valid }

func (d SourceDateTime) Native() time.Time { return d.native }

func (d SourceDateTime) CivilTime() time.Time { return d.civil }

func (d SourceDateTime) UTCOffset() time.Duration { return d.offset }

// DateOnly reports whether the ORIGINAL spelling is independently accepted
// by the canonical DATE scanner. It does not mean the DATETIME has no clock
// or shares that DATE's calendar day; ParseSourceDate exposes the other meaning.
func (d SourceDateTime) DateOnly() bool { return d.dateOnly }

// ISOFormat matches the source comparable moment's datetime.isoformat(), with
// four-digit years, optional six-digit microseconds and exact offset seconds /
// microseconds. It is not the bank's second-resolution _iso_moscow wire helper.
func (d SourceDateTime) ISOFormat() string {
	if !d.valid {
		return ""
	}
	text := d.civil.Format("2006-01-02T15:04:05")
	if micro := d.civil.Nanosecond() / 1000; micro != 0 {
		text += fmt.Sprintf(".%06d", micro)
	}
	off, sign := d.offset.Microseconds(), "+"
	if off < 0 {
		off, sign = -off, "-"
	}
	seconds, micro := off/1000000, off%1000000
	text += fmt.Sprintf("%s%02d:%02d", sign, seconds/3600, seconds/60%60)
	if seconds%60 != 0 || micro != 0 {
		text += fmt.Sprintf(":%02d", seconds%60)
	}
	if micro != 0 {
		text += fmt.Sprintf(".%06d", micro)
	}
	return text
}

// ParseSourceDateTime implements the canonical Python3.12 source ISO grammar
// and default naive-Moscow attachment entirely in Go. Clock/offset fractions
// truncate to microseconds ONCE when read, before computing the true instant.
// Source civil years are 1..9999; a truthful fractional-offset Native UTC value
// can be outside those civil years. Moscow-bound conversion validates separately.
func ParseSourceDateTime(value string) (SourceDateTime, error) {
	invalid := func() (SourceDateTime, error) { return SourceDateTime{}, NewParseError("date") }
	if !utf8.ValidString(value) {
		return invalid()
	}
	_, dateErr := ParseSourceDate(value)
	dateOnly := dateErr == nil
	value = domainCanonicalBasicISO(value)
	if len(value) < 10 {
		return invalid()
	}
	day, err := time.Parse("2006-01-02", value[:10])
	if err != nil || day.Year() < 1 || day.Year() > 9999 {
		return invalid()
	}
	wall := day
	aware, zoneText := false, ""
	if len(value) != 10 {
		if len(value) < 13 {
			return invalid()
		}
		clock := value[11:]
		end := strings.IndexAny(clock, "Z+-")
		aware = end >= 0
		if !aware {
			end = len(clock)
		} else {
			zoneText = clock[end:]
		}
		fields, residual, valid := domainISOClock(clock, end)
		if !valid || (!aware && residual) || fields[0] > 23 || fields[1] > 59 || fields[2] > 59 {
			return invalid()
		}
		wall = time.Date(day.Year(), day.Month(), day.Day(), fields[0], fields[1], fields[2], fields[3]*1000, time.UTC)
	}
	var moment time.Time
	var offset time.Duration
	if !aware {
		moment, err = domainAttachMoscow(wall)
		if err != nil {
			return invalid()
		}
		_, seconds := moment.Zone()
		offset = time.Duration(seconds) * time.Second
	} else {
		if zoneText != "Z" && !(zoneText[0] == 'Z' && domainISOByte(zoneText, 1) == 0) {
			if len(zoneText) < 3 || (zoneText[0] != '+' && zoneText[0] != '-') {
				return invalid()
			}
			fields, residual, valid := domainISOClock(zoneText[1:], len(zoneText)-1)
			if !valid || residual {
				return invalid()
			}
			seconds := fields[0]*3600 + fields[1]*60 + fields[2]
			// Canonical C datetime treats a zero integral offset as UTC and
			// ignores its fractional component (including negative zero).
			if seconds != 0 {
				offset = time.Duration(seconds)*time.Second + time.Duration(fields[3])*time.Microsecond
			}
			if zoneText[0] == '-' {
				offset = -offset
			}
		}
		if offset <= -24*time.Hour || offset >= 24*time.Hour {
			return invalid()
		}
		moment = wall.Add(-offset)
		if offset%time.Second == 0 && offset != 0 {
			moment = moment.In(time.FixedZone("", int(offset/time.Second)))
		}
	}
	return SourceDateTime{civil: wall, native: moment, offset: offset, valid: true, dateOnly: dateOnly, aware: aware}, nil
}

func domainISOMoment(value string) (time.Time, error) {
	moment, err := ParseSourceDateTime(value)
	return moment.Native(), err
}

// An imaginary naive wall clock cannot be represented in a real IANA location:
// In(Moscow) would move it across the gap. A named, native fixed location carries
// its fold=0 offset in the time.Time itself; copies retain it without registries
// or extra TimeFilter fields. Ordinary native UTC/fixed/IANA inputs still convert.
const domainMoscowWallZone = "Europe/Moscow (naive wall)"

// domainAttachMoscow implements source replace(tzinfo=MOSCOW, fold=0), not Go
// Date's unspecified choice at transitions. wall is a validated UTC time whose
// fields represent the naive clock. Walk actual IANA UTC intervals in order:
// the first valid candidate is the earlier fold; a forward gap uses the offset
// immediately before the transition and retains the original imaginary clock.
func domainAttachMoscow(wall time.Time) (time.Time, error) {
	const window = 24 * time.Hour // source offsets are strictly less than 24h
	limit := wall.Add(window)
	gapOffset, inGap := 0, false
	for cursor := wall.Add(-window); cursor.Before(limit); {
		zone := cursor.In(domainMoscow)
		_, offset := zone.Zone()
		if offset <= -24*60*60 || offset >= 24*60*60 {
			return time.Time{}, NewParseError("date")
		}
		start, end := zone.ZoneBounds()
		candidate := wall.Add(-time.Duration(offset) * time.Second)
		if (start.IsZero() || !candidate.Before(start)) && (end.IsZero() || candidate.Before(end)) {
			return candidate.In(domainMoscow), nil
		}
		if end.IsZero() {
			break
		}
		if !end.After(cursor) {
			return time.Time{}, NewParseError("date")
		}
		_, nextOffset := end.In(domainMoscow).Zone()
		beforeWall := end.UTC().Add(time.Duration(offset) * time.Second)
		afterWall := end.UTC().Add(time.Duration(nextOffset) * time.Second)
		if !wall.Before(beforeWall) && wall.Before(afterWall) {
			gapOffset, inGap = offset, true
		}
		cursor = end
	}
	if inGap {
		return wall.Add(-time.Duration(gapOffset) * time.Second).In(time.FixedZone(domainMoscowWallZone, gapOffset)), nil
	}
	return time.Time{}, NewParseError("date")
}

func domainMoscowBound(moment time.Time) time.Time {
	if moment.Location().String() == domainMoscowWallZone {
		return moment
	}
	return moment.In(domainMoscow)
}

// Python datetime supports years 1..9999 and timezone offsets strictly below
// 24 hours. Go permits year zero and normalizes +24:00, so parsing alone is not
// sufficient validation. Historical sub-minute offsets remain legitimate.
func domainSourceMoment(moment time.Time) (time.Time, error) {
	_, offset := moment.Zone()
	if moment.Year() < 1 || moment.Year() > 9999 || offset <= -24*60*60 || offset >= 24*60*60 {
		return time.Time{}, NewParseError("date")
	}
	// Text fractions were already truncated once by domainISOClock. This is
	// range validation, not permission to normalize explicit native values.
	return moment, nil
}

// OperationSortKey treats invalid dates as the oldest UTC moment, and naive
// ISO dates as Moscow time. It never guesses an unknown operation's date.
func OperationSortKey(date string) time.Time {
	t, err := domainISOMoment(date)
	if err != nil {
		return time.Time{}
	}
	return t
}

// SourceOperationCompare reproduces the canonical operation_sort_key ordering
// relation. Parsed naive values share one Moscow ZoneInfo and compare civil
// fields, even across imaginary clocks; aware fixed-zone and mixed-identity
// pairs compare truthful instants. Invalid input uses the source min-UTC key.
// A zero result means neither < nor >, NOT Python datetime equality. The mixed
// relation can contain cycles: do not pass it to an arbitrary total-order sort.
// SortSourceOperations supplies the pinned canonical application algorithm.
func SourceOperationCompare(a, b string) int {
	return domainCompareSourceKeys(domainOperationSourceKey(a), domainOperationSourceKey(b))
}

func domainOperationSourceKey(date string) SourceDateTime {
	d, err := ParseSourceDateTime(date)
	if err != nil {
		// Python's sentinel is datetime.min with the UTC singleton, not a
		// made-up instant before every parse-valid year-one Moscow clock.
		return SourceDateTime{civil: time.Time{}, native: time.Time{}, valid: true, aware: true}
	}
	return d
}

func domainCompareSourceKeys(a, b SourceDateTime) int {
	if !a.aware && !b.aware {
		return a.civil.Compare(b.civil)
	}
	return a.native.Compare(b.native)
}
