package sber

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func FuzzReviewCycle3SourceDatetimeContracts(f *testing.F) {
	var controls []cycle3Row
	dec := json.NewDecoder(strings.NewReader(cycle3CanonicalTracerJSON))
	for dec.More() {
		var row cycle3Row
		if err := dec.Decode(&row); err != nil {
			f.Fatal(err)
		}
		controls = append(controls, row)
	}
	for _, raw := range []string{"20260801T123000Z", "2026-W31-6T12.5Z", "2026-08-01🙂12:30:00+02:99", "2026-08-01T12:30:00+03:00:00.5", "2026-08-01T12:30:00+00:00:00.5", "0001-01-01T00:00:00+00:00:01.5", "2010-03-28T02:30:00.1234569", "2026-08-01T1:30:00Z", "2026-08-01T12:30:00Z\x00ignored", ""} {
		f.Add(raw, uint16(0))
	}
	// Parent source-contract controls, not a production RED: retain the
	// immutable worker failure and exercise DATE/DATETIME independence.
	f.Add("0001010100", uint16(0))
	f.Add("2024W09200", uint16(0))
	f.Fuzz(func(t *testing.T, raw string, selector uint16) {
		source, err := ParseSourceDateTime(raw)
		again, other := ParseSourceDateTime(raw)
		// Native time.Time locations may be newly allocated fixed zones; their
		// pointer identity is not source datetime value equality.
		if (err == nil) != (other == nil) || source.Valid() != again.Valid() || source.CivilTime() != again.CivilTime() || !source.Native().Equal(again.Native()) || source.UTCOffset() != again.UTCOffset() || source.DateOnly() != again.DateOnly() || source.ISOFormat() != again.ISOFormat() {
			t.Fatal("parsing is nondeterministic")
		}
		key := OperationSortKey(raw)
		// DATE and DATETIME are independent canonical grammars. DATE-first
		// constructors may accept a spelling rejected by DATETIME, and may
		// use a different calendar day even when both scanners accept it.
		day, dateErr := ParseSourceDate(raw)
		g, boundsErr := NewTimeFilter(raw, raw)
		if err != nil {
			if !key.IsZero() || (TimeFilter{}).Contains(raw) {
				t.Fatal("invalid raw source acquired key/match")
			}
			if raw != "" && dateErr != nil && (boundsErr == nil || g.From != nil || g.To != nil) {
				t.Fatal("input rejected by both source grammars published bounds")
			}
		} else {
			civil := source.CivilTime()
			if civil.Year() < 1 || civil.Year() > 9999 || civil.Nanosecond()%1000 != 0 || source.UTCOffset() <= -24*time.Hour || source.UTCOffset() >= 24*time.Hour || !key.Equal(source.Native()) || !(TimeFilter{}).Contains(raw) {
				t.Fatal("accepted source broke civil range/precision/true instant")
			}
			if dateErr != nil && boundsErr == nil && (!g.Contains(raw) || g.From == nil || g.To == nil) {
				t.Fatal("successful DATETIME bounds are unusable")
			}
		}
		if dateErr == nil {
			if boundsErr != nil || g.From == nil || g.To == nil {
				t.Fatal("accepted independent DATE lost its complete bounds")
			}
			// Check the inferred DATE's civil window directly, not membership
			// of the possibly different independently parsed DATETIME.
			datePrefix := day.Format("02.01.") + strconv.Itoa(day.Year())
			from, to, wireErr := g.SourceRequestBounds()
			if wireErr != nil || from != datePrefix+"T00:00:00" || to != datePrefix+"T23:59:59" {
				t.Fatal("DATE-first constructor changed canonical civil bounds")
			}
		}
		// Independent generated clock-width negative, not a permissive native
		// parse/format round trip. Existing source one-digit rules stay intact.
		bad := "2026-08-01T" + strconv.Itoa(int(selector%10)) + ":30:00Z"
		if _, e := ParseSourceDateTime(bad); e == nil || (TimeFilter{}).Contains(bad) {
			t.Fatal("one-digit source-invalid clock accepted")
		}
		row := controls[int(selector)%len(controls)]
		value, e := ParseSourceDateTime(row.Text)
		if (e == nil) != row.Valid {
			t.Fatal("frozen canonical grammar control changed")
		}
		if row.Valid && (value.Native().Unix() != row.Key.UnixSecond || value.Native().Nanosecond()/1000 != row.Key.InstantMicrosecond || value.UTCOffset().Microseconds() != row.Key.OffsetMicroseconds || value.ISOFormat() != row.SourceISO) {
			t.Fatal("frozen canonical exact offset/instant changed")
		}
	})
}
