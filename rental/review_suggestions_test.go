package rental_test

import (
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

// Test-only strengthening of two independently exercised review boundaries.
// All ledgers here are SYNTHETIC. Production behavior is unchanged.
func TestObservedAtMustCoverThroughEvenWhenFreshAtAsOf(t *testing.T) {
	for _, tc := range []struct {
		name           string
		observedOffset time.Duration
	}{
		{"observed-equals-as-of", 0},
		{"observed-after-as-of-before-through", time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticInput()
			in.Evidence = completeEvidence(in)
			in.Evidence[0].CoverageThrough = in.AsOf.Add(2 * time.Second)
			in.Evidence[0].ObservedAt = in.AsOf.Add(tc.observedOffset)
			got, err := rental.Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Periods()[0].State != rental.Unknown || !hasReason(got.Periods()[0], "EVIDENCE_STALE") || len(got.Candidates()) != 0 {
				t.Fatalf("observation before claimed coverage must block debt even when fresh at AsOf: periods=%+v candidates=%+v", got.Periods(), got.Candidates())
			}

			r := syntheticReceipt()
			r.Amount = in.Periods[0].Price
			in.Receipts = []rental.Receipt{r}
			paid, err := rental.Evaluate(in)
			if err != nil || paid.Periods()[0].State != rental.Paid || len(paid.Candidates()) != 0 {
				t.Fatalf("stale coverage cannot undo confirmed full funding: err=%v periods=%+v", err, paid.Periods())
			}

			in.Receipts = nil
			in.Evidence[0].ObservedAt = in.Evidence[0].CoverageThrough
			atBoundary, err := rental.Evaluate(in)
			if err != nil || atBoundary.Periods()[0].State != rental.Due || len(atBoundary.Candidates()) != 1 {
				t.Fatalf("observation exactly at coverage end must satisfy this gate: err=%v periods=%+v", err, atBoundary.Periods())
			}
		})
	}
}

func TestExplicitAdvanceDueDateUsesDueAtNotPeriodStart(t *testing.T) {
	for _, tc := range []struct {
		name       string
		offset     time.Duration
		state      rental.State
		candidates int
	}{
		{"before-due", -time.Nanosecond, rental.NotDue, 0},
		{"at-due", 0, rental.Due, 1},
		{"after-due-before-start", time.Nanosecond, rental.Due, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticInput()
			in.Periods[0].Start = instant("2026-02-01T00:00:00Z")
			in.Periods[0].End = instant("2026-03-01T00:00:00Z")
			in.Periods[0].DueAt = instant("2026-01-20T00:00:00Z")
			in.AsOf = in.Periods[0].DueAt.Add(tc.offset)
			in.Evidence = completeEvidence(in)
			got, err := rental.Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			p := got.Periods()[0]
			if p.State != tc.state || len(got.Candidates()) != tc.candidates {
				t.Fatalf("independent advance due date must govern eligibility, not period start: periods=%+v candidates=%+v", got.Periods(), got.Candidates())
			}
			if p.Period != in.Periods[0] {
				t.Fatal("due decision changed an explicit contractual anchor")
			}
			if tc.candidates != 0 && !got.Candidates()[0].DueAt.Equal(in.Periods[0].DueAt) {
				t.Fatal("candidate lost independent explicit due date")
			}
		})
	}
}
