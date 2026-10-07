package sber

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Source: models.py frozen transfer dataclasses and safe repr contracts.
func TestTransferModelsFormattingAndSerialization(t *testing.T) {
	m, _ := ParseMoney(map[string]any{"amount": "10.50", "currency": "RUB"})
	resource := NewTransferResource("fixture-sensitive-resource", "card", "Карта 4111 1111 1111 1111", "RUB")
	draft := NewTransferDraft("fixture-sensitive-process", "me2meCreate", "transferRequisites", []TransferResource{resource}, []TransferResource{})
	prepared := NewPreparedTransfer(draft.PID(), draft.Flow(), "ready", resource.ID(), "fixture-destination", *m, "fixture-purpose")
	document := "fixture-sensitive-document"
	result := NewTransferResult(draft.PID(), draft.Flow(), "completed", &document)
	for _, v := range []any{resource, draft, prepared, result} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			text := fmt.Sprintf(format, v)
			if strings.Contains(text, "fixture-sensitive") || strings.Contains(text, "fixture-purpose") || strings.Contains(text, "4111 1111") || strings.Contains(text, "10.50") {
				t.Fatalf("unsafe transfer fmt: %s", text)
			}
		}
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "fixture-sensitive") || strings.Contains(string(b), "fixture-purpose") || strings.Contains(string(b), "4111 1111") {
			t.Fatalf("unsafe transfer JSON: %s", b)
		}
	}
	// A page and products preserve nullable fields, all operation financial fields.
	page := OperationsPage{Operations: []Operation{{ID: "fixture", Amount: m, ScopeCardIDs: []string{}}}, NextOffset: nil}
	b, err := json.Marshal(page)
	if err != nil || !strings.Contains(string(b), `"next_offset":null`) || !strings.Contains(string(b), `"amount":"10.50"`) {
		t.Fatalf("page JSON: %s %v", b, err)
	}
	var round OperationsPage
	if err = json.Unmarshal(b, &round); err != nil || !reflect.DeepEqual(page, round) {
		t.Fatalf("page roundtrip: %+v %v", round, err)
	}
}

func TestPANRedactedExportAndPrivateJSON(t *testing.T) {
	amount, _ := ParseDecimal("4111111111111111") // Real financial amount must not be mistaken for PAN text.
	value := map[string]any{"products": Products{Accounts: []Account{{Name: "4111 1111 1111 1111", Balance: &Money{Amount: amount, Currency: "RUB"}}}, Cards: []Card{}}, "nested": []any{map[int]string{7: "PAN 4111-1111-1111-1111"}}, "decimal": amount}
	data, err := ExportJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	account := got["products"].(map[string]any)["accounts"].([]any)[0].(map[string]any)
	if account["name"] != "•••• 1111" || account["balance"].(map[string]any)["amount"] != "4111111111111111" || got["decimal"] != "4111111111111111" || got["nested"].([]any)[0].(map[string]any)["7"] != "PAN •••• 1111" {
		t.Fatalf("bad exported JSON: %s", data)
	}
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err = os.WriteFile(victim, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "result.json")
	if err = os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	if err = WritePrivateJSON(path, value); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("private mode/symlink: %v %v", info, err)
	}
	bytes, err := os.ReadFile(victim)
	if err != nil || string(bytes) != "safe" {
		t.Fatalf("victim changed: %s %v", bytes, err)
	}
	bytes, err = os.ReadFile(path)
	if err != nil || string(bytes) != string(data)+"\n" {
		t.Fatalf("private data mismatch: %v", err)
	}
	if err = WritePrivateJSON(path, make(chan int)); err == nil {
		t.Fatal("unsupported value accepted")
	}
	bytes, _ = os.ReadFile(path)
	if string(bytes) != string(data)+"\n" {
		t.Fatal("failed export replaced prior file")
	}
	names, err := os.ReadDir(dir)
	if err != nil || len(names) != 2 {
		t.Fatalf("leaked temporary files: %v %v", names, err)
	}
	if _, err := JSONable(map[string]any{"bad": make(chan int)}); err == nil {
		t.Fatal("unsupported JSONable type accepted")
	}
}

type pointerOnlySecret struct {
	Password string `json:"password"`
}

func (*pointerOnlySecret) MarshalJSON() ([]byte, error) {
	return []byte(`{"password":"<redacted>"}`), nil
}
func TestExportRespectsPointerSecretMarshaler(t *testing.T) {
	for _, v := range []any{&pointerOnlySecret{Password: "fixture-private-password"}, map[string]any{"credentials": &pointerOnlySecret{Password: "fixture-private-password"}}} {
		b, err := ExportJSON(v)
		if err != nil || strings.Contains(string(b), "fixture-private-password") || !strings.Contains(string(b), "<redacted>") {
			t.Fatalf("pointer credential leak: %s %v", b, err)
		}
	}
	pe := NewParseError("fixture-private-field")
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		if strings.Contains(fmt.Sprintf(format, pe), "fixture-private-field") {
			t.Fatalf("unsafe parse error: %s", fmt.Sprintf(format, pe))
		}
	}
	b, err := json.Marshal(pe)
	if err != nil || strings.Contains(string(b), "fixture-private-field") {
		t.Fatalf("unsafe parse error JSON: %s %v", b, err)
	}
	cycle := map[string]any{}
	cycle["cycle"] = cycle
	if _, err := ExportJSON(cycle); err == nil {
		t.Fatal("cyclic map accepted")
	}
	var p *Money
	if v, err := JSONable(p); err != nil || v != nil {
		t.Fatalf("nil pointer: %v %v", v, err)
	}
}

func TestExactMoneyDecodeEdgeCases(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{{"１２,５０", "12.50"}, {"1_234.50", "1234.50"}, {".50", "0.50"}, {"1.", "1"}, {"12e-2", "0.12"}, {"+0.0000010", "0.0000010"}, {"-00001.2300", "-1.2300"}, {"1234e2", "1.234E+5"}} {
		t.Run(tc.in, func(t *testing.T) {
			d, err := ParseDecimal(tc.in)
			if err != nil || d.String() != tc.want {
				t.Fatalf("decimal %q = %v %v", tc.in, d, err)
			}
		})
	}
	for _, tc := range []struct{ code any }{{json.Number("0.00")}, {int64(0)}, {float32(0)}} {
		t.Run(fmt.Sprint(tc.code), func(t *testing.T) {
			m, err := ParseMoney(map[string]any{"amount": "10", "currencyCode": tc.code, "currency": "USD"})
			if err != nil || m.Currency != "USD" {
				t.Fatalf("zero code fallback: %v %v", m, err)
			}
		})
	}
	for _, text := range []string{`{"amount":"10","currency":"RUB"}{}`, `{"amount":"10","currency":"RUB"}false`} {
		t.Run(text, func(t *testing.T) {
			var m Money
			if err := m.UnmarshalJSON([]byte(text)); err == nil {
				t.Fatalf("trailing money JSON accepted: %s", text)
			}
		})
	}
}

func TestOperationDetailUnionJSONRoundtrip(t *testing.T) {
	r := readDomainGolden(t, "resources", "op_details")
	p, err := DecodeJSON(strings.NewReader(string(r.Data.Input)))
	if err != nil {
		t.Fatal(err)
	}
	original, err := ParseOperationDetails(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var got OperationDetail
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, got) {
		t.Fatalf("typed field union roundtrip: original=%+v got=%+v", original, got)
	}
	// The financial value stays Money in an explicit export, even when Luhn-valid.
	d, _ := ParseDecimal("4111111111111111")
	field := OperationDetailField{Value: &Money{Amount: d, Currency: "RUB"}}
	b, err = ExportJSON(field)
	if err != nil || !strings.Contains(string(b), `"amount": "4111111111111111"`) {
		t.Fatalf("financial union export: %s %v", b, err)
	}
}

type invalidJSONMarshaler struct{ Fail bool }

func (m invalidJSONMarshaler) MarshalJSON() ([]byte, error) {
	if m.Fail {
		return nil, errors.New("fixture-private-error")
	}
	return []byte("not-json"), nil
}
func TestDomainSerializationAndParserErrorPaths(t *testing.T) {
	for _, v := range []any{int8(1), int16(1), int32(1), uint(1), uint8(1), uint16(1), uint32(1), uint64(1), float32(1.25)} {
		if _, err := ParseDecimal(v); err != nil {
			t.Fatalf("native numeric %T: %v", v, err)
		}
	}
	for _, v := range []any{"1e999999999999999999999", "1e-9223372036854775808", "12e9223372036854775807", float32(math.Inf(1))} {
		if _, err := ParseDecimal(v); err == nil {
			t.Fatalf("invalid decimal accepted: %v", v)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `{"amount":true}`, `{"amount":null}`} {
		var m Money
		if err := json.Unmarshal([]byte(raw), &m); err == nil {
			t.Fatalf("bad Money JSON accepted %s", raw)
		}
	}
	var d Decimal
	for _, raw := range []string{`"10.50"`, `9007199254740993.01`} {
		if err := json.Unmarshal([]byte(raw), &d); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{`null`, `true`, `"NaN"`, `{`} {
		if err := d.UnmarshalJSON([]byte(raw)); err == nil {
			t.Fatalf("bad Decimal JSON accepted %s", raw)
		}
	}
	if d.GoString() != d.String() {
		t.Fatal("decimal debug formatting")
	}
	pe := NewParseError("fixture-private-error")
	pe.sberError()
	if pe.String() != pe.Error() || pe.GoString() != pe.Error() {
		t.Fatal("static error strings")
	}
	for _, v := range []any{invalidJSONMarshaler{}, invalidJSONMarshaler{Fail: true}, math.Inf(1), json.Number("oops"), []any{make(chan int)}, struct{ Unsupported chan int }{make(chan int)}} {
		if _, err := ExportJSON(v); err == nil {
			t.Fatalf("bad export type accepted: %T", v)
		}
	}
	private, _ := ParseDecimal("4111111111111111")
	if b, err := ExportJSON(&private); err != nil || string(b) != `"4111111111111111"` {
		t.Fatalf("Decimal pointer export %s %v", b, err)
	}
	for _, v := range []any{true, int(1), uint(1), float64(1.2), [1]string{"PAN 4111 1111 1111 1111"}, (*int)(nil), struct {
		Skip    string `json:"-"`
		Empty   string `json:"empty,omitempty"`
		Value   string
		private string
	}{Skip: "ignored", Value: "safe", private: "never"}} {
		if _, err := ExportJSON(v); err != nil {
			t.Fatalf("native export %T: %v", v, err)
		}
	}
	dir := t.TempDir()
	if err := WritePrivateJSON(filepath.Join(dir, "missing", "file.json"), map[string]any{}); err == nil {
		t.Fatal("missing directory accepted")
	}
	target := filepath.Join(dir, "directory")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := WritePrivateJSON(target, map[string]any{}); err == nil {
		t.Fatal("directory target replaced")
	}
	names, _ := os.ReadDir(dir)
	if len(names) != 1 {
		t.Fatalf("failure temporary leaked %v", names)
	}
	var field OperationDetailField
	for _, raw := range []string{`true`, `{"value":true}`, `{"value":{"amount":"NaN"}}`} {
		if err := json.Unmarshal([]byte(raw), &field); err == nil {
			t.Fatalf("bad detail field accepted %s", raw)
		}
	}
}

func FuzzExactDecimalRoundtrip(f *testing.F) {
	for _, s := range []string{"0", "-0.00", "12,50", "9007199254740993.01", "1234e2", "1e-10000", "NaN", "１２.５０", "1_234.50"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := ParseDecimal(s)
		if err != nil {
			return
		}
		text := d.String()
		again, err := ParseDecimal(text)
		if err != nil || again != d {
			t.Fatalf("decimal roundtrip %q -> %q %v", s, text, err)
		}
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		var round Decimal
		if err = json.Unmarshal(b, &round); err != nil || round != d {
			t.Fatalf("JSON roundtrip %q %v", s, err)
		}
	})
}

func TestDecimalNativeAndStrictUnmarshal(t *testing.T) {
	original, _ := ParseDecimal("9007199254740993.01")
	for _, v := range []any{original, &original} {
		t.Run(fmt.Sprintf("%T", v), func(t *testing.T) {
			got, err := ParseDecimal(v)
			if err != nil || got != original {
				t.Fatalf("native decimal %T -> %v %v", v, got, err)
			}
			m, err := ParseMoney(map[string]any{"amount": v, "currency": "USD"})
			if err != nil || m.Amount != original {
				t.Fatalf("native Money %T -> %v %v", v, m, err)
			}
		})
	}
	for _, raw := range []string{`"10.50"false`, `12 34`} {
		t.Run(raw, func(t *testing.T) {
			var d Decimal
			if err := d.UnmarshalJSON([]byte(raw)); err == nil {
				t.Fatalf("trailing Decimal JSON accepted: %s", raw)
			}
		})
	}
}

// Source: models.py:_decimal/_money; regressions.py:test_a20_currency_null_falls_back.
func TestExactMoneyCanonicalJSON(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want string
	}{
		{"-12,50", "-12.50"}, {json.Number("9007199254740993.01"), "9007199254740993.01"},
		{"000.50", "0.50"}, {"1e2", "1E+2"}, {"1e-7", "1E-7"}, {"-0.00", "-0.00"},
		{int64(12), "12"}, {0.1, "0.1"},
	} {
		m, err := ParseMoney(map[string]any{"amount": tc.in, "currencyCode": nil, "currency": map[string]any{"code": "RUB", "name": "руб."}})
		if err != nil || m == nil {
			t.Fatalf("ParseMoney(%v): %v", tc.in, err)
		}
		if m.Amount.String() != tc.want || m.Currency != "RUB" {
			t.Fatalf("%v -> %+v, want %s RUB", tc.in, m, tc.want)
		}
		b, err := json.Marshal(m)
		if err != nil || string(b) != `{"amount":"`+tc.want+`","currency":"RUB"}` {
			t.Fatalf("json=%s err=%v", b, err)
		}
		var round Money
		if err = json.Unmarshal(b, &round); err != nil || round.Amount.String() != tc.want {
			t.Fatalf("roundtrip: %+v %v", round, err)
		}
	}
	for _, in := range []any{true, false, nil, "NaN", "Infinity", "-Infinity", "--1", "1,2,3", "oops", math.Inf(1), math.NaN(), []any{1}, map[string]any{}} {
		_, err := ParseDecimal(in)
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Fatalf("%v: want ParseError got %v", in, err)
		}
	}
	for _, in := range []any{nil, map[string]any{}} {
		m, err := ParseMoney(in)
		if err != nil || m != nil {
			t.Fatalf("absent money: %v %v", m, err)
		}
	}
	for _, in := range []any{nil, true, "garbage"} {
		_, err := ParseMoney(map[string]any{"amount": in})
		if err == nil {
			t.Fatalf("explicit invalid amount %v accepted", in)
		}
	}
	m, err := ParseMoney(map[string]any{"amount": "10", "currencyCode": []string{"RUB"}})
	if err != nil || m.Currency != "" {
		t.Fatalf("currency type coercion: %+v %v", m, err)
	}
	var n Money
	if err = json.Unmarshal([]byte(`{"amount":9007199254740993.01,"currency":"USD"}`), &n); err != nil || n.Amount.String() != "9007199254740993.01" {
		t.Fatalf("exact unmarshal: %+v %v", n, err)
	}
	if (Decimal{}).String() != "0" {
		t.Fatal("zero value must be exact zero")
	}
}
