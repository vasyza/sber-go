// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"fmt"
	"time"
)

// NewTimeFilter accepts ISO dates or datetimes; empty strings leave a bound
// unset. Aware datetimes convert to Europe/Moscow, naive ones start there.
func NewTimeFilter(from, to string) (TimeFilter, error) {
	return domainNewTimeFilter(from, to, false)
}

// NewSourceTimeFilter uses the canonical application's Moscow request-WALL
// ordering, before truncating wire output to seconds. It is separate from the
// native NewTimeFilter instant-order convention. Returned bounds remain real
// native instants with source imaginary-clock attachment; Contains still uses
// the native instant contract. No explicit native bound or nanosecond is changed.
func NewSourceTimeFilter(from, to string) (TimeFilter, error) {
	return domainNewTimeFilter(from, to, true)
}

func domainNewTimeFilter(from, to string, sourceWall bool) (TimeFilter, error) {
	var f TimeFilter
	for _, bound := range []struct {
		text   string
		end    bool
		target **time.Time
	}{{from, false, &f.From}, {to, true, &f.To}} {
		if bound.text == "" {
			continue
		}
		// The string adapter tries the independent DATE scanner first, exactly
		// like the canonical typed-input oracle. Do not infer this from the
		// DATETIME scanner's separator width or use its possibly different day.
		if day, dateErr := ParseSourceDate(bound.text); dateErr == nil {
			if bound.end {
				day = day.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
			}
			moment, err := domainAttachMoscow(day)
			if err != nil {
				return TimeFilter{}, err
			}
			*bound.target = &moment
			continue
		}
		source, err := ParseSourceDateTime(bound.text)
		if err != nil {
			return TimeFilter{}, err
		}
		// Canonical astimezone first constructs a bounded UTC datetime. That
		// intermediate can overflow even when the eventual Moscow wall fits.
		// Naive attachment has no such conversion and remains valid at year 1.
		if source.aware {
			year := source.Native().UTC().Year()
			if year < 1 || year > 9999 {
				return TimeFilter{}, NewParseError("date")
			}
		}
		moment, err := domainSourceMoment(domainMoscowBound(source.Native()))
		if err != nil {
			return TimeFilter{}, err
		}
		if bound.end && source.DateOnly() {
			moment, err = domainAttachMoscow(time.Date(moment.Year(), moment.Month(), moment.Day(), 23, 59, 59, 0, time.UTC))
			if err != nil {
				return TimeFilter{}, err
			}
		}
		*bound.target = &moment
	}
	if f.From != nil && f.To != nil {
		a, b := *f.From, *f.To
		if sourceWall {
			a, b = domainRequestWall(a), domainRequestWall(b)
		}
		if a.After(b) {
			return TimeFilter{}, NewParseError("time_filter")
		}
	}
	return f, nil
}

// Strip timezone identity without normalizing imaginary clocks. Comparing
// these civil fields includes all retained microseconds, unlike wire strings.
func domainRequestWall(moment time.Time) time.Time {
	wall := domainMoscowBound(moment)
	return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.UTC)
}

func (f TimeFilter) Contains(date string) bool {
	t, err := domainISOMoment(date)
	if err != nil {
		return false
	}
	return (f.From == nil || !t.Before(*f.From)) && (f.To == nil || !t.After(*f.To))
}

func (f TimeFilter) RequestBounds() (string, string) {
	return domainBoundFormat(f.From, "02.01.2006T15:04:05"), domainBoundFormat(f.To, "02.01.2006T15:04:05")
}

func (f TimeFilter) ISOBounds() (string, string) {
	return domainBoundFormat(f.From, time.RFC3339), domainBoundFormat(f.To, time.RFC3339)
}

// SourceRequestBounds is the additive canonical _request_datetime wire path.
// Unlike the legacy native RequestBounds, early years match Python3.12/Linux
// strftime without padding. Explicit native bounds are read, never rewritten.
func (f TimeFilter) SourceRequestBounds() (string, string, error) {
	return f.domainSourceBounds(false)
}

// SourceISOBounds matches _iso_moscow: second precision and its fixed +03:00
// suffix, even for historical Moscow clocks. The suffix is SOURCE WIRE POLICY,
// not the true instant's UTC offset. ISOBounds retains the native IANA policy.
func (f TimeFilter) SourceISOBounds() (string, string, error) {
	return f.domainSourceBounds(true)
}

func (f TimeFilter) domainSourceBounds(iso bool) (string, string, error) {
	var out [2]string
	for i, bound := range []*time.Time{f.From, f.To} {
		if bound == nil {
			continue
		}
		// Validate both source-like native input and destination conversion;
		// never return one successful bound alongside a failed other bound.
		if _, err := domainSourceMoment(*bound); err != nil {
			return "", "", err
		}
		if bound.Location() != domainMoscow && bound.Location().String() != domainMoscowWallZone {
			year := bound.UTC().Year()
			if year < 1 || year > 9999 {
				return "", "", NewParseError("date")
			}
		}
		wall := domainMoscowBound(*bound)
		if _, err := domainSourceMoment(wall); err != nil {
			return "", "", err
		}
		if iso {
			out[i] = fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02d+03:00", wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second())
		} else {
			out[i] = fmt.Sprintf("%02d.%02d.%dT%02d:%02d:%02d", wall.Day(), wall.Month(), wall.Year(), wall.Hour(), wall.Minute(), wall.Second())
		}
	}
	return out[0], out[1], nil
}

func domainBoundFormat(value *time.Time, layout string) string {
	if value == nil {
		return ""
	}
	if layout == time.RFC3339 {
		return domainISOFormat(domainMoscowBound(*value))
	}
	return domainMoscowBound(*value).Format(layout)
}
