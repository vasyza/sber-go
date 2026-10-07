package bank

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"reflect"
	"testing"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// Synthetic exact native identity roles; no requester/session/network.
const cycle3LiteralID = "4111111111111111"

func cycle3FinancialAmount(t *testing.T) *Money {
	t.Helper()
	d, err := ParseDecimal("4111111111111111.50")
	if err != nil {
		t.Fatal(err)
	}
	return &Money{Amount: d, Currency: "RUB"}
}

func cycle3DecodedJSON(t *testing.T, data []byte) any {
	t.Helper()
	if strictjson.Validate(data) != nil {
		t.Fatalf("invalid complete document: %s", data)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func cycle3AssertGenericJSON(t *testing.T, input, expected any) {
	t.Helper()
	want, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ExportJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cycle3DecodedJSON(t, out), cycle3DecodedJSON(t, want)) {
		t.Fatalf("native financial shape/identity/value changed: got=%s want=%s", out, want)
	}
	normalized, err := JSONable(input)
	if err != nil {
		t.Fatal(err)
	}
	normalizedJSON, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cycle3DecodedJSON(t, normalizedJSON), cycle3DecodedJSON(t, want)) {
		t.Fatalf("JSONable lost native provenance: got=%s want=%s", normalizedJSON, want)
	}
	after, err := json.Marshal(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("financial export mutated its input")
	}
}

func TestModelCycle3LiteralIdentityIsTypedNotDisplay(t *testing.T) {
	m := cycle3FinancialAmount(t)
	id := cycle3LiteralID
	empty := ""
	financial := true
	pan := "PAN 4111-1111-1111-1111"
	masked := "PAN •••• 1111"
	account := Account{ID: id, Name: pan, Last4: "1111", State: "OPEN", Hidden: true, Arrested: true, Balance: m, Kind: "ctaccount"}
	card := Card{ID: id, Name: pan, Last4: "1111", Type: "debit", State: "OPEN", Hidden: true, Arrested: true, IsMain: true, Balance: m, BalanceSource: &empty, AccountID: &id, AccountBalance: m}
	op := Operation{ID: id, Date: "2026-10-01", Form: pan, Type: "transfer", ClassificationCode: id, CreationChannel: pan, State: "DONE", StateName: pan, StateDescription: pan, Merchant: pan, Description: pan, Amount: m, BillingAmount: m, NationalAmount: m, Commission: m, Tips: m, RefusalReason: pan, FromResource: &Resource{Type: "card", ID: id}, ToResource: &Resource{Type: "account", ID: id}, ScopeCardIDs: []string{id, "literal-second"}, IsFinancial: &financial, IsHidden: true, BalanceAfter: m, BalanceAfterResourceID: &id, BalanceAfterResourceName: &pan}
	wantAccount := account
	wantAccount.Name = masked
	wantCard := card
	wantCard.Name = masked
	wantOperation := op
	wantOperation.Form = masked
	wantOperation.CreationChannel = masked
	wantOperation.StateName = masked
	wantOperation.StateDescription = masked
	wantOperation.Merchant = masked
	wantOperation.Description = masked
	wantOperation.RefusalReason = masked
	wantOperation.BalanceAfterResourceName = &masked
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"account", account, wantAccount},
		{"account_pointer", &account, &wantAccount},
		{"card", card, wantCard},
		{"card_pointer", &card, &wantCard},
		{"resource", Resource{Type: "card", ID: id}, Resource{Type: "card", ID: id}},
		{"operation", op, wantOperation},
		{"operation_pointer", &op, &wantOperation},
		{"ledger", CardLedgerEntry{OperationID: id, CardID: id, Direction: "debit", Amount: m}, CardLedgerEntry{OperationID: id, CardID: id, Direction: "debit", Amount: m}},
		{"detail", OperationDetail{UOHID: id, Title: pan, Amount: m, Fields: []OperationDetailField{{Name: pan, Type: "money", Value: m}, {Name: "text", Value: pan}}}, OperationDetail{UOHID: id, Title: masked, Amount: m, Fields: []OperationDetailField{{Name: masked, Type: "money", Value: m}, {Name: "text", Value: masked}}}},
		{"card_info", CardInfo{ID: id, Name: pan, CardHolder: pan, Limits: CardLimits{Purchase: m, Available: m, AvailableTotal: m}, Credit: &CreditInfo{Limit: m, OwnSum: m, Debt: m, MinPayment: m, MinPaymentDate: "2026-10-01"}}, CardInfo{ID: id, Name: masked, CardHolder: masked, Limits: CardLimits{Purchase: m, Available: m, AvailableTotal: m}, Credit: &CreditInfo{Limit: m, OwnSum: m, Debt: m, MinPayment: m, MinPaymentDate: "2026-10-01"}}},
		{"category", CategoryAmount{ID: id, Name: pan, ExternalID: pan, NationalAmount: m, VisibleAmount: m, CountOperations: 3}, CategoryAmount{ID: id, Name: masked, ExternalID: masked, NationalAmount: m, VisibleAmount: m, CountOperations: 3}},
		{"products", Products{Accounts: []Account{account}, Cards: []Card{card}}, Products{Accounts: []Account{wantAccount}, Cards: []Card{wantCard}}},
		{"operations_page", OperationsPage{Operations: []Operation{op}}, OperationsPage{Operations: []Operation{wantOperation}}},
		{"nil_literal_pointers", Card{ID: id}, Card{ID: id}},
		{"nil_scope", Operation{ID: id}, Operation{ID: id}},
		{"empty_scope", Operation{ID: id, ScopeCardIDs: []string{}}, Operation{ID: id, ScopeCardIDs: []string{}}},
		{"ordinary_id_key_is_display", map[string]any{"id": id, "amount": id, "display": pan}, map[string]any{"id": "•••• 1111", "amount": "•••• 1111", "display": masked}},
		{"foreign_native_tag_is_not_provenance", struct {
			ID string `json:"id" sber:"literal"`
		}{id}, map[string]any{"id": "•••• 1111"}},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.want) })
	}
}

type cycle3SnapshotRequester struct {
	BusinessRequester // nil: any attempted resource/session activity panics.
	Secret            string
	MarshalCalls      int
}

func (r *cycle3SnapshotRequester) MarshalJSON() ([]byte, error) {
	r.MarshalCalls++
	return []byte(`"<requester-redacted>"`), nil
}

type cycle3ForeignLegacyEntity struct {
	BankAccount
	Password string
}

func (cycle3ForeignLegacyEntity) MarshalJSON() ([]byte, error) {
	return []byte(`{"id":"<foreign-redacted>","amount":"4111111111111111.50"}`), nil
}

type cycle3ForeignStreamEntity struct {
	BankCard
	Password string
}

func (*cycle3ForeignStreamEntity) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteValue([]byte(`{"id":"<foreign-stream-redacted>","amount":"4111111111111111.50"}`))
}

func TestModelCycle3BoundFinancialProvenancePreservesContainerShape(t *testing.T) {
	m := cycle3FinancialAmount(t)
	id := cycle3LiteralID
	empty := ""
	pan := "PAN 4111-1111-1111-1111"
	masked := "PAN •••• 1111"
	raw := Products{Accounts: []Account{{ID: id, Name: pan, Last4: "1111", State: "OPEN", Hidden: true, Arrested: true, Balance: m, Kind: "ctaccount"}}, Cards: []Card{{ID: id, Name: pan, Last4: "1111", Type: "debit", State: "OPEN", Hidden: true, Arrested: true, IsMain: true, Balance: m, BalanceSource: &empty, AccountID: &id, AccountBalance: m}}}
	want := entityCopyProducts(raw)
	want.Accounts[0].Name = masked
	want.Cards[0].Name = masked
	requester := &cycle3SnapshotRequester{Secret: "synthetic-private-\xff"}
	p := NewBankPortfolio(raw, requester)
	a, c := p.Accounts()[0], p.Cards()[0]
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"account_pointer", a, want.Accounts[0]}, {"card_pointer", c, want.Cards[0]}, {"portfolio_pointer", p, want},
		{"account_value", *a, want.Accounts[0]}, {"card_value", *c, want.Cards[0]}, {"portfolio_value", *p, want},
		{"slice", []any{a, c, p}, []any{want.Accounts[0], want.Cards[0], want}},
		{"map", map[string]any{"a": a, "c": c, "p": p}, map[string]any{"a": want.Accounts[0], "c": want.Cards[0], "p": want}},
		{"array", [1]*BankCard{c}, []any{want.Cards[0]}},
		{"addressable_field", &struct {
			Card BankCard `json:"card"`
		}{*c}, map[string]any{"card": want.Cards[0]}},
		{"interface_field", &struct {
			Account any `json:"account"`
		}{a}, map[string]any{"account": want.Accounts[0]}},
		{"addressable_array", &[2]BankAccount{*a, *a}, []any{want.Accounts[0], want.Accounts[0]}},
		{"slice_values", []BankPortfolio{*p, *p}, []any{want, want}},
		{"native_and_bound", []any{a.Snapshot(), *a}, []any{want.Accounts[0], want.Accounts[0]}},
		{"nil_account", (*BankAccount)(nil), nil}, {"nil_card", (*BankCard)(nil), nil}, {"nil_portfolio", (*BankPortfolio)(nil), nil},
		{"zero_account", BankAccount{}, Account{}}, {"zero_card", BankCard{}, Card{}}, {"zero_portfolio", BankPortfolio{}, Products{}},
		{"nil_products_lists", NewBankPortfolio(Products{}, requester), Products{}},
		{"empty_products_lists", NewBankPortfolio(Products{Accounts: []Account{}, Cards: []Card{}}, requester), Products{Accounts: []Account{}, Cards: []Card{}}},
		{"foreign_legacy_embedded_entity", cycle3ForeignLegacyEntity{BankAccount: *a, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-redacted>", "amount": "•••• 1111.50"}},
		{"foreign_stream_embedded_entity", &cycle3ForeignStreamEntity{BankCard: *c, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-stream-redacted>", "amount": "•••• 1111.50"}},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.want) })
	}
	if !reflect.DeepEqual(p.Raw(), raw) || requester.MarshalCalls != 0 || c.Account() != a || a.Cards()[0] != c {
		t.Fatal("financial export traversed requester or changed snapshots/relations")
	}
	copy := a.Snapshot()
	copy.Balance.Currency = "USD"
	if a.Balance().Currency != "RUB" {
		t.Fatal("financial export damaged defensive copy semantics")
	}
}
