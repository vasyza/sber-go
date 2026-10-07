package sber

import (
	"encoding/json"
	"math"
	"testing"
)

type cycle3ForeignAccount Account

func TestModelCycle3FinancialLiteralCodesAndExactNumbers(t *testing.T) {
	id := cycle3LiteralID
	m := cycle3FinancialAmount(t)
	m.Currency = id // Source currency is a literal code, never display text.
	account := Account{ID: id, Balance: m, Kind: "ctaccount"}
	card := Card{ID: id, AccountID: &id, Balance: m, AccountBalance: m}
	raw := Products{Accounts: []Account{account}, Cards: []Card{card}}
	p := NewBankPortfolio(raw, nil)
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"currency_money_value", *m, *m}, {"currency_money_pointer", m, m},
		{"currency_account", account, account}, {"currency_card", card, card}, {"currency_products", raw, raw},
		{"currency_bound_account", p.Accounts()[0], account}, {"currency_bound_card", p.Cards()[0], card}, {"currency_bound_portfolio", p, raw},
		{"currency_stream", cycle3TypedStreamFallback{Payload: raw}, map[string]any{"payload": raw, "display": ""}},
		{"currency_stream_bound", cycle3TypedStreamFallback{Payload: p}, map[string]any{"payload": raw, "display": ""}},
		{"currency_scalar_stream", cycle3TypedStreamFallback{Payload: m}, map[string]any{"payload": m, "display": ""}},
		{"currency_mixed", []any{m, p.Cards()[0], card}, []any{m, card, card}},
		{"account_kind_literal", Account{Kind: id}, Account{Kind: id}},
		{"resource_type_literal", Resource{Type: id, ID: id}, Resource{Type: id, ID: id}},
		{"operation_date_literal", Operation{ID: id, Date: id}, Operation{ID: id, Date: id}},
		{"card_balance_source_literal", Card{ID: id, BalanceSource: &id}, Card{ID: id, BalanceSource: &id}},
		{"ledger_direction_literal", CardLedgerEntry{OperationID: id, CardID: id, Direction: id}, CardLedgerEntry{OperationID: id, CardID: id, Direction: id}},
		{"foreign_named_native_schema", cycle3ForeignAccount(Account{ID: id, Kind: id}), Account{ID: "•••• 1111", Kind: "•••• 1111"}},
		{"raw_number_luhn", json.Number(id), json.Number(id)},
		{"raw_number_exact", json.Number("9007199254740993.0100"), json.Number("9007199254740993.0100")},
		{"decimal_luhn", m.Amount, m.Amount},
		{"decimal_pointer_luhn", &m.Amount, &m.Amount},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.want) })
	}
	// Currency outside native Money is still display text, even at that JSON key.
	cycle3AssertGenericJSON(t, map[string]any{"currency": id}, map[string]any{"currency": "•••• 1111"})
	for _, input := range []any{math.NaN(), math.Inf(1), math.Inf(-1), float32(math.Inf(1)), nil, true, false, "NaN", "Infinity"} {
		if _, err := ParseDecimal(input); err == nil {
			t.Fatalf("nonfinite/nonmonetary amount accepted: %T", input)
		}
	}
	for _, text := range []string{"-0.00", "9007199254740993.0100", "4111111111111111.50", "-4111111111111111.50", "1.2300e-1000"} {
		d, err := ParseDecimal(json.Number(text))
		if err != nil {
			t.Fatal(err)
		}
		cycle3AssertGenericJSON(t, Money{Amount: d, Currency: "�"}, Money{Amount: d, Currency: "�"})
	}
	for _, payload := range []any{(*Money)(nil), (*Decimal)(nil), (*BankAccount)(nil), (*BankCard)(nil), (*BankPortfolio)(nil), (*Account)(nil), (*Card)(nil), (*Resource)(nil), (*Operation)(nil)} {
		cycle3AssertGenericJSON(t, cycle3TypedStreamFallback{Payload: payload}, map[string]any{"payload": nil, "display": ""})
	}
}
