package bank

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
	"testing"
	"unicode/utf8"

	"github.com/vasyza/sber-go/internal/strictjson"
)

func TestModelCycle3LiteralNativeTextIntegrity(t *testing.T) {
	for _, text := range []string{"synthetic_native_\xff", "synthetic_native_\xfe", "literal � 😀"} {
		for _, value := range []any{
			Account{ID: text, Kind: text}, Card{ID: text, AccountID: &text, BalanceSource: &text}, Resource{ID: text, Type: text},
			Operation{ID: text, Date: text, ClassificationCode: text, ScopeCardIDs: []string{text}, BalanceAfterResourceID: &text},
			CardLedgerEntry{OperationID: text, CardID: text, Direction: text}, OperationDetail{UOHID: text}, CardInfo{ID: text}, CategoryAmount{ID: text},
			Money{Amount: cycle3FinancialAmount(t).Amount, Currency: text},
		} {
			for _, input := range []any{value, []any{value}, map[string]any{"value": value}, cycle3TypedStreamFallback{Payload: value}} {
				if utf8.ValidString(text) {
					if _, err := ExportJSON(input); err != nil {
						t.Fatalf("literal replacement character rejected: %T %v", input, err)
					}
					continue
				}
				out, err := ExportJSON(input)
				var pe *ParseError
				if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
					t.Fatalf("original native text repaired: %T %s %v", input, out, err)
				}
			}
		}
	}
}

type cycle3DeferredStreamFloat float64

func (cycle3DeferredStreamFloat) MarshalJSONTo(*jsontext.Encoder) error {
	return errors.New("synthetic-private-stream-error")
}

func TestModelCycle3DeferredStreamingMarkerStaysStatic(t *testing.T) {
	v := cycle3DeferredStreamFloat(1.25)
	for _, input := range []any{v, []any{v}, [1]cycle3DeferredStreamFloat{v}, map[string]any{"value": v}, struct {
		Value any `json:"value"`
	}{v}} {
		normalized, err := JSONable(input)
		if err != nil {
			t.Fatal("finite streaming deferred compatibility changed")
		}
		data, err := json.Marshal(normalized)
		var pe *ParseError
		if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || bytes.Contains([]byte(err.Error()), []byte("synthetic-private")) {
			t.Fatal("deferred marker retained source receiver/cause or accepted invalid output")
		}
		data, err = ExportJSON(input)
		if data != nil || !errors.As(err, &pe) || errors.Unwrap(err) != nil {
			t.Fatal("deferred export not fail-closed")
		}
	}
	for _, input := range []any{&v, cycle3TypedStreamFallback{Payload: v}, cycle3TypedStreamContainer{Payload: v}} {
		normalized, err := JSONable(input)
		if normalized != nil || err == nil {
			t.Fatal("new pointer/custom-container deferred bypass")
		}
	}
}

func FuzzModelCycle3FinancialProvenance(f *testing.F) {
	for _, tc := range []struct{ id, currency, amount string }{
		{cycle3LiteralID, "RUB", "4111111111111111.50"},
		{"literal � 😀", cycle3LiteralID, "9007199254740993.0100"},
		{"synthetic_native_\xff", "RUB", "-0.00"},
		{"synthetic_native_\xfe", "RUB", "1.2300e-1000"},
		{"literal", "synthetic_native_\xff", "0"},
		{"", "", "-4111111111111111.50"},
	} {
		f.Add(tc.id, tc.currency, tc.amount)
	}
	f.Fuzz(func(t *testing.T, id, currency, amount string) {
		if len(id) > 4096 || len(currency) > 4096 || len(amount) > 4096 {
			t.Skip()
		}
		d, err := ParseDecimal(amount)
		if err != nil {
			return
		}
		m := &Money{Amount: d, Currency: currency}
		raw := Card{ID: id, Name: "PAN 4111-1111-1111-1111", Balance: m, AccountID: &id, AccountBalance: m}
		p := NewBankPortfolio(Products{Cards: []Card{raw}}, nil)
		bound := p.Cards()[0]
		want := raw
		want.Name = "PAN •••• 1111"
		for _, tc := range []struct{ input, expected any }{
			{raw, want}, {&raw, want}, {bound, want}, {*bound, want},
			{[]any{raw, bound}, []any{want, want}},
			{map[string]any{"raw": raw, "bound": bound}, map[string]any{"raw": want, "bound": want}},
			{cycle3TypedStreamFallback{Payload: bound}, map[string]any{"payload": want, "display": ""}},
		} {
			if !utf8.ValidString(id) || !utf8.ValidString(currency) {
				out, err := ExportJSON(tc.input)
				var pe *ParseError
				if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
					t.Fatal("fuzz native financial text repaired")
				}
				continue
			}
			cycle3AssertGenericJSON(t, tc.input, tc.expected)
		}
		if raw.ID != id || raw.AccountID == nil || *raw.AccountID != id || raw.Balance.Amount != d || raw.Balance.Currency != currency || !reflect.DeepEqual(p.Raw().Cards[0], raw) {
			t.Fatal("fuzz export mutated native financial values")
		}
	})
}

func FuzzEntityCycle3OrdinaryNativeText(f *testing.F) {
	for _, text := range []string{"", "literal � 😀", cycle3LiteralID, "synthetic_native_\xff", "synthetic_native_\xfe", "PAN 4111-1111-1111-1111"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 4096 {
			t.Skip()
		}
		raw := Card{ID: text, Name: text, AccountID: &text, Balance: &Money{Currency: text}}
		c := NewBankCard(raw, nil, nil)
		p := NewBankPortfolio(Products{Cards: []Card{raw}}, nil)
		for _, tc := range []struct {
			owned    json.Marshaler
			snapshot any
			explicit func() ([]byte, error)
		}{{c, raw, c.ExportJSON}, {*c, raw, c.ExportJSON}, {p, Products{Cards: []Card{raw}}, p.ExportJSON}, {*p, Products{Cards: []Card{raw}}, p.ExportJSON}} {
			if !utf8.ValidString(text) {
				cycle3CheckOwnedNativeReject(t, tc.owned, tc.explicit)
				continue
			}
			want, err := json.Marshal(tc.snapshot)
			if err != nil {
				t.Fatal(err)
			}
			data, err := tc.owned.MarshalJSON()
			if err != nil || !bytes.Equal(data, want) {
				t.Fatal("ordinary native financial bytes changed")
			}
			data, err = json.Marshal(tc.owned)
			if err != nil || !bytes.Equal(data, want) {
				t.Fatal("standard owned financial bytes changed")
			}
		}
		if !reflect.DeepEqual(c.Snapshot(), raw) || !reflect.DeepEqual(p.Raw().Cards[0], raw) {
			t.Fatal("ordinary serialization mutated original text")
		}
	})
}

type cycle3FinancialRawStream struct {
	Native Account
	Data   []byte
}

func (v cycle3FinancialRawStream) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("native")); err != nil {
		return err
	}
	if err := jsonv2.MarshalEncode(e, v.Native); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("custom")); err != nil {
		return err
	}
	if err := e.WriteValue(v.Data); err != nil {
		return err
	}
	return e.WriteToken(jsontext.EndObject)
}

func FuzzModelCycle3StreamingFinancialDocument(f *testing.F) {
	for _, data := range [][]byte{
		[]byte(`{"id":"4111111111111111","amount":"4111111111111111.50","n":9007199254740993.0100}`),
		[]byte(`{"id":"\ud83d\ude00","replacement":"�"}`),
		[]byte(`{"amount":true,"\u0061mount":"1.00"}`),
		[]byte(`{"id":"\ud800"}`), []byte(`true false`), {'"', 0xff, '"'},
		[]byte(`{"4111 1111 1111 1111":1,"4111-1111-1111-1111":2}`),
	} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		before := bytes.Clone(data)
		valid := strictjson.Validate(data) == nil
		if valid {
			valid = !cycle2MaskedKeyCollision(cycle3DecodedJSON(t, data))
		}
		d := cycle3FinancialAmount(t)
		input := cycle3FinancialRawStream{Native: Account{ID: cycle3LiteralID, Balance: d}, Data: data}
		out, err := ExportJSON(input)
		if valid {
			if err != nil || strictjson.Validate(out) != nil {
				t.Fatalf("valid complete stream rejected: %v", err)
			}
			got := cycle3DecodedJSON(t, out).(map[string]any)
			account := got["native"].(map[string]any)
			if account["id"] != cycle3LiteralID || account["balance"].(map[string]any)["amount"] != d.Amount.String() {
				t.Fatal("native type provenance lost in streaming document")
			}
		} else {
			var pe *ParseError
			if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
				t.Fatal("invalid custom stream bypassed financial document gate")
			}
		}
		if !bytes.Equal(data, before) {
			t.Fatal("stream mutated original custom bytes")
		}
	})
}

func TestModelCycle3NativeUnionCycleStillRejected(t *testing.T) {
	v := OperationDetail{UOHID: cycle3LiteralID}
	v.Fields = []OperationDetailField{{Value: &v}}
	for _, input := range []any{&v, cycle3TypedStreamFallback{Payload: &v}, cycle3TypedStreamContainer{Payload: &v}} {
		data, err := ExportJSON(input)
		var pe *ParseError
		if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
			t.Fatal("native financial provenance bypassed cycle gate")
		}
	}
}
