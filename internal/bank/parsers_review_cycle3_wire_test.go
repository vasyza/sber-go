package bank

import (
	"errors"
	"strings"
	"testing"
	"time"
)

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
