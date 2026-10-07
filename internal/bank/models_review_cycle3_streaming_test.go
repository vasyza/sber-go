package bank

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
	"testing"
)

// The native value is deliberately delegated through the official encoder.
// Custom raw JSON alongside it must not inherit native financial provenance.
type cycle3TypedStreamContainer struct {
	Payload any
	Secret  string
}

func (v cycle3TypedStreamContainer) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("native/~snapshot")); err != nil {
		return err
	}
	if err := jsonv2.MarshalEncode(e, v.Payload); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("raw")); err != nil {
		return err
	}
	if err := e.WriteValue([]byte(`{"id":"4111111111111111","amount":"4111111111111111.50","n":9007199254740993.0100,"tiny":1.2300e-1000}`)); err != nil {
		return err
	}
	return e.WriteToken(jsontext.EndObject)
}
func (cycle3TypedStreamContainer) MarshalJSON() ([]byte, error) {
	return []byte(`"synthetic-legacy-private"`), nil
}

type cycle3TypedStreamFallback struct {
	Payload any    `json:"payload"`
	Display string `json:"display"`
}

func (cycle3TypedStreamFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }

type cycle3TypedLegacyFallback struct{ Payload any }

func (cycle3TypedLegacyFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }
func (cycle3TypedLegacyFallback) MarshalJSON() ([]byte, error) {
	return []byte(`{"id":"4111111111111111","amount":"4111111111111111.50"}`), nil
}

func TestModelCycle3StreamingDelegationRetainsReachedNativeProvenance(t *testing.T) {
	id := cycle3LiteralID
	m := cycle3FinancialAmount(t)
	pan, masked := "PAN 4111-1111-1111-1111", "PAN •••• 1111"
	account := Account{ID: id, Name: pan, Balance: m, Kind: "ctaccount"}
	card := Card{ID: id, Name: pan, Balance: m, AccountID: &id, AccountBalance: m}
	wantAccount := account
	wantAccount.Name = masked
	wantCard := card
	wantCard.Name = masked
	raw := Products{Accounts: []Account{account}, Cards: []Card{card}}
	wantRaw := Products{Accounts: []Account{wantAccount}, Cards: []Card{wantCard}}
	requester := &cycle3SnapshotRequester{Secret: "synthetic-private-\xff"}
	p := NewBankPortfolio(raw, requester)
	a, c := p.Accounts()[0], p.Cards()[0]
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"raw_account", account, wantAccount}, {"raw_card", card, wantCard}, {"raw_products", raw, wantRaw},
		{"bound_account_pointer", a, wantAccount}, {"bound_account_value", *a, wantAccount},
		{"bound_card_pointer", c, wantCard}, {"bound_card_value", *c, wantCard},
		{"bound_portfolio_pointer", p, wantRaw}, {"bound_portfolio_value", *p, wantRaw},
		{"slice", []any{a, c, p}, []any{wantAccount, wantCard, wantRaw}},
		{"map", map[string]any{"a": a, "c": c, "p": p}, map[string]any{"a": wantAccount, "c": wantCard, "p": wantRaw}},
		{"array", [1]BankCard{*c}, []any{wantCard}},
		{"addressable_field", &struct {
			Card BankCard `json:"card"`
		}{*c}, map[string]any{"card": wantCard}},
		{"operation", Operation{ID: id, Amount: m, FromResource: &Resource{Type: "card", ID: id}, ScopeCardIDs: []string{id}}, Operation{ID: id, Amount: m, FromResource: &Resource{Type: "card", ID: id}, ScopeCardIDs: []string{id}}},
		{"native_credential_union", OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: cycle3ForeignLegacyEntity{BankAccount: *a, Password: "synthetic-private-\xff"}}, {Value: m}}}, OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: map[string]any{"id": "<foreign-redacted>", "amount": "•••• 1111.50"}}, {Value: m}}}},
		{"native_stream_credential_union", OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: &cycle3ForeignStreamEntity{BankCard: *c, Password: "synthetic-private-\xff"}}, {Value: m}}}, OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: map[string]any{"id": "<foreign-stream-redacted>", "amount": "•••• 1111.50"}}, {Value: m}}}},
		{"nil_entity_pointer", (*BankCard)(nil), nil},
		{"foreign_entity_redactor", cycle3ForeignLegacyEntity{BankAccount: *a, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-redacted>", "amount": "•••• 1111.50"}},
		{"foreign_stream_redactor", &cycle3ForeignStreamEntity{BankCard: *c, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-stream-redacted>", "amount": "•••• 1111.50"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, shape := range []string{"delegated", "unsupported_native_fallback"} {
				t.Run(shape, func(t *testing.T) {
					var input, expected any
					if shape == "delegated" {
						input = cycle3TypedStreamContainer{Payload: tc.input, Secret: "synthetic-private-\xff"}
						expected = map[string]any{"native/~snapshot": tc.want, "raw": map[string]any{"id": "•••• 1111", "amount": "•••• 1111.50", "n": json.Number("9007199254740993.0100"), "tiny": json.Number("1.2300e-1000")}}
					} else {
						input = cycle3TypedStreamFallback{Payload: tc.input, Display: pan}
						expected = map[string]any{"payload": tc.want, "display": masked}
					}
					cycle3AssertGenericJSON(t, input, expected)
				})
			}
		})
	}
	// Foreign explicit legacy JSON wins on sentinel fallback, not its native
	// payload. Neither financial field names nor the hidden payload exempt it.
	cycle3AssertGenericJSON(t, cycle3TypedLegacyFallback{Payload: account}, map[string]any{"id": "•••• 1111", "amount": "•••• 1111.50"})
	if requester.MarshalCalls != 0 || !reflect.DeepEqual(p.Raw(), raw) {
		t.Fatal("stream provenance traversed requester/mutated owned financial snapshots")
	}
}

func TestModelCycle3StreamingNativeTextIsValidatedBeforeLoss(t *testing.T) {
	for _, invalid := range []string{"synthetic_native_\xff", "synthetic_native_\xfe"} {
		id := invalid
		for _, payload := range []any{Account{ID: invalid}, Card{AccountID: &id}, Operation{ScopeCardIDs: []string{invalid}}, Resource{ID: invalid}, NewBankAccount(Account{ID: invalid}, nil, nil), NewBankCard(Card{ID: invalid}, nil, nil), NewBankPortfolio(Products{Cards: []Card{{ID: invalid}}}, nil)} {
			for _, input := range []any{cycle3TypedStreamContainer{Payload: payload}, cycle3TypedStreamFallback{Payload: payload}} {
				out, err := ExportJSON(input)
				var pe *ParseError
				if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
					t.Fatalf("stream repaired native identity or retained cause: %T %s %v", input, out, err)
				}
				normalized, err := JSONable(input)
				if normalized != nil || !errors.As(err, &pe) {
					t.Fatal("streaming struct incorrectly returned deferred validity")
				}
			}
		}
	}
	for _, payload := range []any{Account{ID: "literal-�"}, NewBankCard(Card{ID: "literal-�"}, nil, nil), Products{Accounts: []Account{{ID: "literal-�"}}}} {
		cycle3AssertGenericJSON(t, cycle3TypedStreamFallback{Payload: payload}, map[string]any{"payload": payload, "display": ""})
	}
}
