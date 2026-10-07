package sber

import (
	"context"
	"testing"
)

func TestClientBindingSessionFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, nil)
	api := clientBindingGetter[*SessionAPI](t, c, "Session")
	if api != clientBindingGetter[*SessionAPI](t, c, "Session") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	exported, err := api.Export()
	if err != nil || exported.APIBase != c.core().bundle.APIBase {
		t.Fatalf("session export: %v", err)
	}
	exported.Cookies[0].Value = "caller-edit"
	creds, err := api.Credentials()
	if err != nil || creds.UFSSession != "session-binding-access" {
		t.Fatalf("session credentials snapshot: %v", err)
	}
	if n, _, _ := tr.counts(); n != 0 {
		t.Fatal("in-memory export made a request")
	}
	if err := api.WarmUp(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.WebBase+clientWarmUpPath {
		t.Fatal("warmup did not use owned transport")
	}
}
