package rental_test

import (
	"github.com/vasyza/sber-go/rental"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestReceiptDedupReviewIsDeterministicAcrossOrderingAndEquivalentInstants(t *testing.T) {
	in := syntheticThreeInput()
	unresolved := syntheticReceipt()
	unresolved.ID = "synthetic-unresolved"
	unresolved.TenantID = ""
	unresolved.PossibleTenantIDs = []string{"synthetic-moto-Anna-M", "synthetic-moto-Anna"}
	unresolved.Confirmed = false
	same := unresolved
	same.ReceivedAt = instant("2026-01-10T03:00:00+03:00")
	knownA := syntheticReceipt()
	knownA.ID = "synthetic-a"
	knownA.Amount.Minor = 80001
	knownB := knownA
	knownB.ID = "synthetic-b"
	knownB.Amount.Minor = 2
	in.Receipts = []rental.Receipt{unresolved, same, knownB, knownA}
	baseline, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	reversed := cloneInput(in)
	slices.Reverse(reversed.Tenants)
	slices.Reverse(reversed.Periods)
	slices.Reverse(reversed.Receipts)
	slices.Reverse(reversed.Evidence)
	for i := range reversed.Receipts {
		slices.Reverse(reversed.Receipts[i].PossibleTenantIDs)
	}
	before := cloneInput(reversed)
	got, err := rental.Evaluate(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, reversed) {
		t.Fatal("canonical ordering mutated input")
	}
	if encodedView(t, got) != encodedView(t, baseline) {
		t.Fatal("same semantic receipts yielded different reports because caller order or timestamp representation changed")
	}
	if len(got.Review()) != 1 || got.Review()[0].ReceivedAt.Location().String() != "UTC" {
		t.Fatal("deduplicated receipt-review instants must be canonical UTC, independently of first duplicate")
	}
}

// All test inputs, tenant IDs and prices in this package are SYNTHETIC.
// They are not owner records, contractual dates or actual tenant mappings.
func instant(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func syntheticInput() rental.Input {
	return rental.Input{
		AsOf:    instant("2026-03-10T00:00:00Z"),
		Tenants: []rental.Tenant{{ID: "synthetic-parking", Currency: "RUB", LedgerStart: instant("2026-01-01T00:00:00Z")}},
		Periods: []rental.Period{{
			ID: "synthetic-jan", TenantID: "synthetic-parking",
			Start: instant("2026-01-01T00:00:00Z"), End: instant("2026-02-01T00:00:00Z"),
			DueAt: instant("2026-01-05T00:00:00Z"), Price: rental.Money{Minor: 250000, Currency: "RUB"},
		}},
	}
}

func TestConflictingGlobalReceiptIdentityFailsClosed(t *testing.T) {
	mutations := map[string]func(*rental.Receipt){
		"amount":       func(r *rental.Receipt) { r.Amount.Minor++ },
		"currency":     func(r *rental.Receipt) { r.Amount.Currency = "USD" },
		"tenant":       func(r *rental.Receipt) { r.TenantID = "synthetic-motorcycle" },
		"confirmation": func(r *rental.Receipt) { r.Confirmed = false },
		"timestamp":    func(r *rental.Receipt) { r.ReceivedAt = r.ReceivedAt.Add(time.Second) },
		"method":       func(r *rental.Receipt) { r.Method = rental.Transfer },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			in := syntheticInput()
			r := rental.Receipt{ID: "synthetic-global", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-02T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 80001, Currency: "RUB"}}
			conflict := r
			mutate(&conflict)
			in.Receipts = []rental.Receipt{r, conflict}
			got, err := rental.Evaluate(in)
			if err == nil || !reflect.DeepEqual(got, rental.Evaluation{}) {
				t.Fatalf("conflicting stable receipt ID must return error and no partial decisions: err=%v output=%+v", err, got)
			}
		})
	}
}

func TestIdenticalGlobalReceiptIDCountsOnceAcrossTimestampRepresentations(t *testing.T) {
	in := syntheticInput()
	r := rental.Receipt{ID: "synthetic-shared-id", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-02T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 80001, Currency: "RUB"}}
	same := r
	same.ReceivedAt = instant("2026-01-02T03:00:00+03:00") // Same instant, not a new receipt.
	in.Receipts = []rental.Receipt{r, same, r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Periods()[0].Applied.Minor != 80001 || got.Totals()[0].ReceivedMinor != 80001 || len(got.Allocations()) != 1 {
		t.Fatalf("stable receipt identity was counted more than once: periods=%+v totals=%+v allocations=%+v", got.Periods(), got.Totals(), got.Allocations())
	}
}

func TestExcessCreditIsExactWithoutInventingFuturePeriods(t *testing.T) {
	in := syntheticInput()
	in.Receipts = []rental.Receipt{{ID: "synthetic-excess", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-02T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 250001, Currency: "RUB"}}}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	credits := got.Credits()
	if len(credits) != 1 || credits[0].ReceiptID != "synthetic-excess" || credits[0].TenantID != "synthetic-parking" || credits[0].Amount.Minor != 1 || credits[0].Amount.Currency != "RUB" {
		t.Fatalf("excess minor unit must remain explicit credit: %+v", credits)
	}
	totals := got.Totals()
	if len(totals) != 1 || totals[0].ReceivedMinor != 250001 || totals[0].AppliedMinor != 250000 || totals[0].CreditMinor != 1 || totals[0].ObligationMinor != 250000 || totals[0].RemainingMinor != 0 {
		t.Fatalf("conservation totals must include excess exactly once: %+v", totals)
	}
	if len(got.Periods()) != 1 {
		t.Fatal("credit invented a period absent from the owner's input")
	}
}

func TestSeveralReceiptsApplyChronologicallyWithExactPartialRemainder(t *testing.T) {
	in := syntheticInput()
	in.Receipts = []rental.Receipt{
		{ID: "synthetic-later", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-02-02T00:00:00Z"), Method: rental.Transfer, Amount: rental.Money{Minor: 49998, Currency: "RUB"}},
		{ID: "synthetic-earlier", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-20T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 80001, Currency: "RUB"}},
	}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Periods()[0]
	if p.Applied.Minor != 129999 || p.Remaining.Minor != 120001 || p.State != rental.Unknown {
		t.Fatalf("partial exact receipts without evidence must retain UNKNOWN balance: %+v", p)
	}
	a := got.Allocations()
	if len(a) != 2 || a[0].ReceiptID != "synthetic-earlier" || a[1].ReceiptID != "synthetic-later" {
		t.Fatalf("receipt chronology must be deterministic, not caller slice order: %+v", a)
	}
}

func TestSyntheticCash5000PrepaysTwoExplicit2500PeriodsOldestFirst(t *testing.T) {
	in := syntheticInput()
	in.AsOf = instant("2026-01-10T00:00:00Z")
	feb := in.Periods[0]
	feb.ID = "synthetic-feb"
	feb.Start = instant("2026-02-01T00:00:00Z")
	feb.End = instant("2026-03-01T00:00:00Z")
	feb.DueAt = instant("2026-02-05T00:00:00Z")
	in.Periods = []rental.Period{feb, in.Periods[0]} // Deliberately unordered.
	in.Receipts = []rental.Receipt{{
		ID: "synthetic-cash-5000", TenantID: "synthetic-parking", Confirmed: true,
		ReceivedAt: instant("2026-01-06T00:00:00Z"), Method: rental.Cash,
		Amount: rental.Money{Minor: 500000, Currency: "RUB"},
	}}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	allocations := got.Allocations()
	if len(allocations) != 2 || allocations[0].PeriodID != "synthetic-jan" || allocations[1].PeriodID != "synthetic-feb" {
		t.Fatalf("oldest explicit obligation must receive cash first: %+v", allocations)
	}
	for _, p := range got.Periods() {
		if p.State != rental.Paid || p.Applied.Minor != 250000 || p.Remaining.Minor != 0 {
			t.Fatalf("SYNTHETIC 5000 cash must cover both 2500 periods exactly: %+v", p)
		}
	}
	if got.Periods()[1].Period.Start != feb.Start || got.Periods()[1].Period.DueAt != feb.DueAt {
		t.Fatal("prepayment changed explicit future anchor")
	}
}

func TestConfirmedReceiptPaysExplicitPeriodWithoutCompleteHistory(t *testing.T) {
	in := syntheticInput()
	in.Receipts = []rental.Receipt{{
		ID: "synthetic-cash-1", TenantID: "synthetic-parking", Confirmed: true,
		ReceivedAt: instant("2026-02-20T00:00:00Z"), Method: rental.Cash,
		Amount: rental.Money{Minor: 250000, Currency: "RUB"},
	}}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	periods := got.Periods()
	if len(periods) != 1 || periods[0].State != rental.Paid || periods[0].Remaining.Minor != 0 || periods[0].Applied.Minor != 250000 {
		t.Fatalf("confirmed exact receipt must prove PAID: %+v", periods)
	}
	allocations := got.Allocations()
	if len(allocations) != 1 || allocations[0].ReceiptID != "synthetic-cash-1" || allocations[0].PeriodID != "synthetic-jan" || allocations[0].Amount.Minor != 250000 {
		t.Fatalf("missing exact allocation: %+v", allocations)
	}
	if periods[0].Period.Start != in.Periods[0].Start || periods[0].Period.DueAt != in.Periods[0].DueAt {
		t.Fatal("late receipt shifted a contractual anchor or due date")
	}
}

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
