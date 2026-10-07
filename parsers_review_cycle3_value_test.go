package sber

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

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
