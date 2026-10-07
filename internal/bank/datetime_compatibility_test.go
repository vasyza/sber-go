package bank

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReviewCycle3CalendarWeekDateOnlyUpperSecond(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(cycle3CanonicalTracerJSON))
	for dec.More() {
		var row cycle3Row
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.Family != "date_only" {
			continue
		}
		t.Run(row.ID, func(t *testing.T) {
			source, err := ParseSourceDateTime(row.Text)
			if err != nil || source.DateOnly() != row.DateOnly {
				t.Fatal("date/datetime source distinction lost")
			}
			f, err := NewTimeFilter(row.Text, row.Text)
			if err != nil {
				t.Fatal(err)
			}
			a, b := row.FromBound.Fields, row.ToBound.Fields
			from := time.Unix(a.UnixSecond, int64(a.InstantMicrosecond)*1000)
			to := time.Unix(b.UnixSecond, int64(b.InstantMicrosecond)*1000)
			if f.From == nil || f.To == nil || !f.From.Equal(from) || !f.To.Equal(to) || !f.Contains(row.Text) {
				t.Fatal("source inclusive date bounds lost")
			}
			if row.DateOnly {
				civil := source.CivilTime()
				last := civil.Format("2006-01-02") + "T23:59:59"
				if !f.Contains(last) || !f.Contains(last+".0000009") || f.Contains(last+".000001") {
					t.Fatal("date-only upper second rounded/excluded")
				}
			}
		})
	}
}

// Every source row is retained, valid or invalid: no eligible-subset filter.
// Python is solely the archived licensed offline oracle, not a Go dependency.
func TestReviewCycle3CompleteCanonicalSourceCorpus(t *testing.T) {
	data, err := os.ReadFile("../../testdata/datetime/source-corpus.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	count := 0
	for {
		var row struct {
			cycle3Row
			Bank     string
			BankDate string `json:"bank_date"`
		}
		err := dec.Decode(&row)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		count++
		t.Run(row.ID, func(t *testing.T) {
			source, err := ParseSourceDateTime(row.Text)
			key := OperationSortKey(row.Text)
			if (err == nil) != row.Valid || source.Valid() != row.Valid || (TimeFilter{}).Contains(row.Text) != row.Valid {
				t.Fatalf("source acceptance mismatch for %q", row.Text)
			}
			if row.Valid {
				if key.Unix() != row.Key.UnixSecond || key.Nanosecond()/1000 != row.Key.InstantMicrosecond || !key.Equal(source.Native()) {
					t.Fatal("true source sort instant lost")
				}
				civil := source.CivilTime()
				k := row.Key
				if civil.Year() != k.Year || int(civil.Month()) != k.Month || civil.Day() != k.Day || civil.Hour() != k.Hour || civil.Minute() != k.Minute || civil.Second() != k.Second || civil.Nanosecond()/1000 != k.Microsecond || source.UTCOffset().Microseconds() != k.OffsetMicroseconds || source.ISOFormat() != row.SourceISO || source.DateOnly() != row.DateOnly {
					t.Fatalf("source wall/offset/precision mismatch: %q -> %s", row.Text, source.ISOFormat())
				}
				if k.OffsetMicroseconds%1000000 == 0 {
					cycle3CheckFields(t, key, k)
				} else if key.Location() != time.UTC {
					t.Fatal("fractional source offset faked as native zone")
				}
				// Boundary precision and inclusive comparisons operate on true
				// instants, not civil fields or rounded offset seconds.
				later, earlier := key.Add(time.Nanosecond), key.Add(-time.Nanosecond)
				if (TimeFilter{From: &later}).Contains(row.Text) || (TimeFilter{To: &earlier}).Contains(row.Text) {
					t.Fatal("neighboring native nanosecond bound collapsed")
				}
			} else if !key.IsZero() || source.ISOFormat() != "" {
				t.Fatal("invalid source acquired key/metadata")
			}
			f, err := NewTimeFilter(row.Text, row.Text)
			if row.Text == "" {
				if err != nil || f.From != nil || f.To != nil || f.Contains("") {
					t.Fatal("native unset constructor adaptation changed")
				}
			} else if !row.BoundsValid {
				var pe *ParseError
				if !errors.As(err, &pe) || pe.Field() != "date" || f.From != nil || f.To != nil {
					t.Fatalf("source parse/conversion rejection partially published: %q %+v %v", row.Text, f, err)
				}
				// Check both constructor bound positions, not only both at once.
				for _, pair := range [][2]string{{row.Text, ""}, {"", row.Text}} {
					if g, e := NewTimeFilter(pair[0], pair[1]); e == nil || g.From != nil || g.To != nil {
						t.Fatal("one-sided invalid/overflow source bound published")
					}
				}
			} else {
				if err != nil || f.From == nil || f.To == nil || !f.Contains(row.Text) || !f.Contains(source.ISOFormat()) {
					t.Fatalf("valid source filter failed: %q %v", row.Text, err)
				}
				cycle3CheckFields(t, *f.From, row.FromBound.Fields)
				cycle3CheckFields(t, *f.To, row.ToBound.Fields)
				a, b, e := f.SourceRequestBounds()
				if e != nil || a != row.FromBound.Request || b != row.ToBound.Request {
					t.Fatalf("canonical request wire mismatch: %q/%q %v", a, b, e)
				}
				a, b, e = f.SourceISOBounds()
				if e != nil || a != row.FromBound.ISO || b != row.ToBound.ISO {
					t.Fatalf("canonical ISO wire mismatch: %q/%q %v", a, b, e)
				}
				beforeFrom, beforeTo := *f.From, *f.To
				copiedFrom, copiedTo := beforeFrom, beforeTo
				copied := TimeFilter{From: &copiedFrom, To: &copiedTo}
				a, b, e = copied.SourceRequestBounds()
				if e != nil || a != row.FromBound.Request || b != row.ToBound.Request || !copied.Contains(row.Text) || *f.From != beforeFrom || *f.To != beforeTo {
					t.Fatal("copied bounds lost metadata/instant or helper mutated original")
				}
			}
			if row.Bank != "" {
				payload := map[string]any{"body": map[string]any{"operations": []any{map[string]any{"date": row.Bank, "uohId": "cycle3-synthetic"}}}}
				before, _ := json.Marshal(payload)
				ops, e := ParseOperations(payload)
				after, _ := json.Marshal(payload)
				if e != nil || len(ops) != 1 || ops[0].Date != row.BankDate || !bytes.Equal(before, after) {
					t.Fatal("bank-date helper diverged / mutated input")
				}
			}
		})
	}
	if count != 20011 {
		t.Fatalf("full source denominator lost: %d", count)
	}
}
func cycle3CheckFields(t *testing.T, moment time.Time, k cycle3Fields) {
	t.Helper()
	_, off := moment.Zone()
	if moment.Year() != k.Year || int(moment.Month()) != k.Month || moment.Day() != k.Day || moment.Hour() != k.Hour || moment.Minute() != k.Minute || moment.Second() != k.Second || moment.Nanosecond()/1000 != k.Microsecond || int64(off)*1000000 != k.OffsetMicroseconds || moment.Unix() != k.UnixSecond || moment.Nanosecond()/1000 != k.InstantMicrosecond {
		t.Fatalf("source-representable time fields changed: %v / %+v", moment, k)
	}
}

// Fixture bytes retain the original embedded corpus without a final newline.
// SHA256: 8e21abcd997df913c584b67a6a00b190ecc88a758602a2c98a0d1a72128241d5

// Newly exposed by the full, unfiltered canonical differential, not inferred
// from a native parse/format round trip. The source accepts Z followed by a NUL
// and ignores its remaining text; a single newline is also a date separator.
func TestReviewCycle3CanonicalNULAndBasicLineSeparator(t *testing.T) {
	want := time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC)
	for _, text := range []string{"2026-08-01T12:30:00Z\x00ignored", "20260801\n12:30:00Z"} {
		t.Run(text, func(t *testing.T) { cycle3DateAccepted(t, text, want) })
	}
}

type cycle3Fields struct {
	Year, Month, Day, Hour, Minute, Second, Microsecond int
	OffsetMicroseconds                                  int64 `json:"offset_microseconds"`
	UnixSecond                                          int64 `json:"unix_second"`
	InstantMicrosecond                                  int   `json:"instant_microsecond"`
}
type cycle3Bound struct {
	Fields       cycle3Fields
	Request, ISO string
}
type cycle3Row struct {
	ID, Text, Family string
	ParsedISO        string `json:"parsed_iso"`
	SourceISO        string `json:"source_iso"`
	Valid            bool
	DateOnly         bool `json:"date_only"`
	BoundsValid      bool `json:"bounds_valid"`
	Key              cycle3Fields
	FromBound        cycle3Bound `json:"from_bound"`
	ToBound          cycle3Bound `json:"to_bound"`
}

func cycle3Family(t *testing.T, family string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(cycle3CanonicalTracerJSON))
	for dec.More() {
		var row cycle3Row
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.Family != family {
			continue
		}
		t.Run(row.ID, func(t *testing.T) {
			if !row.Valid {
				cycle3DateRejected(t, row.Text)
				return
			}
			want := time.Unix(row.Key.UnixSecond, int64(row.Key.InstantMicrosecond)*1000)
			cycle3DateAccepted(t, row.Text, want)
			got := OperationSortKey(row.Text)
			if row.Key.OffsetMicroseconds%1000000 == 0 {
				_, off := got.Zone()
				if got.Year() != row.Key.Year || int(got.Month()) != row.Key.Month || got.Day() != row.Key.Day || got.Hour() != row.Key.Hour || got.Minute() != row.Key.Minute || got.Second() != row.Key.Second || got.Nanosecond()/1000 != row.Key.Microsecond || int64(off)*1000000 != row.Key.OffsetMicroseconds {
					t.Fatal("representable source wall/zone changed")
				}
			}
		})
	}
}
func TestReviewCycle3ISOWeekCalendar(t *testing.T)                 { cycle3Family(t, "week") }
func TestReviewCycle3SingleUnicodeSeparator(t *testing.T)          { cycle3Family(t, "separator") }
func TestReviewCycle3ReducedFractionsAndAwareMarkers(t *testing.T) { cycle3Family(t, "reduced") }
func TestReviewCycle3ExactUTCOffsets(t *testing.T)                 { cycle3Family(t, "offset") }

// Raw, unfiltered actual /usr/bin/python3 3.12.3 source rows. Original helper
// and input bytes are independently sealed in repair-cycle3/dates.
const cycle3CanonicalTracerJSON = `{"id":"tracer-basic-000","family":"basic","text":"20260801T12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-001","family":"basic","text":"2026-08-01T123000Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-002","family":"basic","text":"20260801T123000Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-003","family":"basic","text":"20260801T1230Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-004","family":"basic","text":"20260801T12Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785585600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:00:00+00:00","source_iso":"2026-08-01T12:00:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":0},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":0},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-005","family":"basic","text":"2026081T123000Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-basic-006","family":"basic","text":"20260230T123000Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-basic-007","family":"basic","text":"00000801T123000Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-basic-008","family":"basic","text":"20260801T1Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-basic-009","family":"basic","text":"20260801T123Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785585600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:00:00+00:00","source_iso":"2026-08-01T12:00:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":0},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":0},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-010","family":"basic","text":"20260801T12300Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-basic-011","family":"basic","text":"20260801T24Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-basic-012","family":"basic","text":"2026-08-01T12:3000Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-basic-013","family":"basic","text":"2026-08-01T1230:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-000","family":"week","text":"2026-W31-6T12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-week-001","family":"week","text":"2026W316T123000Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-week-002","family":"week","text":"2026-W31T12:30:00Z","key":{"year":2026,"month":7,"day":27,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785155400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-07-27T12:30:00+00:00","source_iso":"2026-07-27T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785155400,"instant_microsecond":0},"request":"27.07.2026T15:30:00","iso":"2026-07-27T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785155400,"instant_microsecond":0},"request":"27.07.2026T15:30:00","iso":"2026-07-27T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-week-003","family":"week","text":"2026W31T123000Z","key":{"year":2026,"month":7,"day":27,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785155400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-07-27T12:30:00+00:00","source_iso":"2026-07-27T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785155400,"instant_microsecond":0},"request":"27.07.2026T15:30:00","iso":"2026-07-27T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785155400,"instant_microsecond":0},"request":"27.07.2026T15:30:00","iso":"2026-07-27T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-week-004","family":"week","text":"2020-W53-7T12:30:00Z","key":{"year":2021,"month":1,"day":3,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1609677000,"instant_microsecond":0},"valid":true,"parsed_iso":"2021-01-03T12:30:00+00:00","source_iso":"2021-01-03T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2021,"month":1,"day":3,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1609677000,"instant_microsecond":0},"request":"03.01.2021T15:30:00","iso":"2021-01-03T15:30:00+03:00"},"to_bound":{"fields":{"year":2021,"month":1,"day":3,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1609677000,"instant_microsecond":0},"request":"03.01.2021T15:30:00","iso":"2021-01-03T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-week-005","family":"week","text":"0001-W01-1T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135551800,"instant_microsecond":0},"valid":true,"parsed_iso":"0001-01-01T12:30:00+00:00","source_iso":"0001-01-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":1,"month":1,"day":1,"hour":15,"minute":0,"second":17,"microsecond":0,"offset_microseconds":9017000000,"unix_second":-62135551800,"instant_microsecond":0},"request":"01.01.1T15:00:17","iso":"1-01-01T15:00:17+03:00"},"to_bound":{"fields":{"year":1,"month":1,"day":1,"hour":15,"minute":0,"second":17,"microsecond":0,"offset_microseconds":9017000000,"unix_second":-62135551800,"instant_microsecond":0},"request":"01.01.1T15:00:17","iso":"1-01-01T15:00:17+03:00"},"bounds_valid":true}
{"id":"tracer-week-006","family":"week","text":"2026-W00-1T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-007","family":"week","text":"2026-W54-1T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-008","family":"week","text":"2026-W53-1T12:30:00Z","key":{"year":2026,"month":12,"day":28,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1798461000,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-12-28T12:30:00+00:00","source_iso":"2026-12-28T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":12,"day":28,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1798461000,"instant_microsecond":0},"request":"28.12.2026T15:30:00","iso":"2026-12-28T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":12,"day":28,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1798461000,"instant_microsecond":0},"request":"28.12.2026T15:30:00","iso":"2026-12-28T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-week-009","family":"week","text":"2026-W31-0T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-010","family":"week","text":"2026-W31-8T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-011","family":"week","text":"9999-W52-7T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-012","family":"week","text":"2026W31-6T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-week-013","family":"week","text":"2026-W316T12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-separator-000","family":"separator","text":"2026-08-01x12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-001","family":"separator","text":"2026-08-01t12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-002","family":"separator","text":"2026-08-01\ud83d\ude4212:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-003","family":"separator","text":"2026-08-01\u044f12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-004","family":"separator","text":"2026-08-01\n12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-005","family":"separator","text":"2026-08-01\u000012:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-006","family":"separator","text":"2026-08-01+12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-007","family":"separator","text":"2026-08-01-12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-008","family":"separator","text":"2026-08-01Z12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-009","family":"separator","text":"2026-08-01012:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-010","family":"separator","text":"2026-08-01\u202812:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-011","family":"separator","text":"2026-08-01\ufffd12:30:00Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-separator-012","family":"separator","text":"2026-08-01xx12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-separator-013","family":"separator","text":"2026-08-01\ud83d\ude42\ud83d\ude4212:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-separator-014","family":"separator","text":"2026-08-01T\uff11\uff12:30:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-separator-015","family":"separator","text":"2026-08-01T12:\uff13\uff10:00Z","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-reduced-000","family":"reduced","text":"2026-08-01T12.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:00:00.500000","source_iso":"2026-08-01T12:00:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":500000},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":500000},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-001","family":"reduced","text":"2026-08-01T12.5Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":0,"unix_second":1785585600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:00:00.500000+00:00","source_iso":"2026-08-01T12:00:00.500000+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":500000},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":500000},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-002","family":"reduced","text":"2026-08-01T12.5+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:00:00.500000+03:00","source_iso":"2026-08-01T12:00:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":500000},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":500000},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-003","family":"reduced","text":"2026-08-01T12,1234569","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":123456},"valid":true,"parsed_iso":"2026-08-01T12:00:00.123456","source_iso":"2026-08-01T12:00:00.123456+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":123456},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":123456},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-004","family":"reduced","text":"2026-08-01T12,1234569Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":0,"unix_second":1785585600,"instant_microsecond":123456},"valid":true,"parsed_iso":"2026-08-01T12:00:00.123456+00:00","source_iso":"2026-08-01T12:00:00.123456+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":123456},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785585600,"instant_microsecond":123456},"request":"01.08.2026T15:00:00","iso":"2026-08-01T15:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-005","family":"reduced","text":"2026-08-01T12,1234569+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":123456},"valid":true,"parsed_iso":"2026-08-01T12:00:00.123456+03:00","source_iso":"2026-08-01T12:00:00.123456+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":123456},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":0,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785574800,"instant_microsecond":123456},"request":"01.08.2026T12:00:00","iso":"2026-08-01T12:00:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-006","family":"reduced","text":"2026-08-01T12:30.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000","source_iso":"2026-08-01T12:30:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-007","family":"reduced","text":"2026-08-01T12:30.5Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000+00:00","source_iso":"2026-08-01T12:30:00.500000+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":500000},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":500000},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-008","family":"reduced","text":"2026-08-01T12:30.5+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000+03:00","source_iso":"2026-08-01T12:30:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-009","family":"reduced","text":"2026-08-01T1230,5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000","source_iso":"2026-08-01T12:30:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-010","family":"reduced","text":"2026-08-01T1230,5Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000+00:00","source_iso":"2026-08-01T12:30:00.500000+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":500000},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":500000},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-011","family":"reduced","text":"2026-08-01T1230,5+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000+03:00","source_iso":"2026-08-01T12:30:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-012","family":"reduced","text":"2026-08-01T123000.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000","source_iso":"2026-08-01T12:30:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-013","family":"reduced","text":"2026-08-01T123000.5Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000+00:00","source_iso":"2026-08-01T12:30:00.500000+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":500000},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":500000},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-014","family":"reduced","text":"2026-08-01T123000.5+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00.500000+03:00","source_iso":"2026-08-01T12:30:00.500000+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":500000},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-015","family":"reduced","text":"2026-08-01T12:30:00.","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-reduced-016","family":"reduced","text":"2026-08-01T12:30:00.Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-017","family":"reduced","text":"2026-08-01T12:30:00.+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+03:00","source_iso":"2026-08-01T12:30:00+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":0},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":0},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-018","family":"reduced","text":"2026-08-01T12:30:00,","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-reduced-019","family":"reduced","text":"2026-08-01T12:30:00,Z","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-020","family":"reduced","text":"2026-08-01T12:30:00,+03:00","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+03:00","source_iso":"2026-08-01T12:30:00+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":0},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576600,"instant_microsecond":0},"request":"01.08.2026T12:30:00","iso":"2026-08-01T12:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-021","family":"reduced","text":"2026-08-01T12:30:00.12xZ","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-reduced-022","family":"reduced","text":"2026-08-01T12:30:00.xZ","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-reduced-023","family":"reduced","text":"2026-08-01T12:30:00.123456xZ","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":123456,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":123456},"valid":true,"parsed_iso":"2026-08-01T12:30:00.123456+00:00","source_iso":"2026-08-01T12:30:00.123456+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":123456},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":123456,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":123456},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-reduced-024","family":"reduced","text":"2026-08-01T12:30:00.123456x","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-000","family":"offset","text":"2026-08-01T12:30:00+030017","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10817000000,"unix_second":1785576583,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+03:00:17","source_iso":"2026-08-01T12:30:00+03:00:17","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":29,"second":43,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576583,"instant_microsecond":0},"request":"01.08.2026T12:29:43","iso":"2026-08-01T12:29:43+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":29,"second":43,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785576583,"instant_microsecond":0},"request":"01.08.2026T12:29:43","iso":"2026-08-01T12:29:43+03:00"},"bounds_valid":true}
{"id":"tracer-offset-001","family":"offset","text":"2026-08-01T12:30:00-030017","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":-10817000000,"unix_second":1785598217,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00-03:00:17","source_iso":"2026-08-01T12:30:00-03:00:17","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":18,"minute":30,"second":17,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785598217,"instant_microsecond":0},"request":"01.08.2026T18:30:17","iso":"2026-08-01T18:30:17+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":18,"minute":30,"second":17,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785598217,"instant_microsecond":0},"request":"01.08.2026T18:30:17","iso":"2026-08-01T18:30:17+03:00"},"bounds_valid":true}
{"id":"tracer-offset-002","family":"offset","text":"2026-08-01T12:30:00+02:99","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":13140000000,"unix_second":1785574260,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+03:39","source_iso":"2026-08-01T12:30:00+03:39","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":11,"minute":51,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785574260,"instant_microsecond":0},"request":"01.08.2026T11:51:00","iso":"2026-08-01T11:51:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":11,"minute":51,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785574260,"instant_microsecond":0},"request":"01.08.2026T11:51:00","iso":"2026-08-01T11:51:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-003","family":"offset","text":"2026-08-01T12:30:00-02:99","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":-13140000000,"unix_second":1785600540,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00-03:39","source_iso":"2026-08-01T12:30:00-03:39","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":19,"minute":9,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785600540,"instant_microsecond":0},"request":"01.08.2026T19:09:00","iso":"2026-08-01T19:09:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":19,"minute":9,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785600540,"instant_microsecond":0},"request":"01.08.2026T19:09:00","iso":"2026-08-01T19:09:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-004","family":"offset","text":"2026-08-01T12:30:00+02:30:99","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":9099000000,"unix_second":1785578301,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+02:31:39","source_iso":"2026-08-01T12:30:00+02:31:39","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":58,"second":21,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785578301,"instant_microsecond":0},"request":"01.08.2026T12:58:21","iso":"2026-08-01T12:58:21+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":58,"second":21,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785578301,"instant_microsecond":0},"request":"01.08.2026T12:58:21","iso":"2026-08-01T12:58:21+03:00"},"bounds_valid":true}
{"id":"tracer-offset-005","family":"offset","text":"2026-08-01T12:30:00+0099","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":5940000000,"unix_second":1785581460,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+01:39","source_iso":"2026-08-01T12:30:00+01:39","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":13,"minute":51,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785581460,"instant_microsecond":0},"request":"01.08.2026T13:51:00","iso":"2026-08-01T13:51:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":13,"minute":51,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785581460,"instant_microsecond":0},"request":"01.08.2026T13:51:00","iso":"2026-08-01T13:51:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-006","family":"offset","text":"2026-08-01T12:30:00+00:00:99","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":99000000,"unix_second":1785587301,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:01:39","source_iso":"2026-08-01T12:30:00+00:01:39","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":28,"second":21,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587301,"instant_microsecond":0},"request":"01.08.2026T15:28:21","iso":"2026-08-01T15:28:21+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":28,"second":21,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587301,"instant_microsecond":0},"request":"01.08.2026T15:28:21","iso":"2026-08-01T15:28:21+03:00"},"bounds_valid":true}
{"id":"tracer-offset-007","family":"offset","text":"2026-08-01T12:30:00+23:59:59","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":86399000000,"unix_second":1785501001,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+23:59:59","source_iso":"2026-08-01T12:30:00+23:59:59","date_only":false,"from_bound":{"fields":{"year":2026,"month":7,"day":31,"hour":15,"minute":30,"second":1,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785501001,"instant_microsecond":0},"request":"31.07.2026T15:30:01","iso":"2026-07-31T15:30:01+03:00"},"to_bound":{"fields":{"year":2026,"month":7,"day":31,"hour":15,"minute":30,"second":1,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785501001,"instant_microsecond":0},"request":"31.07.2026T15:30:01","iso":"2026-07-31T15:30:01+03:00"},"bounds_valid":true}
{"id":"tracer-offset-008","family":"offset","text":"2026-08-01T12:30:00+23:99","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-009","family":"offset","text":"2026-08-01T12:30:00+24","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-010","family":"offset","text":"2026-08-01T12:30:00+03:00:00.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800500000,"unix_second":1785576599,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00+03:00:00.500000","source_iso":"2026-08-01T12:30:00+03:00:00.500000","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":29,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576599,"instant_microsecond":500000},"request":"01.08.2026T12:29:59","iso":"2026-08-01T12:29:59+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":29,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785576599,"instant_microsecond":500000},"request":"01.08.2026T12:29:59","iso":"2026-08-01T12:29:59+03:00"},"bounds_valid":true}
{"id":"tracer-offset-011","family":"offset","text":"2026-08-01T12:30:00-03:00:00.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":-10800500000,"unix_second":1785598200,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00-03:00:00.500000","source_iso":"2026-08-01T12:30:00-03:00:00.500000","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":18,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785598200,"instant_microsecond":500000},"request":"01.08.2026T18:30:00","iso":"2026-08-01T18:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":18,"minute":30,"second":0,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785598200,"instant_microsecond":500000},"request":"01.08.2026T18:30:00","iso":"2026-08-01T18:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-012","family":"offset","text":"2026-08-01T12:30:00+00:00:00.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-013","family":"offset","text":"2026-08-01T12:30:00-00:00:00.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-014","family":"offset","text":"2026-08-01T12:30:00+00.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-015","family":"offset","text":"2026-08-01T12:30:00+00:00.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":1785587400,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00+00:00","source_iso":"2026-08-01T12:30:00+00:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587400,"instant_microsecond":0},"request":"01.08.2026T15:30:00","iso":"2026-08-01T15:30:00+03:00"},"bounds_valid":true}
{"id":"tracer-offset-016","family":"offset","text":"2026-08-01T12:30:00+02.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":7200500000,"unix_second":1785580199,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00+02:00:00.500000","source_iso":"2026-08-01T12:30:00+02:00:00.500000","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":13,"minute":29,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785580199,"instant_microsecond":500000},"request":"01.08.2026T13:29:59","iso":"2026-08-01T13:29:59+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":13,"minute":29,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785580199,"instant_microsecond":500000},"request":"01.08.2026T13:29:59","iso":"2026-08-01T13:29:59+03:00"},"bounds_valid":true}
{"id":"tracer-offset-017","family":"offset","text":"2026-08-01T12:30:00+0230.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":9000500000,"unix_second":1785578399,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00+02:30:00.500000","source_iso":"2026-08-01T12:30:00+02:30:00.500000","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":59,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785578399,"instant_microsecond":500000},"request":"01.08.2026T12:59:59","iso":"2026-08-01T12:59:59+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":59,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785578399,"instant_microsecond":500000},"request":"01.08.2026T12:59:59","iso":"2026-08-01T12:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-offset-018","family":"offset","text":"2026-08-01T12:30:00+02:30.5","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":9000500000,"unix_second":1785578399,"instant_microsecond":500000},"valid":true,"parsed_iso":"2026-08-01T12:30:00+02:30:00.500000","source_iso":"2026-08-01T12:30:00+02:30:00.500000","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":59,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785578399,"instant_microsecond":500000},"request":"01.08.2026T12:59:59","iso":"2026-08-01T12:59:59+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":59,"second":59,"microsecond":500000,"offset_microseconds":10800000000,"unix_second":1785578399,"instant_microsecond":500000},"request":"01.08.2026T12:59:59","iso":"2026-08-01T12:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-offset-019","family":"offset","text":"2026-08-01T12:30:00+03:00:30.9999999","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":10830999999,"unix_second":1785576569,"instant_microsecond":1},"valid":true,"parsed_iso":"2026-08-01T12:30:00+03:00:30.999999","source_iso":"2026-08-01T12:30:00+03:00:30.999999","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":29,"second":29,"microsecond":1,"offset_microseconds":10800000000,"unix_second":1785576569,"instant_microsecond":1},"request":"01.08.2026T12:29:29","iso":"2026-08-01T12:29:29+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":12,"minute":29,"second":29,"microsecond":1,"offset_microseconds":10800000000,"unix_second":1785576569,"instant_microsecond":1},"request":"01.08.2026T12:29:29","iso":"2026-08-01T12:29:29+03:00"},"bounds_valid":true}
{"id":"tracer-offset-020","family":"offset","text":"2026-08-01T12:30:00-00:00:01.0000009","key":{"year":2026,"month":8,"day":1,"hour":12,"minute":30,"second":0,"microsecond":0,"offset_microseconds":-1000000,"unix_second":1785587401,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T12:30:00-00:00:01","source_iso":"2026-08-01T12:30:00-00:00:01","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":1,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587401,"instant_microsecond":0},"request":"01.08.2026T15:30:01","iso":"2026-08-01T15:30:01+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":15,"minute":30,"second":1,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785587401,"instant_microsecond":0},"request":"01.08.2026T15:30:01","iso":"2026-08-01T15:30:01+03:00"},"bounds_valid":true}
{"id":"tracer-offset-021","family":"offset","text":"2026-08-01T12:30:00+00:00:00.","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-022","family":"offset","text":"2026-08-01T12:30:00+03:00:00.","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-023","family":"offset","text":"2026-08-01T12:30:00+03:00:00,","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-024","family":"offset","text":"2026-08-01T12:30:00+03:00:00.5x","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-025","family":"offset","text":"2026-08-01T12:30:00+3:00","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-026","family":"offset","text":"2026-08-01T12:30:00+003","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-offset-027","family":"offset","text":"2026-08-01T12:30:00+03:0000","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":0,"unix_second":-62135596800,"instant_microsecond":0},"valid":false,"bounds_valid":false,"date_only":false}
{"id":"tracer-date_only-000","family":"date_only","text":"20260801","key":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T00:00:00","source_iso":"2026-08-01T00:00:00+03:00","date_only":true,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"request":"01.08.2026T00:00:00","iso":"2026-08-01T00:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785617999,"instant_microsecond":0},"request":"01.08.2026T23:59:59","iso":"2026-08-01T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-001","family":"date_only","text":"2026-W31-6","key":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T00:00:00","source_iso":"2026-08-01T00:00:00+03:00","date_only":true,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"request":"01.08.2026T00:00:00","iso":"2026-08-01T00:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785617999,"instant_microsecond":0},"request":"01.08.2026T23:59:59","iso":"2026-08-01T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-002","family":"date_only","text":"2026W316","key":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T00:00:00","source_iso":"2026-08-01T00:00:00+03:00","date_only":true,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"request":"01.08.2026T00:00:00","iso":"2026-08-01T00:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785617999,"instant_microsecond":0},"request":"01.08.2026T23:59:59","iso":"2026-08-01T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-003","family":"date_only","text":"2026-W31","key":{"year":2026,"month":7,"day":27,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785099600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-07-27T00:00:00","source_iso":"2026-07-27T00:00:00+03:00","date_only":true,"from_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785099600,"instant_microsecond":0},"request":"27.07.2026T00:00:00","iso":"2026-07-27T00:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785185999,"instant_microsecond":0},"request":"27.07.2026T23:59:59","iso":"2026-07-27T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-004","family":"date_only","text":"2026W31","key":{"year":2026,"month":7,"day":27,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785099600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-07-27T00:00:00","source_iso":"2026-07-27T00:00:00+03:00","date_only":true,"from_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785099600,"instant_microsecond":0},"request":"27.07.2026T00:00:00","iso":"2026-07-27T00:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":7,"day":27,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785185999,"instant_microsecond":0},"request":"27.07.2026T23:59:59","iso":"2026-07-27T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-005","family":"date_only","text":"00010101","key":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":9017000000,"unix_second":-62135605817,"instant_microsecond":0},"valid":true,"parsed_iso":"0001-01-01T00:00:00","source_iso":"0001-01-01T00:00:00+02:30:17","date_only":true,"from_bound":{"fields":{"year":1,"month":1,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":9017000000,"unix_second":-62135605817,"instant_microsecond":0},"request":"01.01.1T00:00:00","iso":"1-01-01T00:00:00+03:00"},"to_bound":{"fields":{"year":1,"month":1,"day":1,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":9017000000,"unix_second":-62135519418,"instant_microsecond":0},"request":"01.01.1T23:59:59","iso":"1-01-01T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-006","family":"date_only","text":"99991231","key":{"year":9999,"month":12,"day":31,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":253402203600,"instant_microsecond":0},"valid":true,"parsed_iso":"9999-12-31T00:00:00","source_iso":"9999-12-31T00:00:00+03:00","date_only":true,"from_bound":{"fields":{"year":9999,"month":12,"day":31,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":253402203600,"instant_microsecond":0},"request":"31.12.9999T00:00:00","iso":"9999-12-31T00:00:00+03:00"},"to_bound":{"fields":{"year":9999,"month":12,"day":31,"hour":23,"minute":59,"second":59,"microsecond":0,"offset_microseconds":10800000000,"unix_second":253402289999,"instant_microsecond":0},"request":"31.12.9999T23:59:59","iso":"9999-12-31T23:59:59+03:00"},"bounds_valid":true}
{"id":"tracer-date_only-007","family":"date_only","text":"2026-W31-6T00:00:00","key":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"valid":true,"parsed_iso":"2026-08-01T00:00:00","source_iso":"2026-08-01T00:00:00+03:00","date_only":false,"from_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"request":"01.08.2026T00:00:00","iso":"2026-08-01T00:00:00+03:00"},"to_bound":{"fields":{"year":2026,"month":8,"day":1,"hour":0,"minute":0,"second":0,"microsecond":0,"offset_microseconds":10800000000,"unix_second":1785531600,"instant_microsecond":0},"request":"01.08.2026T00:00:00","iso":"2026-08-01T00:00:00+03:00"},"bounds_valid":true}`

// Canonical-source expectations: actual Python3.12.3 AST helper execution;
// complete raw inputs/outputs/hashes live in repair-cycle3/dates, never Python
// imported or executed by the Go library or its test runtime.
func cycle3DateAccepted(t *testing.T, text string, want time.Time) {
	t.Helper()
	before := text
	moment, err := domainISOMoment(text)
	if err != nil || !moment.Equal(want) || !OperationSortKey(text).Equal(want) {
		t.Fatalf("source-valid %q rejected or wrong instant: %v, %v; want %v", text, moment, err, want)
	}
	if !(TimeFilter{}).Contains(text) {
		t.Fatal("source-valid date excluded by unbounded filter")
	}
	f, err := NewTimeFilter(text, text)
	if err != nil || f.From == nil || f.To == nil || !f.From.Equal(want) || !f.To.Equal(want) || !f.Contains(text) {
		t.Fatalf("inclusive source bounds lost: %+v, %v", f, err)
	}
	if text != before {
		t.Fatal("source text changed")
	}
	nativeFrom, nativeTo := *f.From, *f.To
	copyFilter := TimeFilter{From: &nativeFrom, To: &nativeTo}
	if !copyFilter.Contains(text) {
		t.Fatal("native copied bounds lost instant")
	}
	// Compare adjacent source microseconds using explicit native bounds: source
	// strings truncate, caller nanoseconds are not normalized or mutated.
	later := want.Add(time.Microsecond)
	earlier := want.Add(-time.Microsecond)
	if (TimeFilter{From: &later}).Contains(text) || (TimeFilter{To: &earlier}).Contains(text) {
		t.Fatal("noninclusive neighboring bound accepted")
	}
	f.RequestBounds()
	f.ISOBounds()
	if *f.From != nativeFrom || *f.To != nativeTo {
		t.Fatal("wire helper mutated bound")
	}
}
func cycle3DateRejected(t *testing.T, text string) {
	t.Helper()
	if !OperationSortKey(text).Equal(time.Time{}) || (TimeFilter{}).Contains(text) {
		t.Fatal("invalid source acquired instant/match")
	}
	for _, pair := range [][2]string{{text, ""}, {"", text}} {
		f, err := NewTimeFilter(pair[0], pair[1])
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Field() != "date" || f.From != nil || f.To != nil || strings.Contains(err.Error(), text) {
			t.Fatalf("invalid source partially published/unsafe error: %+v,%v", f, err)
		}
	}
}
func TestReviewCycle3BasicCalendarClock(t *testing.T) {
	want := time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC)
	for _, text := range []string{"20260801T12:30:00Z", "2026-08-01T123000Z", "20260801T123000Z", "20260801T1230Z", "20260801T12Z", "20260801T123Z", "20260801T12300Z"} {
		t.Run(text, func(t *testing.T) {
			expected := want
			if strings.HasSuffix(text, "T12Z") || strings.HasSuffix(text, "T123Z") {
				expected = want.Add(-30 * time.Minute)
			}
			cycle3DateAccepted(t, text, expected)
		})
	}
	for _, text := range []string{"2026081T123000Z", "20260230T123000Z", "00000801T123000Z", "20260801T1Z", "20260801T24Z", "2026-08-01T12:3000Z", "2026-08-01T1230:00Z"} {
		t.Run("invalid/"+text, func(t *testing.T) { cycle3DateRejected(t, text) })
	}
}

// The native instant cannot truthfully expose a fractional Zone() offset.
// Require a per-value exact civil/offset API, not rounded offset metadata or a
// process-global registry. Copies must retain all source precision.
func TestReviewCycle3SourceDateTimeExactValue(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(cycle3CanonicalTracerJSON))
	for dec.More() {
		var row cycle3Row
		if err := dec.Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.Family != "offset" {
			continue
		}
		t.Run(row.ID, func(t *testing.T) {
			source, err := ParseSourceDateTime(row.Text)
			if !row.Valid {
				if err == nil || source.Valid() || !source.Native().IsZero() || source.ISOFormat() != "" {
					t.Fatal("invalid source acquired metadata")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			before := source
			copy := source
			civil := copy.CivilTime()
			want := row.Key
			if !copy.Valid() || civil.Location() != time.UTC || civil.Year() != want.Year || int(civil.Month()) != want.Month || civil.Day() != want.Day || civil.Hour() != want.Hour || civil.Minute() != want.Minute || civil.Second() != want.Second || civil.Nanosecond()/1000 != want.Microsecond || copy.UTCOffset().Microseconds() != want.OffsetMicroseconds || copy.ISOFormat() != row.SourceISO {
				t.Fatalf("source metadata changed: %+v / %s; want %s", copy, copy.ISOFormat(), row.SourceISO)
			}
			native := copy.Native()
			if native.Unix() != want.UnixSecond || native.Nanosecond()/1000 != want.InstantMicrosecond || !native.Equal(OperationSortKey(row.Text)) {
				t.Fatal("exact source instant lost")
			}
			if want.OffsetMicroseconds%1000000 != 0 && native.Location() != time.UTC {
				t.Fatal("fractional offset was faked as an integral native zone")
			}
			if source != before || copy != source {
				t.Fatal("value copies lost metadata or getters mutated source")
			}
		})
	}
	var zero SourceDateTime
	if zero.Valid() || zero.ISOFormat() != "" || !zero.Native().IsZero() {
		t.Fatal("zero source value invented a valid date")
	}
}

// Additive source-wire API: preserve existing native RequestBounds/ISOBounds
// names/conventions, but require an explicit source-exact formatting path.
type cycle3SourceWire interface {
	SourceRequestBounds() (string, string, error)
	SourceISOBounds() (string, string, error)
}

func TestReviewCycle3CanonicalSourceWire(t *testing.T) {
	for _, tc := range []struct{ text, request, iso string }{
		{"1900-07-01T12:00:00", "01.07.1900T12:00:00", "1900-07-01T12:00:00+03:00"},
		{"0001-01-01T00:00:00", "01.01.1T00:00:00", "1-01-01T00:00:00+03:00"},
		{"0999-01-01T00:00:00", "01.01.999T00:00:00", "999-01-01T00:00:00+03:00"},
		{"2010-03-28T02:30:00", "28.03.2010T02:30:00", "2010-03-28T02:30:00+03:00"},
		{"2010-03-28T02:30:00+03:00", "28.03.2010T03:30:00", "2010-03-28T03:30:00+03:00"},
		{"2026-08-01T12:30:00+03:00:00.5", "01.08.2026T12:29:59", "2026-08-01T12:29:59+03:00"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			f, err := NewTimeFilter(tc.text, tc.text)
			if err != nil {
				t.Fatal(err)
			}
			wire, ok := any(f).(cycle3SourceWire)
			if !ok {
				t.Fatal("exact source wire helpers absent")
			}
			from, to, err := wire.SourceRequestBounds()
			if err != nil || from != tc.request || to != tc.request {
				t.Fatalf("source request mismatch: %q/%q,%v", from, to, err)
			}
			from, to, err = wire.SourceISOBounds()
			if err != nil || from != tc.iso || to != tc.iso {
				t.Fatalf("source ISO mismatch: %q/%q,%v", from, to, err)
			}
			copied := *f.From
			before := copied
			copyFilter := TimeFilter{From: &copied, To: &copied}
			copyWire := any(copyFilter).(cycle3SourceWire)
			from, to, err = copyWire.SourceRequestBounds()
			if err != nil || from != tc.request || to != tc.request || copied != before {
				t.Fatal("copied wall/zone metadata lost or mutated")
			}
		})
	}
}
func TestReviewCycle3SourceWireDirectNativeUnsetErrors(t *testing.T) {
	native := time.Date(2026, 8, 1, 12, 30, 0, 123456789, time.FixedZone("explicit native", 10800))
	before := native
	f := TimeFilter{From: &native, To: &native}
	wire, ok := any(f).(cycle3SourceWire)
	if !ok {
		t.Fatal("exact source wire helpers absent")
	}
	a, b, e := wire.SourceISOBounds()
	if e != nil || a != "2026-08-01T12:30:00+03:00" || b != a || native != before || f.Contains("2026-08-01T12:30:00.1234569+03:00") {
		t.Fatal("native nanoseconds normalized/rounded or bounds mutated")
	}
	unset := any(TimeFilter{}).(cycle3SourceWire)
	a, b, e = unset.SourceRequestBounds()
	if e != nil || a != "" || b != "" {
		t.Fatal("unset source helper invented a date")
	}
	a, b, e = unset.SourceISOBounds()
	if e != nil || a != "" || b != "" {
		t.Fatal("unset source helper invented ISO")
	}
	for _, native := range []time.Time{time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("UTC intermediate overflow", 1)), time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), time.Date(2026, 8, 1, 12, 0, 0, 0, time.FixedZone("private input", 86400))} {
		wire := any(TimeFilter{From: &native, To: &native}).(cycle3SourceWire)
		a, b, e = wire.SourceRequestBounds()
		var pe *ParseError
		if !errors.As(e, &pe) || pe.Field() != "date" || a != "" || b != "" || strings.Contains(e.Error(), "private input") {
			t.Fatal("native source-invalid/conversion failure published partial wire bounds")
		}
	}
}
