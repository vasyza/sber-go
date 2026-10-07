package sber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	sber "github.com/vasyza/sber-go"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

const independentID = "4111111111111111"
const independentAmount = "4111111111111111.50"
const independentPAN = "PAN 4111-1111-1111-1111"
const independentMasked = "PAN •••• 1111"

func independentDecode(t *testing.T, b []byte) any {
	t.Helper()
	if !utf8.Valid(b) {
		t.Fatal("invalid output UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		t.Fatal(err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		t.Fatalf("not a complete document: %v", err)
	}
	return v
}

func independentAssert(t *testing.T, input, expected any) {
	t.Helper()
	want, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	out, err := sber.ExportJSON(input)
	if err != nil {
		t.Errorf("ExportJSON(%T) rejected finite expected shape: %v", input, err)
		return
	}
	if !reflect.DeepEqual(independentDecode(t, out), independentDecode(t, want)) {
		t.Errorf("ExportJSON(%T) shape/provenance: got=%s want=%s", input, out, want)
	}
	normalized, err := sber.JSONable(input)
	if err != nil {
		t.Errorf("JSONable(%T) rejected: %v", input, err)
		return
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Errorf("normalization contains serializer/error: %v", err)
		return
	}
	if !reflect.DeepEqual(independentDecode(t, encoded), independentDecode(t, want)) {
		t.Errorf("JSONable(%T) shape/provenance: got=%s want=%s", input, encoded, want)
	}
}

func independentReject(t *testing.T, input any) {
	t.Helper()
	out, err := sber.ExportJSON(input)
	var pe *sber.ParseError
	if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
		t.Errorf("ExportJSON(%T) not fail-closed: data=%s err=%v", input, out, err)
	}
	normalized, err := sber.JSONable(input)
	if normalized != nil || !errors.As(err, &pe) || errors.Unwrap(err) != nil {
		t.Errorf("JSONable(%T) not eager/static: %#v %v", input, normalized, err)
	}
	if err != nil && (strings.Contains(err.Error(), "independent-private") || strings.Contains(err.Error(), "native-invalid")) {
		t.Error("original input/cause escaped in error")
	}
}

func independentMoney(t *testing.T, amount, currency string) *sber.Money {
	t.Helper()
	d, err := sber.ParseDecimal(amount)
	if err != nil {
		t.Fatal(err)
	}
	return &sber.Money{Amount: d, Currency: currency}
}

type independentRequester struct {
	sber.BusinessRequester
	Secret       string
	MarshalCalls int
	Reads        []map[string]any
	Response     map[string]any
}

func (r *independentRequester) MarshalJSON() ([]byte, error) {
	r.MarshalCalls++
	panic("must not traverse private requester")
}
func (r *independentRequester) PostRead(_ context.Context, _ string, payload map[string]any) (map[string]any, error) {
	r.Reads = append(r.Reads, payload)
	return r.Response, nil
}

type independentStream struct {
	Payload any
	Raw     []byte
}

func (v independentStream) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("~reached/0")); err != nil {
		return err
	}
	if err := jsonv2.MarshalEncode(e, v.Payload); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("foreign")); err != nil {
		return err
	}
	if v.Raw == nil {
		if err := e.WriteToken(jsontext.Null); err != nil {
			return err
		}
	} else if err := e.WriteValue(v.Raw); err != nil {
		return err
	}
	return e.WriteToken(jsontext.EndObject)
}
func (independentStream) MarshalJSON() ([]byte, error) { panic("legacy loses to stream") }

type independentFallback struct {
	Payload  any   `json:"payload"`
	Optional []any `json:"optional,omitempty"`
}

func (independentFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }

type independentRaw []byte

func (r independentRaw) MarshalJSON() ([]byte, error) { return []byte(r), nil }

type independentLegacySecret struct {
	sber.BankAccount
	Private  any
	Password string
}

func (independentLegacySecret) MarshalJSON() ([]byte, error) {
	return []byte(`{"redacted":true,"amount":"4111111111111111.50","id":"4111111111111111"}`), nil
}

type independentMixedSecret struct {
	sber.BankCard
	Password string
	Private  any
	Calls    *[4]int
}

func (s *independentMixedSecret) MarshalJSONTo(e *jsontext.Encoder) error {
	s.Calls[0]++
	return e.WriteToken(jsontext.String("<stream-redacted>"))
}
func (s independentMixedSecret) MarshalJSON() ([]byte, error) {
	s.Calls[1]++
	return []byte(`"<legacy-redacted>"`), nil
}
func (s *independentMixedSecret) AppendText(b []byte) ([]byte, error) {
	s.Calls[2]++
	return append(b, "<append-redacted>"...), nil
}
func (s independentMixedSecret) MarshalText() ([]byte, error) {
	s.Calls[3]++
	return []byte("<text-redacted>"), nil
}

type independentTextSecret struct {
	sber.BankAccount
	Private  any
	Password string
}

func (*independentTextSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, "<append-redacted>"...), nil
}

// Explicit JSON method overrides the inherited snapshot redactor.
func (s *independentTextSecret) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteToken(jsontext.String("<inherited-wrapper-redacted>"))
}

type independentTextOnly struct {
	Private  any
	Password string
}

func (independentTextOnly) MarshalText() ([]byte, error) { return []byte("text " + independentID), nil }

type independentAlias = sber.Account
type independentDerived sber.Account

func TestIndependentCycle3ReachedFinancialContainers(t *testing.T) {
	m := independentMoney(t, independentAmount, independentID)
	id, empty := independentID, ""
	a := sber.Account{ID: id, Name: independentPAN, Last4: "1111", State: "OPEN", Balance: m, Kind: "ctaccount", Hidden: true}
	c := sber.Card{ID: id, Name: independentPAN, Last4: "1111", Balance: m, AccountID: &id, AccountBalance: m, BalanceSource: &empty, IsMain: true}
	r := &independentRequester{Secret: "independent-private\xff"}
	raw := sber.Products{Accounts: []sber.Account{a}, Cards: []sber.Card{c}}
	p := sber.NewBankPortfolio(raw, r)
	ba, bc := p.Accounts()[0], p.Cards()[0]
	wa, wc := a, c
	wa.Name, wc.Name = independentMasked, independentMasked
	want := sber.Products{Accounts: []sber.Account{wa}, Cards: []sber.Card{wc}}
	rawBefore, _ := json.Marshal(raw)
	for _, tc := range []struct {
		name     string
		in, want any
	}{
		{"raw_account", a, wa}, {"raw_card", c, wc}, {"raw_portfolio", raw, want},
		{"account_pointer", ba, wa}, {"account_value", *ba, wa}, {"card_pointer", bc, wc}, {"card_value", *bc, wc},
		{"portfolio_pointer", p, want}, {"portfolio_value", *p, want},
		{"nested_mixed", []any{[2]any{ba, *bc}, map[string]any{"p": p}}, []any{[]any{wa, wc}, map[string]any{"p": want}}},
		{"addressable_field", &struct {
			Value sber.BankCard `json:"value"`
		}{*bc}, map[string]any{"value": wc}},
		{"value_array", [2]sber.BankAccount{*ba, *ba}, []any{wa, wa}},
		{"array_pointer", &[2]sber.BankPortfolio{*p, *p}, []any{want, want}},
		{"typed_nil_interface", struct {
			Value any `json:"value"`
		}{(*sber.BankAccount)(nil)}, map[string]any{"value": nil}},
		{"typed_nil_money", (*sber.Money)(nil), nil}, {"nil_decimal", (*sber.Decimal)(nil), nil},
		{"nil_portfolio_lists", sber.NewBankPortfolio(sber.Products{}, r), sber.Products{}},
		{"empty_portfolio_lists", sber.NewBankPortfolio(sber.Products{Accounts: []sber.Account{}, Cards: []sber.Card{}}, r), sber.Products{Accounts: []sber.Account{}, Cards: []sber.Card{}}},
		{"alias_exact_schema", independentAlias(a), wa},
		{"derived_not_exact_schema", independentDerived(a), map[string]any{"id": "•••• 1111", "name": independentMasked, "last4": "1111", "state": "OPEN", "hidden": true, "arrested": false, "balance": m, "kind": "ctaccount"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			independentAssert(t, tc.in, tc.want)
			independentAssert(t, independentStream{Payload: tc.in}, map[string]any{"~reached/0": tc.want, "foreign": nil})
			independentAssert(t, independentFallback{Payload: tc.in, Optional: []any{}}, map[string]any{"payload": tc.want})
		})
	}
	after, _ := json.Marshal(raw)
	if !bytes.Equal(rawBefore, after) || r.MarshalCalls != 0 || len(r.Reads) != 0 || bc.Account() != ba || ba.Cards()[0] != bc {
		t.Fatal("export mutated snapshot or reached private binding/requester")
	}
	copy := ba.Snapshot()
	copy.Balance.Currency = "mutated"
	copy.Balance.Amount = sber.Decimal{}
	cc := bc.Snapshot()
	*cc.AccountID = "changed"
	*cc.BalanceSource = "changed"
	cc.AccountBalance.Currency = "changed"
	if ba.Balance().Currency != id || bc.AccountID() == nil || *bc.AccountID() != id || *bc.BalanceSource() != "" {
		t.Fatal("defensive copy integrity lost")
	}
}

func TestIndependentCycle3ForeignCredentialMethodAuthority(t *testing.T) {
	bad := sber.NewBankAccount(sber.Account{ID: "native-invalid\xff"}, nil, nil)
	c := sber.NewBankCard(sber.Card{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	calls := [4]int{}
	mixed := independentMixedSecret{BankCard: *c, Password: "independent-private\xff", Private: bad, Calls: &calls}
	expected := map[string]any{"redacted": true, "amount": "•••• 1111.50", "id": "•••• 1111"}
	for _, tc := range []struct {
		name     string
		in, want any
	}{
		{"legacy_embedding", independentLegacySecret{BankAccount: *bad, Private: bad, Password: "independent-private\xff"}, expected},
		{"stream_all_methods", &mixed, "<stream-redacted>"},
		{"addressable_struct", &struct {
			Value independentMixedSecret `json:"value"`
		}{mixed}, map[string]any{"value": "<stream-redacted>"}},
		{"addressable_slice", []independentMixedSecret{mixed}, []any{"<stream-redacted>"}},
		{"addressable_array", &[1]independentMixedSecret{mixed}, []any{"<stream-redacted>"}},
		{"nonaddressable_legacy", mixed, "<legacy-redacted>"},
		{"text_native_union", independentTextOnly{Private: bad, Password: "independent-private\xff"}, "text •••• 1111"},
		{"override_promoted_entity", &independentTextSecret{BankAccount: *bad, Private: bad, Password: "independent-private\xff"}, "<inherited-wrapper-redacted>"},
		{"named_raw_custom", independentRaw(`{"currency":"4111111111111111","amount":"4111111111111111.50","id":"4111111111111111"}`), map[string]any{"currency": "•••• 1111", "amount": "•••• 1111.50", "id": "•••• 1111"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			independentAssert(t, tc.in, tc.want)
			independentAssert(t, map[string]any{"v": tc.in}, map[string]any{"v": tc.want})
			// Package V1 semantics keep non-addressable value method behavior.
			independentAssert(t, independentStream{Payload: tc.in}, map[string]any{"~reached/0": tc.want, "foreign": nil})
		})
	}
	if calls[2] != 0 || calls[3] != 0 || calls[0] == 0 || calls[1] == 0 {
		t.Fatal("serializer precedence not exercised")
	}
}

func TestIndependentCycle3NativeInvalidUnicodeAndLiteralReplacement(t *testing.T) {
	for _, text := range []string{"native-invalid\xff", "native-invalid\xfe", "literal � 😀"} {
		m := independentMoney(t, independentAmount, text)
		a := sber.Account{ID: text, Kind: text, Balance: m}
		c := sber.Card{ID: text, AccountID: &text, BalanceSource: &text, Balance: m, AccountBalance: m}
		p := sber.NewBankPortfolio(sber.Products{Accounts: []sber.Account{a}, Cards: []sber.Card{c}}, nil)
		for i, input := range []any{m, a, c, sber.Resource{ID: text, Type: text}, sber.Operation{ID: text, ScopeCardIDs: []string{text}}, p, p.Accounts()[0], p.Cards()[0]} {
			t.Run(fmt.Sprintf("text_%x/shape_%d", text, i), func(t *testing.T) {
				shapes := []any{input, []any{input}, map[string]any{"v": input}, independentStream{Payload: input}, independentFallback{Payload: input}}
				for _, shape := range shapes {
					if utf8.ValidString(text) {
						if _, err := sber.ExportJSON(shape); err != nil {
							t.Errorf("valid replacement text rejected: %v", err)
						}
					} else {
						independentReject(t, shape)
					}
				}
			})
		}
		for _, v := range []json.Marshaler{p, *p, p.Accounts()[0], *p.Accounts()[0], p.Cards()[0], *p.Cards()[0]} {
			direct, err := v.MarshalJSON()
			ordinary, oe := json.Marshal(v)
			if utf8.ValidString(text) {
				if err != nil || oe != nil || !bytes.Equal(direct, ordinary) {
					t.Error("ordinary literal U+FFFD control failed")
				}
			} else {
				var pe *sber.ParseError
				if direct != nil || ordinary != nil || !errors.As(err, &pe) || !errors.As(oe, &pe) || pe.Field() != "financial_snapshot" || errors.Unwrap(pe) != nil {
					t.Error("ordinary/direct entity marshal repaired invalid native text")
				}
			}
		}
	}
}

func TestIndependentCycle3OriginalDocumentsAndNumericLexemes(t *testing.T) {
	bad := [][]byte{
		[]byte(`{"x":1,"\u0078":2}`), []byte(`{"ignored":{"x":1,"\u0078":2}}`), []byte(`[{"x":1,"x":2}]`),
		[]byte(`"\ud800"`), []byte(`{"\udfff":1}`), []byte(`{"id":"\ud800\ud800"}`), {'"', 0xff, '"'},
		[]byte(`{"x":1} false`), []byte(`01`), []byte(`+1`), []byte(`1.`), []byte(`1e`), []byte(`{"4111 1111 1111 1111":1,"4111-1111-1111-1111":2}`),
	}
	for i, raw := range bad {
		t.Run(fmt.Sprintf("rejected_%d", i), func(t *testing.T) {
			before := bytes.Clone(raw)
			for _, shape := range []any{independentRaw(raw), json.RawMessage(raw), []any{independentRaw(raw)}, independentStream{Payload: sber.Account{ID: independentID}, Raw: raw}} {
				independentReject(t, shape)
			}
			if !bytes.Equal(before, raw) {
				t.Error("original document mutated")
			}
		})
	}
	raw := []byte(`{"n":9007199254740993.0100,"tiny":1.2300e-1000,"zero":-0.00,"p":"\ud83d\ude00","replacement":"�","display":"4111111111111111"}`)
	expected := map[string]any{"n": json.Number("9007199254740993.0100"), "tiny": json.Number("1.2300e-1000"), "zero": json.Number("-0.00"), "p": "😀", "replacement": "�", "display": "•••• 1111"}
	independentAssert(t, independentRaw(raw), expected)
	independentAssert(t, independentStream{Payload: sber.Resource{ID: independentID}, Raw: raw}, map[string]any{"~reached/0": sber.Resource{ID: independentID}, "foreign": expected})
	for _, token := range []string{"+1", "01", "1.", " 1", "1 ", "NaN", "1e", "1_0", "１２"} {
		independentReject(t, json.Number(token))
	}
	for _, token := range []string{"-0.00", "9007199254740993.0100", "1.2300e-1000", "1e9999999999999999999999"} {
		independentAssert(t, json.Number(token), json.Number(token))
	}
	for _, doc := range [][]byte{[]byte(`{"amount":"1.00","ignored":{"x":1,"\u0078":2}}`), []byte(`{"amount":"1.00","currency":"\ud800"}`), []byte(`{"amount":"1.00"} null`)} {
		target := *independentMoney(t, "-0.00", "unchanged")
		before := target
		if target.UnmarshalJSON(doc) == nil || target != before {
			t.Error("Money direct unmarshal collapsed original invalid input or changed receiver")
		}
		if json.Unmarshal(doc, &target) == nil || target != before {
			t.Error("Money standard unmarshal changed receiver")
		}
		field := sber.OperationDetailField{Name: "unchanged", Value: "unchanged"}
		old := field
		if field.UnmarshalJSON(doc) == nil || !reflect.DeepEqual(field, old) {
			t.Error("union direct unmarshal lost invalid original document")
		}
	}
}

type independentMutationFallback struct{ Payload any }

func (independentMutationFallback) MarshalJSONTo(e *jsontext.Encoder) error {
	_ = e.WriteToken(jsontext.Null)
	return errors.ErrUnsupported
}

type independentFailureFloat float64

func (independentFailureFloat) MarshalJSONTo(*jsontext.Encoder) error {
	return errors.New("independent-private-error")
}

func TestIndependentCycle3FallbackMutationAndDeferredBoundary(t *testing.T) {
	independentReject(t, independentMutationFallback{Payload: sber.Account{ID: independentID}})
	f := independentFailureFloat(1.25)
	normalized, err := sber.JSONable(f)
	if err != nil || normalized == nil || reflect.TypeOf(normalized) == reflect.TypeOf(f) {
		t.Fatal("deferred marker lost or retains original scalar")
	}
	b, err := json.Marshal(normalized)
	var pe *sber.ParseError
	if b != nil || !errors.As(err, &pe) || strings.Contains(err.Error(), "independent-private") {
		t.Fatal("deferred result not static failure")
	}
	b, err = sber.ExportJSON(f)
	if b != nil || !errors.As(err, &pe) || errors.Unwrap(err) != nil {
		t.Fatal("deferred marker became successful export")
	}
	independentReject(t, &f)
	independentReject(t, independentStream{Payload: f})
	independentReject(t, independentFailureFloat(math.NaN()))
}

func TestIndependentCycle3FiniteSliceAliasesAreNotCycles(t *testing.T) {
	a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	wantA := a.Snapshot()
	finite := make([]any, 2)
	finite[0] = a
	finite[1] = finite[:1]
	empty := make([]any, 2)
	empty[0] = empty[:0]
	empty[1] = a
	for _, tc := range []struct {
		name     string
		in, want any
	}{
		{"prefix_view", finite, []any{wantA, []any{wantA}}},
		{"empty_view", empty, []any{[]any{}, wantA}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.in)
			if err != nil || !reflect.DeepEqual(independentDecode(t, encoded), independentDecode(t, mustIndependentJSON(t, tc.want))) {
				t.Fatal("invalid independent finite-alias oracle")
			}
			independentAssert(t, tc.in, tc.want)
			independentAssert(t, independentStream{Payload: tc.in}, map[string]any{"~reached/0": tc.want, "foreign": nil})
		})
	}
	cycle := make([]any, 1)
	cycle[0] = cycle
	independentReject(t, cycle)
	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	independentReject(t, cyclicMap)
	op := sber.OperationDetail{UOHID: independentID}
	op.Fields = []sber.OperationDetailField{{Value: &op}}
	independentReject(t, &op)
}
func mustIndependentJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestIndependentCycle3UnknownStructKeyDisplayBoundary(t *testing.T) {
	input := struct {
		Value string `json:"4111111111111111"`
	}{Value: independentPAN}
	expected := map[string]any{"•••• 1111": independentMasked}
	independentAssert(t, input, expected)
	independentAssert(t, map[string]any{independentID: independentPAN}, expected)
	independentAssert(t, independentRaw(`{"4111111111111111":"PAN 4111-1111-1111-1111"}`), expected)
	independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
}

func TestIndependentCycle3IndependentSourceOracle(t *testing.T) {
	b, err := os.ReadFile("../../testdata/financial/independent-oracle.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Records []struct {
			Kind     string
			Raw      json.RawMessage
			Expected json.RawMessage
		}
		Decimals []struct {
			Input    string
			Valid    bool
			Expected string
		}
		Display []struct{ Input, Expected string }
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	for i, row := range fixture.Records {
		t.Run(fmt.Sprintf("%03d_%s", i, row.Kind), func(t *testing.T) {
			var native any
			switch row.Kind {
			case "Account", "BankAccount":
				native = &sber.Account{}
			case "Card", "BankCard":
				native = &sber.Card{}
			case "Products", "BankPortfolioRaw":
				native = &sber.Products{}
			case "Money":
				native = &sber.Money{}
			case "Resource":
				native = &sber.Resource{}
			case "Operation":
				native = &sber.Operation{}
			case "OperationDetail":
				native = &sber.OperationDetail{}
			default:
				t.Fatalf("unknown oracle kind %s", row.Kind)
			}
			if err := json.Unmarshal(row.Raw, native); err != nil {
				t.Fatal(err)
			}
			switch row.Kind {
			case "BankAccount":
				native = sber.NewBankAccount(*(native.(*sber.Account)), nil, nil)
			case "BankCard":
				native = sber.NewBankCard(*(native.(*sber.Card)), nil, nil)
			case "BankPortfolioRaw":
				native = sber.NewBankPortfolio(*(native.(*sber.Products)), nil)
			}
			want := independentDecode(t, row.Expected)
			independentAssert(t, native, want)
			independentAssert(t, independentStream{Payload: native}, map[string]any{"~reached/0": want, "foreign": nil})
			independentAssert(t, independentFallback{Payload: native}, map[string]any{"payload": want})
		})
	}
	for i, row := range fixture.Decimals {
		t.Run(fmt.Sprintf("decimal_%03d", i), func(t *testing.T) {
			d, err := sber.ParseDecimal(row.Input)
			if (err == nil) != row.Valid {
				t.Fatalf("canonical Decimal acceptance: %q -> %v", row.Input, err)
			}
			if row.Valid && d.String() != row.Expected {
				t.Fatalf("canonical exact Decimal: %q -> %q want %q", row.Input, d.String(), row.Expected)
			}
		})
	}
	for i, row := range fixture.Display {
		t.Run(fmt.Sprintf("display_%03d", i), func(t *testing.T) {
			if got := sber.RedactPAN(row.Input); got != row.Expected {
				t.Errorf("source PAN policy mismatch input=%q got=%q want=%q", row.Input, got, row.Expected)
			}
			independentAssert(t, row.Input, row.Expected)
		})
	}
}
