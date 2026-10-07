package sber

import (
	"context"
	"reflect"
	"testing"
)

func TestClientBindingCardsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourceCardInfoResponse())
	api := clientBindingGetter[*CardsAPI](t, c, "Cards")
	if api != clientBindingGetter[*CardsAPI](t, c, "Cards") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	cards, err := api.Info(context.Background(), "12345")
	if err != nil || len(cards) != 1 || cards[0].Limits.Available.Amount.String() != "84250.17" {
		t.Fatalf("native card info: %v", err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+CardInfoPath || !reflect.DeepEqual(calls[0].body, map[string]any{"cardIds": []string{"12345"}}) {
		t.Fatal("card info source request changed")
	}
}
