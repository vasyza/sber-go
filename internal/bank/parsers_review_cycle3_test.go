package bank

import (
	"errors"
	"strings"
	"testing"
	"time"
)

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
