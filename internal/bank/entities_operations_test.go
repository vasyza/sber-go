package bank

import (
	"context"
	"strings"
	"testing"
)

func TestEntityResourceSyntaxMatchesAccountKindsAndCard(t *testing.T) {
	p := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	for i, want := range [][2]string{{"ct-account:1001", "transactionAccount:1001"}, {"account:2002", "account:2002"}} {
		a := p.Accounts()[i]
		op, err := a.OperationResource()
		if err != nil || op != want[0] {
			t.Fatal("account operation resource")
		}
		tr, err := a.TransferResource()
		if err != nil || tr != want[1] {
			t.Fatal("account transfer resource")
		}
	}
	c := p.Cards()[0]
	op, err := c.OperationResource()
	if err != nil || op != "card:4004" {
		t.Fatal("card op syntax")
	}
	tr, err := c.TransferResource()
	if err != nil || tr != "card:4004" {
		t.Fatal("card transfer syntax")
	}
	for _, id := range []string{"", " bad", "bad ", "a/b", "bad\n", strings.Repeat("a", 129)} {
		a := NewBankAccount(Account{ID: id, Kind: "ctaccount"}, &resourceScript{t: t}, nil)
		c := NewBankCard(Card{ID: id}, &resourceScript{t: t}, nil)
		if _, err := a.OperationResource(); err == nil {
			t.Fatal("bad account op ID repaired")
		}
		if _, err := a.TransferResource(); err == nil {
			t.Fatal("bad transfer ID repaired")
		}
		if _, err := c.OperationResource(); err == nil {
			t.Fatal("bad card ID")
		}
		if _, err := c.TransferResource(); err == nil {
			t.Fatal("bad card transfer")
		}
	}
	a := NewBankAccount(Account{ID: "valid", Kind: "unknown"}, &resourceScript{t: t}, nil)
	if _, err := a.OperationResource(); err == nil {
		t.Fatal("unknown kind")
	}
	if _, err := a.TransferResource(); err == nil {
		t.Fatal("unknown kind transfer")
	}
	// No request is necessary to compute validated resource identifiers.
}
func TestEntityIterOperationsUsesScopedLazyIterator(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "card:4004", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1", "lookahead")}, {Call: resourceHistoryCall(0, 2, "account:2002", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("2")}}}
	p := NewBankPortfolio(entityFixtureProducts(t), r)
	q := OperationsQuery{Limit: 1, MaxPages: 5, From: "2026-07-01"}
	seq := p.Cards()[0].IterOperations(context.Background(), q)
	if len(r.calls) != 0 {
		t.Fatal("iterator eager")
	}
	for op, err := range seq {
		if err != nil || op.ID != "1" {
			t.Fatal("card iterator")
		}
		break
	}
	for op, err := range p.Accounts()[1].IterOperations(context.Background(), q) {
		if err != nil || op.ID != "2" {
			t.Fatal("account iterator")
		}
	}
	r.done()
}
func TestEntityOperationsAlwaysScopedToSnapshotResource(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 11, "card:4004", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1")}, {Call: resourceHistoryCall(0, 11, "ct-account:1001", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("2")}}}
	p := NewBankPortfolio(entityFixtureProducts(t), r)
	q := OperationsQuery{Resource: "card:wrong-caller-scope", Limit: 10, MaxPages: 5, From: "2026-07-01"}
	cards, err := p.Cards()[0].Operations(context.Background(), q)
	if err != nil || cards[0].ScopeCardIDs[0] != "4004" {
		t.Fatal("scoped card operations")
	}
	accounts, err := p.Accounts()[0].Operations(context.Background(), q)
	if err != nil || len(accounts[0].ScopeCardIDs) != 0 {
		t.Fatal("scoped account operations")
	}
	r.done()
}
