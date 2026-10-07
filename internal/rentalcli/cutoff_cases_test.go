package rentalcli

import (
	"slices"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

type cutoffCase struct {
	name          string
	change        func(map[string]any)
	state         rental.State
	candidates    int
	allocations   int
	reason        rental.Reason
	futureReceipt bool
}

func syntheticCutoffCases() []cutoffCase {
	set := func(collection, key, value string) func(map[string]any) {
		return func(document map[string]any) { firstObject(document, collection)[key] = value }
	}
	coverage := func(asOf, through, observed string) func(map[string]any) {
		return func(document map[string]any) {
			document["AsOf"] = asOf
			proof := firstObject(document, "Evidence")
			proof["CoverageThrough"], proof["ObservedAt"] = through, observed
		}
	}
	return []cutoffCase{
		{name: "due-one-nanosecond-earlier", change: set("Periods", "DueAt", "2026-02-28T23:59:59.999999999Z"), state: rental.Due, candidates: 1},
		{name: "due-exactly-asof", change: set("Periods", "DueAt", "2026-03-01T00:00:00Z"), state: rental.Due, candidates: 1},
		{name: "due-one-nanosecond-later", change: set("Periods", "DueAt", "2026-03-01T00:00:00.000000001Z"), state: rental.NotDue},
		{name: "due-same-instant-offset-and-trailing-zeroes", change: set("Periods", "DueAt", "2026-03-01T03:00:00.000000000+03:00"), state: rental.Due, candidates: 1},
		{name: "receipt-one-nanosecond-earlier", change: set("Receipts", "ReceivedAt", "2026-02-28T23:59:59.999999999Z"), state: rental.Paid, allocations: 1},
		{name: "receipt-exactly-asof", change: set("Receipts", "ReceivedAt", "2026-03-01T00:00:00Z"), state: rental.Paid, allocations: 1},
		{name: "receipt-one-nanosecond-later", change: set("Receipts", "ReceivedAt", "2026-03-01T00:00:00.000000001Z"), state: rental.Due, candidates: 1, futureReceipt: true},
		{name: "receipt-same-instant-offset", change: set("Receipts", "ReceivedAt", "2026-02-28T19:00:00.000000000-05:00"), state: rental.Paid, allocations: 1},
		{name: "coverage-one-nanosecond-short", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000001Z", "2026-03-01T00:00:00.000000002Z"), state: rental.Unknown, reason: rental.EvidenceCoverageGap},
		{name: "coverage-exactly-asof", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000002Z"), state: rental.Due, candidates: 1},
		{name: "coverage-after-asof-fresh-through-coverage", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000003Z", "2026-03-01T00:00:00.000000003Z"), state: rental.Due, candidates: 1},
		{name: "coverage-start-one-nanosecond-late", change: set("Evidence", "CoverageStart", "2026-01-01T00:00:00.000000001Z"), state: rental.Unknown, reason: rental.EvidenceCoverageGap},
		{name: "observation-before-asof", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000001Z"), state: rental.Unknown, reason: rental.EvidenceStale},
		{name: "observation-at-asof-before-coverage", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000003Z", "2026-03-01T00:00:00.000000002Z"), state: rental.Unknown, reason: rental.EvidenceStale},
		{name: "observation-after-asof-before-coverage", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T00:00:00.000000004Z", "2026-03-01T00:00:00.000000003Z"), state: rental.Unknown, reason: rental.EvidenceStale},
		{name: "coverage-and-observation-equivalent-offsets", change: coverage("2026-03-01T00:00:00.000000002Z", "2026-03-01T03:00:00.000000002+03:00", "2026-02-28T19:00:00.000000002-05:00"), state: rental.Due, candidates: 1},
	}
}

func cutoffDocument(t testing.TB, test cutoffCase) map[string]any {
	t.Helper()
	document := syntheticProofDocument(t)
	if !test.futureReceipt && test.allocations == 0 {
		document["Receipts"] = nil
	}
	test.change(document)
	return document
}

func assertCutoffDecision(t testing.TB, test cutoffCase, document map[string]any, result previewDecisions) {
	t.Helper()
	assertDecision(t, result, test.state, test.candidates, test.allocations)
	if test.reason != "" && !slices.Contains(result.Periods[0].Reasons, test.reason) {
		t.Fatalf("missing exact proof blocker %s: %+v", test.reason, result.Periods[0])
	}
	remaining := int64(250000)
	if test.state == rental.Paid {
		remaining = 0
	}
	if result.Periods[0].Remaining.Minor != remaining || len(result.Totals) != 1 || result.Totals[0].RemainingMinor != remaining {
		t.Fatal("cutoff changed exact money")
	}
	if test.futureReceipt {
		if len(result.Review) != 1 || !slices.Contains(result.Review[0].Reasons, rental.ReceiptAfterAsOf) || result.Totals[0].ReceivedMinor != 0 {
			t.Fatal("future receipt was allocated, lost or not retained for review")
		}
		want, err := time.Parse(time.RFC3339Nano, firstObject(document, "Receipts")["ReceivedAt"].(string))
		if err != nil || !result.Review[0].ReceivedAt.Equal(want) {
			t.Fatal("future receipt instant changed")
		}
	}
	wantAsOf, err := time.Parse(time.RFC3339Nano, document["AsOf"].(string))
	if err != nil {
		t.Fatal(err)
	}
	gotAsOf, err := time.Parse(time.RFC3339Nano, result.AsOf)
	if err != nil || !gotAsOf.Equal(wantAsOf) {
		t.Fatal("reference instant changed")
	}
	wantDue, err := time.Parse(time.RFC3339Nano, firstObject(document, "Periods")["DueAt"].(string))
	if err != nil || !result.Periods[0].Period.DueAt.Equal(wantDue) {
		t.Fatal("independent contractual due instant changed")
	}
}

func TestPreviewPreservesExactCutoffDecisions(t *testing.T) {
	for _, test := range syntheticCutoffCases() {
		t.Run(test.name, func(t *testing.T) {
			document := cutoffDocument(t, test)
			assertCutoffDecision(t, test, document, previewDocument(t, document))
		})
	}
}
