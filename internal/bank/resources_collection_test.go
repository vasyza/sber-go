package bank

import (
	"context"
	"errors"
	"testing"
)

func TestResourceCollectionExplicitEmptyWindowIsNotBankCompletenessProof(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 31, "card:4004", "01.07.2026T00:00:00", "31.07.2026T23:59:59"), Response: resourceOperationsResponse()}}}
	got, err := NewOperationsAPI(r).Collect(context.Background(), OperationsQuery{Resource: "card:4004", Limit: 30, MaxPages: 100, From: "2026-07-01", To: "2026-07-31"})
	m := got.Metadata
	if err != nil || len(got.Operations) != 0 || !m.PaginationExhausted || m.BankCapProven || m.WindowCompleteness != "unknown" || !m.ExplicitFrom || !m.ExplicitTo || m.DefaultWindow || m.PagesRead != 1 || m.RequestedFrom != "01.07.2026T00:00:00" || m.RequestedTo != "31.07.2026T23:59:59" {
		t.Fatalf("false completeness: %#v %v", m, err)
	}
	r.done()
}
func TestResourceCollectionCapKeepsExplicitPartialAndUnknownCoverage(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1", "sentinel")}}}
	got, err := NewOperationsAPI(r).Collect(context.Background(), OperationsQuery{Limit: 1, MaxPages: 1, From: "2026-07-01"})
	var capErr *PaginationLimitError
	if !errors.As(err, &capErr) || len(got.Operations) != 1 || got.Metadata.PaginationExhausted || !got.Metadata.ClientCapReached || got.Metadata.WindowCompleteness != "unknown" {
		t.Fatal("cap reported complete")
	}
	r.done()
}
