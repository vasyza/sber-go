package sber

import (
	"context"
	"reflect"
	"testing"
)

func TestClientBindingOperationsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourceOperationsResponse("native-op"))
	api := clientBindingGetter[*OperationsAPI](t, c, "Operations")
	if api != clientBindingGetter[*OperationsAPI](t, c, "Operations") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	page, err := api.Page(context.Background(), OperationsPageOptions{Limit: 2, Resource: "card:4004", From: "2026-07-01", To: "2026-07-31"})
	if err != nil || len(page.Operations) != 1 || page.Operations[0].ID != "native-op" || page.Operations[0].ScopeCardIDs[0] != "4004" {
		t.Fatalf("typed operations: %v", err)
	}
	calls := tr.snapshotCalls()
	want := resourceHistoryCall(0, 3, "card:4004", "01.07.2026T00:00:00", "31.07.2026T23:59:59")
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+OperationsPath || !reflect.DeepEqual(calls[0].body, want.Payload) {
		t.Fatal("source operation request changed")
	}
}
