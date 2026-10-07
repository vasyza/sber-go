package sber

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func cycle3AssertSnapshotError(t *testing.T, output []byte, err error) {
	t.Helper()
	var pe *ParseError
	if output != nil || !errors.As(err, &pe) || pe.Field() != "financial_snapshot" || pe.Error() != "sber: invalid domain data" || errors.Unwrap(pe) != nil {
		t.Fatalf("owned snapshot integrity failure missing/unsafe: data=%s error=%v", output, err)
	}
	if strings.Contains(err.Error(), "synthetic_native_") {
		t.Fatal("owned native error retained original text")
	}
}

func cycle3CheckOwnedNativeReject(t *testing.T, owned json.Marshaler, explicit func() ([]byte, error)) {
	t.Helper()
	data, err := explicit()
	cycle3AssertSnapshotError(t, data, err)
	data, err = owned.MarshalJSON()
	cycle3AssertSnapshotError(t, data, err)
	data, err = json.Marshal(owned)
	cycle3AssertSnapshotError(t, data, err)
	data, err = ExportJSON(owned)
	var pe *ParseError
	if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
		t.Fatal("generic owned export repaired native text")
	}
	normalized, err := JSONable(owned)
	if normalized != nil || !errors.As(err, &pe) || pe.Field() != "json_export" {
		t.Fatal("owned JSONable did not reject original native text eagerly")
	}
}

func TestEntityCycle3OrdinaryMarshalRejectsOriginalNativeText(t *testing.T) {
	for _, invalid := range []string{"synthetic_native_\xff", "synthetic_native_\xfe"} {
		t.Run(string([]byte{invalid[len(invalid)-1]}), func(t *testing.T) {
			for _, field := range []string{"ID", "Name", "Last4", "State", "Kind", "Balance.Currency"} {
				t.Run("account/"+field, func(t *testing.T) {
					raw := Account{ID: "literal-�", Balance: cycle3FinancialAmount(t), Kind: "ctaccount"}
					if field == "Balance.Currency" {
						raw.Balance.Currency = invalid
					} else {
						reflect.ValueOf(&raw).Elem().FieldByName(field).SetString(invalid)
					}
					owned := NewBankAccount(raw, nil, nil)
					cycle3CheckOwnedNativeReject(t, owned, owned.ExportJSON)
					if !reflect.DeepEqual(owned.Snapshot(), raw) {
						t.Fatal("failed account marshaling changed raw native bytes")
					}
				})
			}
			for _, field := range []string{"ID", "Name", "Last4", "Type", "State", "BalanceSource", "AccountID", "Balance.Currency", "AccountBalance.Currency"} {
				t.Run("card/"+field, func(t *testing.T) {
					raw := Card{ID: "literal-�", Balance: cycle3FinancialAmount(t), AccountBalance: cycle3FinancialAmount(t)}
					switch field {
					case "Balance.Currency":
						raw.Balance.Currency = invalid
					case "AccountBalance.Currency":
						raw.AccountBalance.Currency = invalid
					case "BalanceSource", "AccountID":
						reflect.ValueOf(&raw).Elem().FieldByName(field).Set(reflect.ValueOf(&invalid))
					default:
						reflect.ValueOf(&raw).Elem().FieldByName(field).SetString(invalid)
					}
					owned := NewBankCard(raw, nil, nil)
					cycle3CheckOwnedNativeReject(t, owned, owned.ExportJSON)
					if !reflect.DeepEqual(owned.Snapshot(), raw) {
						t.Fatal("failed card marshaling changed raw native bytes")
					}
				})
			}
		})
	}
	for _, tc := range []struct {
		name string
		raw  Products
	}{
		{"same_kind_distinct_invalid_card_ids", Products{Cards: []Card{{ID: "synthetic_native_\xff"}, {ID: "synthetic_native_\xfe"}}}},
		{"same_kind_distinct_invalid_account_ids", Products{Accounts: []Account{{ID: "synthetic_native_\xff"}, {ID: "synthetic_native_\xfe"}}}},
		{"nested_account_currency", Products{Accounts: []Account{{ID: "literal", Balance: &Money{Amount: cycle3FinancialAmount(t).Amount, Currency: "synthetic_native_\xff"}}}}},
		{"nested_card_account_id", Products{Cards: []Card{{ID: "literal", AccountID: func() *string { s := "synthetic_native_\xff"; return &s }()}}}},
	} {
		t.Run("portfolio/"+tc.name, func(t *testing.T) {
			owned := NewBankPortfolio(tc.raw, nil)
			cycle3CheckOwnedNativeReject(t, owned, owned.ExportJSON)
			if !reflect.DeepEqual(owned.Raw(), tc.raw) {
				t.Fatal("failed portfolio marshaling changed raw native bytes")
			}
		})
	}
}

func TestEntityCycle3OrdinaryMarshalKeepsCompactCompleteRawShape(t *testing.T) {
	m := cycle3FinancialAmount(t)
	m.Currency = "�"
	id := cycle3LiteralID
	empty := ""
	raw := Products{Accounts: []Account{{ID: id, Name: "literal � 😀 <>& PAN 4111-1111-1111-1111", Last4: "1111", State: "OPEN", Hidden: true, Arrested: true, Balance: m, Kind: "ctaccount"}, {}}, Cards: []Card{{ID: id, Name: "literal �", Last4: "1111", Type: "debit", State: "OPEN", Hidden: true, Arrested: true, IsMain: true, Balance: m, BalanceSource: &empty, AccountID: &id, AccountBalance: m}, {ID: "nil-amounts"}}}
	requester := &cycle3SnapshotRequester{Secret: "synthetic-private-\xff"}
	p := NewBankPortfolio(raw, requester)
	a, c := p.Accounts()[0], p.Cards()[0]
	for _, tc := range []struct {
		name     string
		value    json.Marshaler
		raw      any
		explicit func() ([]byte, error)
	}{
		{"account_pointer", a, raw.Accounts[0], a.ExportJSON}, {"account_value", *a, raw.Accounts[0], a.ExportJSON},
		{"card_pointer", c, raw.Cards[0], c.ExportJSON}, {"card_value", *c, raw.Cards[0], c.ExportJSON},
		{"portfolio_pointer", p, raw, p.ExportJSON}, {"portfolio_value", *p, raw, p.ExportJSON},
		{"zero_account", BankAccount{}, Account{}, (&BankAccount{}).ExportJSON},
		{"zero_card", BankCard{}, Card{}, (&BankCard{}).ExportJSON},
		{"zero_portfolio", BankPortfolio{}, Products{}, (&BankPortfolio{}).ExportJSON},
		{"empty_portfolio_lists", *NewBankPortfolio(Products{Accounts: []Account{}, Cards: []Card{}}, requester), Products{Accounts: []Account{}, Cards: []Card{}}, NewBankPortfolio(Products{Accounts: []Account{}, Cards: []Card{}}, requester).ExportJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := json.Marshal(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			direct, err := tc.value.MarshalJSON()
			if err != nil || !bytes.Equal(direct, want) || bytes.Contains(direct, []byte("\n")) {
				t.Fatalf("ordinary raw shape/compact bytes changed: got=%s want=%s error=%v", direct, want, err)
			}
			data, err := json.Marshal(tc.value)
			if err != nil || !bytes.Equal(data, want) {
				t.Fatalf("ordinary encoder raw bytes changed: %s %v", data, err)
			}
			data, err = tc.explicit()
			if err != nil || !reflect.DeepEqual(cycle3DecodedJSON(t, data), cycle3DecodedJSON(t, want)) {
				t.Fatalf("explicit raw financial export changed: %s %v", data, err)
			}
		})
	}
	if requester.MarshalCalls != 0 || !reflect.DeepEqual(p.Raw(), raw) {
		t.Fatal("ordinary serializer traversed requester or mutated snapshot")
	}
	for _, value := range []any{(*BankPortfolio)(nil), (*BankAccount)(nil), (*BankCard)(nil)} {
		data, err := json.Marshal(value)
		if err != nil || string(data) != "null" {
			t.Fatal("nil entity JSON behavior changed")
		}
	}
}
