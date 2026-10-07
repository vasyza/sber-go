package sber

import (
	"context"
	"reflect"
	"testing"
)

type clientBindingPortfolioAPI interface {
	Portfolio(context.Context, bool) (*BankPortfolio, error)
}

func clientBindingPortfolio(t *testing.T, c *SberClient, force bool) *BankPortfolio {
	t.Helper()
	api, ok := any(c).(clientBindingPortfolioAPI)
	if !ok {
		t.Fatal("missing concrete SberClient.Portfolio(ctx, bool) (*BankPortfolio, error)")
	}
	p, err := api.Portfolio(context.Background(), force)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func clientBindingCheckSnapshot(t *testing.T, c *SberClient, p *BankPortfolio) {
	t.Helper()
	accounts, cards := p.Accounts(), p.Cards()
	if len(accounts) != 2 || len(cards) != 1 || cards[0].Account() != accounts[0] || accounts[0].Cards()[0] != cards[0] || len(accounts[1].Cards()) != 0 {
		t.Fatal("product identity relationships lost")
	}
	binding := cards[0].bankBinding()
	if binding != accounts[0].bankBinding() || !clientBindingOwnsRequester(c, binding.requester) || binding.operations != c.Operations() || binding.cards != c.Cards() || binding.transfers != c.Transfers() || p.Transfers() != c.Transfers() {
		t.Fatal("snapshot did not retain client's shared native APIs/issuer")
	}
}

func TestClientBindingPortfolioOneReadAndSharedSnapshotLifetime(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	first := clientBindingPortfolio(t, c, false)
	second := clientBindingPortfolio(t, c, true)
	clientBindingCheckSnapshot(t, c, first)
	clientBindingCheckSnapshot(t, c, second)
	if first == second || first.Cards()[0] == second.Cards()[0] || first.Accounts()[0] == second.Accounts()[0] {
		t.Fatal("reads reused a financial snapshot")
	}
	cards, err := c.Cards().List(context.Background(), false)
	if err != nil || cards[0].bankBinding() != first.Cards()[0].bankBinding() || cards[0].Account().Cards()[0] != cards[0] {
		t.Fatal("cards shortcut created a new binding/issuer")
	}
	accounts, err := c.Accounts().List(context.Background(), true)
	if err != nil || accounts[0].bankBinding() != first.Accounts()[0].bankBinding() || accounts[0].Cards()[0].Account() != accounts[0] {
		t.Fatal("accounts shortcut created a new binding/issuer")
	}
	calls := tr.snapshotCalls()
	if len(calls) != 4 {
		t.Fatal("not exactly one products read per snapshot")
	}
	for i, force := range []bool{false, true, false, true} {
		if calls[i].target != c.core().bundle.APIBase+ProductsPath || !reflect.DeepEqual(calls[i].body, resourceProductsCall(force).Payload) {
			t.Fatal("source forceUpdate/withData request changed")
		}
	}

	// All mutable wrappers/slices/raw copies are detached, while the relation
	// identity within one snapshot remains stable.
	raw := first.Raw()
	raw.Accounts[0].Balance.Amount = resourceAmount(t, "999.99")
	*raw.Cards[0].AccountID = "changed"
	raw.Cards[0].Balance.Amount = resourceAmount(t, "0.01")
	cardCopy := first.Cards()[0].Snapshot()
	*cardCopy.AccountID = "different"
	cardCopy.Balance.Amount = resourceAmount(t, "0.02")
	first.Accounts()[0].Balance().Amount = resourceAmount(t, "0.03")
	*first.Cards()[0].AccountID() = "caller-copy"
	cardSlice := first.Cards()
	cardSlice[0] = nil
	for _, p := range []*BankPortfolio{first, second} {
		clientBindingCheckSnapshot(t, c, p)
		if p.Raw().Accounts[0].Balance.Amount.String() != "10.50" || p.Cards()[0].Balance().Amount.String() != "2.50" || *p.Cards()[0].AccountID() != "1001" {
			t.Fatal("raw financial snapshot aliased caller mutation")
		}
	}
}
