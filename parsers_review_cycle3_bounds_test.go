package sber

import (
	"encoding/json"
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
