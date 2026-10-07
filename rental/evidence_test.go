package rental_test

import (
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

func completeEvidence(in rental.Input) []rental.CollectionEvidence {
	var evidence []rental.CollectionEvidence
	for _, tenant := range in.Tenants {
		evidence = append(evidence, rental.CollectionEvidence{
			TenantID: tenant.ID, CoverageStart: tenant.LedgerStart, CoverageThrough: in.AsOf, ObservedAt: in.AsOf,
			Complete: true, OwnerReconciled: true,
		})
	}
	return evidence
}

func hasReason(p rental.PeriodResult, want string) bool {
	for _, reason := range p.Reasons {
		if string(reason) == want {
			return true
		}
	}
	return false
}

func TestContradictoryEvidenceIdentitiesAndBoundariesFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*rental.Input)
	}{
		{"missing-tenant", func(in *rental.Input) { in.Evidence[0].TenantID = "" }},
		{"unknown-tenant", func(in *rental.Input) { in.Evidence[0].TenantID = "synthetic-unknown" }},
		{"duplicate-evidence", func(in *rental.Input) { in.Evidence = append(in.Evidence, in.Evidence[0]) }},
		{"conflicting-evidence", func(in *rental.Input) { e := in.Evidence[0]; e.Complete = false; in.Evidence = append(in.Evidence, e) }},
		{"reversed-coverage", func(in *rental.Input) { in.Evidence[0].CoverageStart = in.Evidence[0].CoverageThrough.Add(time.Second) }},
		{"unsafe-evidence-time", func(in *rental.Input) { in.Evidence[0].ObservedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticInput()
			in.Evidence = completeEvidence(in)
			tc.mutate(&in)
			assertFailClosed(t, in)
		})
	}
}

func TestEveryIncompleteEvidenceGateBlocksDebtButCannotUndoConfirmedPaid(t *testing.T) {
	cases := []struct {
		name   string
		reason string
		mutate func(*rental.Input)
	}{
		{"absent", "MISSING_EVIDENCE", func(in *rental.Input) { in.Evidence = nil }},
		{"zero-value", "EVIDENCE_INCOMPLETE", func(in *rental.Input) { in.Evidence[0] = rental.CollectionEvidence{TenantID: in.Tenants[0].ID} }},
		{"incomplete", "EVIDENCE_INCOMPLETE", func(in *rental.Input) { in.Evidence[0].Complete = false }},
		{"unreconciled", "OWNER_RECONCILIATION_MISSING", func(in *rental.Input) { in.Evidence[0].OwnerReconciled = false }},
		{"missing-from", "EVIDENCE_BOUNDARIES_MISSING", func(in *rental.Input) { in.Evidence[0].CoverageStart = time.Time{} }},
		{"missing-through", "EVIDENCE_BOUNDARIES_MISSING", func(in *rental.Input) { in.Evidence[0].CoverageThrough = time.Time{} }},
		{"missing-observed", "EVIDENCE_BOUNDARIES_MISSING", func(in *rental.Input) { in.Evidence[0].ObservedAt = time.Time{} }},
		{"start-gap", "EVIDENCE_COVERAGE_GAP", func(in *rental.Input) { in.Evidence[0].CoverageStart = in.Tenants[0].LedgerStart.Add(time.Second) }},
		{"end-gap", "EVIDENCE_COVERAGE_GAP", func(in *rental.Input) { in.Evidence[0].CoverageThrough = in.AsOf.Add(-time.Second) }},
		{"stale", "EVIDENCE_STALE", func(in *rental.Input) { in.Evidence[0].ObservedAt = in.AsOf.Add(-time.Second) }},
		{"gap", "EVIDENCE_HAS_GAPS", func(in *rental.Input) { in.Evidence[0].HasGaps = true }},
		{"truncation", "EVIDENCE_TRUNCATED", func(in *rental.Input) { in.Evidence[0].Truncated = true }},
		{"pagination", "EVIDENCE_PAGE_UNCERTAIN", func(in *rental.Input) { in.Evidence[0].PageUncertain = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticInput()
			in.Evidence = completeEvidence(in)
			tc.mutate(&in)
			got, err := rental.Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			if got.Periods()[0].State != rental.Unknown || len(got.Candidates()) != 0 || !hasReason(got.Periods()[0], tc.reason) {
				t.Fatalf("unsafe evidence must give UNKNOWN, reason %q and no candidates: periods=%+v candidates=%+v", tc.reason, got.Periods(), got.Candidates())
			}
			r := syntheticReceipt()
			r.Amount.Minor = in.Periods[0].Price.Minor
			in.Receipts = []rental.Receipt{r}
			paid, err := rental.Evaluate(in)
			if err != nil || paid.Periods()[0].State != rental.Paid || len(paid.Candidates()) != 0 {
				t.Fatalf("known positive funds must still prove PAID despite uncertain extra history: err=%v output=%+v", err, paid)
			}
		})
	}
}

func TestOwnerReconciledCompleteLedgerAllowsOnlyDueRemainderCandidates(t *testing.T) {
	in := syntheticInput()
	in.AsOf = instant("2026-01-10T00:00:00Z")
	future := in.Periods[0]
	future.ID = "synthetic-future"
	future.Start = instant("2026-02-01T00:00:00Z")
	future.End = instant("2026-03-01T00:00:00Z")
	future.DueAt = instant("2026-02-05T00:00:00Z")
	in.Periods = append(in.Periods, future)
	in.Evidence = completeEvidence(in)
	r := syntheticReceipt()
	r.Amount.Minor = 30000
	in.Receipts = []rental.Receipt{r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	periods := got.Periods()
	if periods[0].State != rental.Due || periods[0].Remaining.Minor != 220000 || periods[1].State != rental.NotDue {
		t.Fatalf("complete, reconciled ledger must distinguish due and not due: %+v", periods)
	}
	candidates := got.Candidates()
	if len(candidates) != 1 || candidates[0].TenantID != "synthetic-parking" || candidates[0].PeriodID != "synthetic-jan" || candidates[0].Remaining.Minor != 220000 || !candidates[0].DueAt.Equal(in.Periods[0].DueAt) {
		t.Fatalf("only explicitly due remainder gets metadata candidate: %+v", candidates)
	}
	if !got.AsOf().Equal(in.AsOf) {
		t.Fatal("evaluation did not retain explicit AsOf")
	}
}
