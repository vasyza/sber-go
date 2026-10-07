package bank

import (
	"context"
	"testing"
)

// Each expected wire string is checked against the frozen, actually executed
// Python3.12 source corpus before this tracer runs. No runtime oracle is used.
func TestResourceCycle3PageUsesCanonicalSourceRequestYear(t *testing.T) {
	input := "0001-01-01T00:00:00Z"
	want := "01.01.1T02:30:17"
	requester := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "card:fixture", want, want), Response: resourceOperationsResponse()}}}
	result, err := NewOperationsAPI(requester).Page(context.Background(), OperationsPageOptions{Resource: "card:fixture", Limit: 1, From: input, To: input})
	if err != nil || len(result.Operations) != 0 || result.NextOffset != nil {
		t.Fatalf("canonical source page failed: %v", err)
	}
	requester.done()
}

func TestResourceCycle3CollectionMetadataUsesSameSourceRequestYear(t *testing.T) {
	input := "0001-01-01T00:00:00Z"
	want := "01.01.1T02:30:17"
	requester := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "card:fixture", want, want), Response: resourceOperationsResponse()}}}
	result, err := NewOperationsAPI(requester).Collect(context.Background(), OperationsQuery{Resource: "card:fixture", Limit: 1, MaxPages: 1, From: input, To: input})
	if err != nil {
		t.Fatal(err)
	}
	requester.done()
	m := result.Metadata
	if m.RequestedFrom != want || m.RequestedTo != want {
		t.Fatalf("history evidence describes different request bounds: %q/%q", m.RequestedFrom, m.RequestedTo)
	}
	if !m.ExplicitFrom || !m.ExplicitTo || m.DefaultWindow || m.WindowCompleteness != "unknown" || m.BankCapProven || !m.PaginationExhausted {
		t.Fatal("source wire adoption weakened history uncertainty")
	}
}
