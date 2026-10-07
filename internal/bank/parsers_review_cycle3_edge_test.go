package bank

import (
	"testing"
	"time"
)

// Newly exposed by the full, unfiltered canonical differential, not inferred
// from a native parse/format round trip. The source accepts Z followed by a NUL
// and ignores its remaining text; a single newline is also a date separator.
func TestReviewCycle3CanonicalNULAndBasicLineSeparator(t *testing.T) {
	want := time.Date(2026, 8, 1, 12, 30, 0, 0, time.UTC)
	for _, text := range []string{"2026-08-01T12:30:00Z\x00ignored", "20260801\n12:30:00Z"} {
		t.Run(text, func(t *testing.T) { cycle3DateAccepted(t, text, want) })
	}
}
