package bank

import (
	"context"
	"errors"
	"testing"
)

func TestClientBindingTransfersFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, nil)
	api := clientBindingGetter[*TransfersAPI](t, c, "Transfers")
	if api != clientBindingGetter[*TransfersAPI](t, c, "Transfers") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	if _, err := api.Start(context.Background()); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatalf("default mutation policy: %v", err)
	}
	if n, _, _ := tr.counts(); n != 0 {
		t.Fatal("default workflow contacted transport")
	}
}
