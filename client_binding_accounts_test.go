package sber

import (
	"context"
	"reflect"
	"testing"
)

func TestClientBindingAccountsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	api := clientBindingGetter[*AccountsAPI](t, c, "Accounts")
	if api != clientBindingGetter[*AccountsAPI](t, c, "Accounts") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	accounts, err := api.List(context.Background(), true)
	if err != nil || len(accounts) != 2 || accounts[0].Cards()[0].Account() != accounts[0] {
		t.Fatalf("bound accounts: %v", err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+ProductsPath || !reflect.DeepEqual(calls[0].body, resourceProductsCall(true).Payload) {
		t.Fatal("accounts shortcut changed source product read")
	}
}
