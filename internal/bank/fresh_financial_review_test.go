package bank

import (
	"encoding/json"
	"strings"
	"testing"
)

// Independent synthetic seam matrix. No requester, session, network or tenant.
func TestFreshReviewFinancialControls(t *testing.T) {
	d, e := ParseDecimal("4111111111111111.50")
	if e != nil {
		t.Fatal(e)
	}
	m := &Money{Amount: d, Currency: "RUB"}
	aid := "fixture-parent"
	raw := Products{Accounts: []Account{{ID: aid, Name: RedactPAN("PAN 4111-1111-1111-1111"), Balance: m, Kind: "ctaccount"}}, Cards: []Card{{ID: "fixture-card", AccountID: &aid, Balance: m, AccountBalance: m}}}
	p := NewBankPortfolio(raw, nil)
	c := p.Cards()[0]
	a := p.Accounts()[0]
	for _, v := range []any{raw, a.Snapshot(), c.Snapshot(), p.Raw(), map[string]any{"products": p.Raw()}, []any{a.Snapshot(), c.Snapshot()}} {
		b, e := ExportJSON(v)
		if e != nil || !strings.Contains(string(b), d.String()) {
			t.Fatalf("native typed snapshot control broken: %s %v", b, e)
		}
	}
	for _, f := range []func() ([]byte, error){p.ExportJSON, a.ExportJSON, c.ExportJSON} {
		b, e := f()
		if e != nil || !strings.Contains(string(b), d.String()) {
			t.Fatalf("entity-specific financial control broken: %s %v", b, e)
		}
	}
	literal := NewBankAccount(Account{ID: "4111111111111111", Balance: m, Kind: "ctaccount"}, nil, nil)
	b, e := literal.ExportJSON()
	if e != nil || !strings.Contains(string(b), `"id": "4111111111111111"`) {
		t.Fatalf("literal entity ID control broken: %s %v", b, e)
	}
	if b, e := json.Marshal(literal.Snapshot()); e != nil || !strings.Contains(string(b), `"id":"4111111111111111"`) {
		t.Fatal("raw snapshot literal ID control broken")
	}
}
func TestFreshReviewFinancialLiteralIDs(t *testing.T) {
	d, _ := ParseDecimal("4111111111111111.50")
	m := &Money{Amount: d, Currency: "RUB"}
	raw := Account{ID: "4111111111111111", Name: "Fixture", Balance: m, Kind: "ctaccount"}
	standard, e := json.Marshal(raw)
	if e != nil || !strings.Contains(string(standard), `"id":"4111111111111111"`) {
		t.Fatal("raw literal ID control broken")
	}
	generic, e := ExportJSON(raw)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("native_snapshot_generic=%s", generic)
	if !strings.Contains(string(generic), `"id": "4111111111111111"`) {
		t.Error("generic financial snapshot export rewrote a literal account ID as display PAN")
	}
}
func TestFreshReviewFinancialSeamMatrix(t *testing.T) {
	d, _ := ParseDecimal("4111111111111111.50")
	m := &Money{Amount: d, Currency: "RUB"}
	p := NewBankPortfolio(Products{Accounts: []Account{{ID: "fixture-parent", Balance: m, Kind: "ctaccount"}}, Cards: []Card{{ID: "fixture-card", Balance: m, AccountBalance: m}}}, nil)
	a := p.Accounts()[0]
	c := p.Cards()[0]
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"bound_account_pointer", a}, {"bound_card_pointer", c}, {"bound_portfolio_pointer", p},
		{"bound_account_value", *a}, {"bound_card_value", *c}, {"bound_portfolio_value", *p},
		{"slice", []any{a, c, p}}, {"map", map[string]any{"a": a, "c": c, "p": p}}, {"array", [1]*BankCard{c}},
		{"addressable_field", &struct {
			Card BankCard `json:"card"`
		}{*c}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, e := ExportJSON(tc.value)
			if e != nil {
				t.Fatalf("generic financial export returned an error: %v", e)
			}
			t.Logf("generic=%s", b)
			if !strings.Contains(string(b), d.String()) || strings.Contains(string(b), "•••• 1111.50") {
				t.Error("generic export converted a typed financial amount to PAN-masked display text")
			}
		})
	}
}
