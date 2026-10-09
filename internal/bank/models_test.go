package bank

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/vasyza/sber-sdk/internal/strictjson"
)

// All values in this file are synthetic. Integrity is checked on the original
// document, never on a lossy decode -> marshal round trip.

type cycle2StreamSecret struct {
	Password string `json:"password"`
	Calls    int    `json:"-"`
}

func (v *cycle2StreamSecret) MarshalJSONTo(e *jsontext.Encoder) error {
	v.Calls++
	return e.WriteToken(jsontext.String("<stream-redacted>"))
}

// Pointer streaming methods must take precedence over a value legacy method.
// Its intentionally unsafe output is a synthetic precedence control only.
func (cycle2StreamSecret) MarshalJSON() ([]byte, error) {
	return []byte(`"fixture-legacy-private"`), nil
}

type cycle2StreamFloat float64

func (cycle2StreamFloat) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteValue([]byte(`{"display":"PAN 4111-1111-1111-1111","n":9007199254740993.0100,"tiny":1.2300e-1000,"unicode":"\ud83d\ude00","replacement":"�"}`))
}

type cycle2StreamRaw struct{ Data []byte }

func (v cycle2StreamRaw) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteValue(v.Data)
}

type cycle2StreamFallback struct {
	Money   Money   `json:"money"`
	Decimal Decimal `json:"decimal"`
	Display string  `json:"display"`
}

func (cycle2StreamFallback) MarshalJSONTo(*jsontext.Encoder) error {
	return errors.ErrUnsupported
}

type cycle2StreamLegacyFallback struct{ Document []byte }

func (cycle2StreamLegacyFallback) MarshalJSONTo(*jsontext.Encoder) error {
	return errors.ErrUnsupported
}

func (v cycle2StreamLegacyFallback) MarshalJSON() ([]byte, error) { return v.Document, nil }

type cycle2StreamMutation struct{}

func (cycle2StreamMutation) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.True); err != nil {
		return err
	}
	return errors.ErrUnsupported
}

type cycle2StreamEmpty struct{}

func (cycle2StreamEmpty) MarshalJSONTo(*jsontext.Encoder) error { return nil }

type cycle2StreamMany struct{}

func (cycle2StreamMany) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.True); err != nil {
		return err
	}
	return e.WriteToken(jsontext.False)
}

type cycle2StreamFloatFallback float64

func (cycle2StreamFloatFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }

type cycle2StreamTextFallback int

func (cycle2StreamTextFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }

func (cycle2StreamTextFallback) MarshalText() ([]byte, error) { return []byte("text 😀 �"), nil }

type cycle2StreamContainer []Money

func (v cycle2StreamContainer) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("money/~value")); err != nil {
		return err
	}
	if err := jsonv2.MarshalEncode(e, []Money(v)); err != nil {
		return err
	}
	return e.WriteToken(jsontext.EndObject)
}

func cycle2AssertExportError(t *testing.T, input any) {
	t.Helper()
	out, err := ExportJSON(input)
	var pe *ParseError
	if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" || errors.Unwrap(err) != nil {
		t.Fatalf("unsafe export or retained private cause: %s %v", out, err)
	}
}

func TestModelCycle2StreamingRedactorAddressabilityPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input any
	}{
		{"pointer", &cycle2StreamSecret{Password: "fixture-private-password"}},
		{"struct_field", &struct {
			Credentials cycle2StreamSecret `json:"credentials"`
		}{Credentials: cycle2StreamSecret{Password: "fixture-private-password"}}},
		{"slice_element", []cycle2StreamSecret{{Password: "fixture-private-password"}}},
		{"array_element", &[1]cycle2StreamSecret{{Password: "fixture-private-password"}}},
		{"map_pointer", map[string]any{"credentials": &cycle2StreamSecret{Password: "fixture-private-password"}}},
		{"interface_field", &struct {
			Credentials any `json:"credentials"`
		}{Credentials: &cycle2StreamSecret{Password: "fixture-private-password"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standard, err := json.Marshal(tc.input)
			if err != nil || !strings.Contains(string(standard), "stream-redacted") || strings.Contains(string(standard), "fixture-private") || strings.Contains(string(standard), "fixture-legacy-private") {
				t.Fatal("invalid synthetic standard-serializer precedence control")
			}
			normalized, err := JSONable(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			out, err := json.Marshal(normalized)
			if err != nil || !strings.Contains(string(out), "stream-redacted") || strings.Contains(string(out), "fixture-private") || strings.Contains(string(out), "fixture-legacy-private") {
				t.Fatalf("streaming redaction boundary bypassed: %s %v", out, err)
			}
			out, err = ExportJSON(tc.input)
			if err != nil || strictjson.Validate(out) != nil || !strings.Contains(string(out), "stream-redacted") || strings.Contains(string(out), "fixture-private") || strings.Contains(string(out), "fixture-legacy-private") {
				t.Fatalf("streaming export boundary bypassed: %s %v", out, err)
			}
		})
	}
	var nilSecret *cycle2StreamSecret
	if out, err := ExportJSON(nilSecret); err != nil || string(out) != "null" {
		t.Fatal("nil custom serializer invoked")
	}
	secret := &cycle2StreamSecret{Password: "fixture-private-password"}
	normalized, err := JSONable(secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = json.Marshal(normalized); err != nil || secret.Calls != 1 {
		t.Fatal("original custom method re-entered final encoding")
	}
}

func TestModelCycle2StreamingPrimitiveDocumentNormalization(t *testing.T) {
	for _, input := range []any{cycle2StreamFloat(1.25), &[]cycle2StreamFloat{1.25}, map[string]cycle2StreamFloat{"fixture": 1.25}} {
		out, err := ExportJSON(input)
		if err != nil || strictjson.Validate(out) != nil || strings.Contains(string(out), "4111-1111") || !strings.Contains(string(out), "•••• 1111") || !strings.Contains(string(out), "9007199254740993.0100") || !strings.Contains(string(out), "1.2300e-1000") || !strings.Contains(string(out), "😀") || !strings.Contains(string(out), "�") {
			t.Fatalf("named primitive custom document not normalized or exact numbers lost: %s %v", out, err)
		}
	}
}

func TestModelCycle2StreamingCompleteDocumentIntegrity(t *testing.T) {
	invalidUTF8 := append([]byte(`{"id":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"decoded_duplicate", []byte(`{"amount":true,"\u0061mount":"1.00"}`)},
		{"nested_duplicate", []byte(`{"extra":[{"id":1,"\u0069d":2}]}`)},
		{"invalid_utf8", invalidUTF8},
		{"unpaired_high", []byte(`{"currency":"\ud800"}`)},
		{"unpaired_low", []byte(`"\udfff"`)},
		{"reversed_pair", []byte(`"\udfff\ud800"`)},
		{"trailing", []byte(`{"safe":true}false`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := bytes.Clone(tc.data)
			cycle2AssertExportError(t, cycle2StreamRaw{Data: data})
			cycle2AssertExportError(t, cycle2StreamLegacyFallback{Document: data})
			if !bytes.Equal(data, tc.data) {
				t.Fatal("custom serializer input mutated")
			}
		})
	}
	cycle2AssertExportError(t, cycle2StreamMutation{})
	cycle2AssertExportError(t, cycle2StreamEmpty{})
	cycle2AssertExportError(t, cycle2StreamMany{})
}

func TestModelCycle2StreamingUnsupportedFallbackPreservesFinancialTypes(t *testing.T) {
	amount, err := ParseDecimal("4111111111111111.00")
	if err != nil {
		t.Fatal(err)
	}
	money := Money{Amount: amount, Currency: "RUB"}
	for _, input := range []any{
		cycle2StreamFallback{Money: money, Decimal: amount, Display: "PAN 4111-1111-1111-1111"},
		&cycle2StreamFallback{Money: money, Decimal: amount, Display: "PAN 4111-1111-1111-1111"},
		cycle2StreamContainer{money, money},
	} {
		out, err := ExportJSON(input)
		if err != nil || strictjson.Validate(out) != nil || strings.Count(string(out), "4111111111111111.00") != 2 || strings.Contains(string(out), "4111-1111") {
			t.Fatalf("documented streaming fallback suppressed/redacted native financial types: %s %v", out, err)
		}
		if _, customContainer := input.(cycle2StreamContainer); customContainer && !strings.Contains(string(out), `"money/~value"`) {
			t.Fatal("custom financial container shape was bypassed")
		}
	}
	for _, tc := range []struct {
		input any
		want  string
	}{
		{cycle2StreamFloatFallback(1.25), "1.25"},
		{cycle2StreamTextFallback(1), `"text 😀 �"`},
	} {
		out, err := ExportJSON(tc.input)
		if err != nil || string(out) != tc.want {
			t.Fatalf("official sentinel fallback precedence changed: %s %v", out, err)
		}
	}
	out, err := ExportJSON(cycle2StreamLegacyFallback{Document: []byte(`{"display":"4111-1111-1111-1111","n":9007199254740993.0100}`)})
	if err != nil || strings.Contains(string(out), "4111-1111") || !strings.Contains(string(out), "9007199254740993.0100") {
		t.Fatalf("legacy fallback not validated/normalized: %s %v", out, err)
	}
}

type cycle2NativeKey string

type cycle2FormattedKey string

func (v cycle2FormattedKey) String() string {
	return "fixture-format-" + string(v)
}

type cycle2LossyKey string

func (v cycle2LossyKey) String() string {
	return strings.ToValidUTF8(string(v), "�")
}

type cycle2LossyStructKey struct{ ID string }

func (v cycle2LossyStructKey) String() string {
	return strings.ToValidUTF8(v.ID, "�")
}

func TestModelCycle2NativeUTF8Identity(t *testing.T) {
	invalid := "fixture-" + string([]byte{0xff})
	amount, err := ParseDecimal("9007199254740993.0100")
	if err != nil {
		t.Fatal(err)
	}
	invalidMoney := Money{Amount: amount, Currency: invalid}
	invalidTagType := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeFor[string](), Tag: reflect.StructTag("json:\"" + invalid + "\"")}})
	invalidTag := reflect.New(invalidTagType).Elem()
	invalidTag.Field(0).SetString("safe")
	for _, tc := range []struct {
		name  string
		input any
	}{
		{"string", invalid},
		{"account_id", Account{ID: invalid}},
		{"nested_value", map[string]any{"id": []string{invalid}}},
		{"money", invalidMoney},
		{"money_pointer", &invalidMoney},
		{"key", map[string]int{invalid: 1}},
		{"colliding_keys", map[string]int{invalid: 1, "fixture-�": 2}},
		{"typed_key", map[cycle2NativeKey]int{cycle2NativeKey(invalid): 1}},
		{"formatted_key", map[cycle2FormattedKey]int{cycle2FormattedKey(invalid): 1}},
		{"lossy_key_conversion", map[cycle2LossyKey]int{cycle2LossyKey(invalid): 1}},
		{"lossy_composite_key_conversion", map[cycle2LossyStructKey]int{{ID: invalid}: 1}},
		{"struct_key", map[struct{ ID string }]int{{ID: invalid}: 1}},
		{"struct_tag", invalidTag.Interface()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			normalized, err := JSONable(tc.input)
			var pe *ParseError
			if normalized != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" || errors.Unwrap(err) != nil {
				t.Fatalf("native invalid text was silently converted/masked: %v %v", normalized, err)
			}
			cycle2AssertExportError(t, tc.input)
		})
	}
	if invalidMoney.Currency != invalid || invalidMoney.Amount != amount {
		t.Fatal("failed export mutated native financial data")
	}
	for _, tc := range []struct {
		name  string
		input any
		want  string
	}{
		{"literal_replacement", map[string]string{"fixture-�": "😀 and literal �"}, "😀 and literal �"},
		{"typed_key", map[cycle2NativeKey]string{"fixture-�": "safe"}, "fixture-�"},
		{"formatted_key", map[cycle2FormattedKey]string{"�": "safe"}, "fixture-format-�"},
		{"struct_id", Account{ID: "fixture-�"}, "fixture-�"},
		{"money_currency", Money{Amount: amount, Currency: "�"}, "9007199254740993.0100"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ExportJSON(tc.input)
			if err != nil || strictjson.Validate(out) != nil || !strings.Contains(string(out), tc.want) {
				t.Fatalf("valid native Unicode or exact amount lost: %s %v", out, err)
			}
		})
	}
	// Validate the custom document, not the raw credential implementation fields.
	for _, input := range []any{&pointerOnlySecret{Password: invalid}, &cycle2StreamSecret{Password: invalid}} {
		out, err := ExportJSON(input)
		if err != nil || strictjson.Validate(out) != nil || !strings.Contains(string(out), "redacted") || strings.Contains(string(out), "fixture-") {
			t.Fatal("native text checking bypassed/rejected credential redaction boundary")
		}
	}
}

func TestModelCycle2NativeObjectIdentityAndUnsupported(t *testing.T) {
	duplicateType := reflect.StructOf([]reflect.StructField{
		{Name: "Left", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
		{Name: "Right", Type: reflect.TypeFor[string](), Tag: `json:"id"`},
	})
	duplicate := reflect.New(duplicateType).Elem()
	duplicate.Field(0).SetString("left")
	duplicate.Field(1).SetString("right")
	for _, input := range []any{
		map[any]string{int(1): "left", "1": "right"},
		map[string]int{"4111 1111 1111 1111": 1, "4111-1111-1111-1111": 2},
		duplicate.Interface(),
		make(chan int), func() {}, complex(1, 2),
	} {
		cycle2AssertExportError(t, input)
	}
	cycle := map[string]any{}
	cycle["self"] = cycle
	cycle2AssertExportError(t, cycle)
	list := make([]any, 1)
	list[0] = list
	cycle2AssertExportError(t, list)
	pointer := new(any)
	*pointer = pointer
	cycle2AssertExportError(t, pointer)
	// Shared but acyclic native pointers must not be mistaken for a cycle.
	valid := "😀 �"
	out, err := ExportJSON([]any{&valid, &valid})
	if err != nil || strictjson.Validate(out) != nil || strings.Count(string(out), valid) != 2 {
		t.Fatal("acyclic pointer identity rejected")
	}
}

func FuzzModelCycle2NativeUTF8Export(f *testing.F) {
	for _, text := range []string{"fixture", "�", "😀", "4111-1111-1111-1111", string([]byte{0xff}), "fixture-" + string([]byte{0xff})} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		input := map[string]string{"fixture-" + text: text}
		out, err := ExportJSON(input)
		if !utf8.ValidString(text) {
			cycle2AssertExportError(t, input)
			return
		}
		if err != nil || strictjson.Validate(out) != nil {
			t.Fatalf("valid native text rejected or incomplete export: %v", err)
		}
		var got map[string]string
		if err := json.Unmarshal(out, &got); err != nil || len(got) != 1 {
			t.Fatal("native key identity lost")
		}
		if !strings.ContainsAny(text, "0123456789") && got["fixture-"+text] != text {
			t.Fatal("valid non-PAN Unicode rewritten")
		}
	})
}

// These post-GREEN fuzz properties compare original bytes to the approved
// strict-document gate. Valid documents may still fail for union types or
// collisions introduced by the intentional display-key PAN masking.
func FuzzModelCycle2DetailOriginalDocument(f *testing.F) {
	for _, data := range [][]byte{
		[]byte(`{}`), []byte(`null`), []byte(`{"value":"😀 �"}`),
		[]byte(`{"value":{"amount":9007199254740993.0100,"currency":"RUB"}}`),
		[]byte(`{"value":{"amount":true,"\u0061mount":"1.00"}}`),
		[]byte(`{"name":"a","\u006eame":"b"}`),
		[]byte(`{"value":{"amount":"1.00","currency":"\ud800"}}`),
		[]byte(`{"value":"safe"}false`), {'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'},
	} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		before := bytes.Clone(data)
		original := OperationDetailField{Name: "fixture-original", Type: "text", Value: "unchanged"}
		got := original
		err := got.UnmarshalJSON(data)
		if strictjson.Validate(data) != nil && err == nil {
			t.Fatal("invalid original document accepted")
		}
		if err != nil {
			if got != original || err.Error() != "sber: invalid domain data" || errors.Unwrap(err) != nil {
				t.Fatal("invalid union document mutated receiver or retained cause")
			}
		} else {
			var standard OperationDetailField
			if err := json.Unmarshal(data, &standard); err != nil || !reflect.DeepEqual(got, standard) {
				t.Fatal("direct/standard union decode diverged")
			}
		}
		if !bytes.Equal(data, before) {
			t.Fatal("original union input bytes mutated")
		}
	})
}

func cycle2MaskedKeyCollision(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		seen := map[string]bool{}
		for key, item := range v {
			masked := RedactPAN(key)
			if seen[masked] || cycle2MaskedKeyCollision(item) {
				return true
			}
			seen[masked] = true
		}
	case []any:
		for _, item := range v {
			if cycle2MaskedKeyCollision(item) {
				return true
			}
		}
	}
	return false
}

func FuzzModelCycle2StreamingOriginalDocument(f *testing.F) {
	for _, data := range [][]byte{
		[]byte(`{"display":"4111-1111-1111-1111","n":9007199254740993.0100}`),
		[]byte(`{"id":"\ud83d\ude00","literal":"�"}`),
		[]byte(`{"amount":true,"\u0061mount":"1.00"}`),
		[]byte(`{"id":"\ud800"}`), []byte(`true false`), {'"', 0xff, '"'},
		[]byte(`{"4111 1111 1111 1111":1,"4111-1111-1111-1111":2}`),
	} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		before := bytes.Clone(data)
		valid := strictjson.Validate(data) == nil
		if valid {
			dec := json.NewDecoder(bytes.NewReader(data))
			dec.UseNumber()
			var decoded any
			if err := dec.Decode(&decoded); err != nil {
				t.Fatal("invalid synthetic oracle input")
			}
			valid = !cycle2MaskedKeyCollision(decoded)
		}
		input := cycle2StreamRaw{Data: data}
		out, err := ExportJSON(input)
		if valid {
			if err != nil || strictjson.Validate(out) != nil {
				t.Fatalf("valid streaming document rejected: %v", err)
			}
		} else {
			cycle2AssertExportError(t, input)
		}
		if !bytes.Equal(data, before) {
			t.Fatal("original custom document bytes mutated")
		}
	})
}

func TestModelCycle2DetailOriginalDocumentIntegrity(t *testing.T) {
	amount, err := ParseDecimal("7.2500")
	if err != nil {
		t.Fatal(err)
	}
	original := OperationDetailField{Name: "fixture-original", Type: "money", Value: &Money{Amount: amount, Currency: "USD"}}
	invalidUTF8 := append([]byte(`{"name":"fixture-`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","value":"safe"}`)...)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"duplicate_amount", []byte(`{"name":"new","value":{"amount":true,"amount":"1.00","currency":"RUB"}}`)},
		{"escaped_amount", []byte(`{"value":{"amount":null,"\u0061mount":"1.00"}}`)},
		{"duplicate_currency", []byte(`{"value":{"amount":"1.00","currency":"USD","\u0063urrency":"RUB"}}`)},
		{"nested_code", []byte(`{"value":{"amount":"1.00","currency":{"code":"USD","\u0063ode":"RUB"}}}`)},
		{"duplicate_name", []byte(`{"name":"first","\u006eame":"second","value":"safe"}`)},
		{"unknown_nested_duplicate", []byte(`{"value":"safe","extra":[{"id":1,"\u0069d":2}]}`)},
		{"invalid_utf8", invalidUTF8},
		{"unpaired_high_currency", []byte(`{"value":{"amount":"1.00","currency":"\ud800"}}`)},
		{"unpaired_low_name", []byte(`{"name":"\udfff","value":null}`)},
		{"reversed_pair_value", []byte(`{"value":"\udfff\ud800"}`)},
		{"trailing_object", []byte(`{"value":"safe"}{"private":"fixture-trailing"}`)},
		{"trailing_scalar", []byte(`{"value":"safe"}false`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, direct := range []bool{true, false} {
				name := "json_unmarshal"
				if direct {
					name = "direct"
				}
				t.Run(name, func(t *testing.T) {
					data := bytes.Clone(tc.data)
					got := original
					var err error
					if direct {
						err = got.UnmarshalJSON(data)
					} else {
						err = json.Unmarshal(data, &got)
					}
					if err == nil {
						t.Error("invalid original union document accepted")
					}
					if direct || json.Valid(data) {
						var pe *ParseError
						if !errors.As(err, &pe) || pe.Field() != "detail_field" || err.Error() != "sber: invalid domain data" || errors.Unwrap(err) != nil {
							t.Errorf("union integrity error is not static: %v", err)
						}
					}
					if !reflect.DeepEqual(got, original) || got.Value != original.Value {
						t.Error("failed union decode mutated receiver or its original Money identity")
					}
					if !bytes.Equal(data, tc.data) {
						t.Error("union decode mutated original input bytes")
					}
				})
			}
		})
	}
}

func TestModelCycle2DetailAcceptedUnionForms(t *testing.T) {
	for _, tc := range []struct {
		name, document, fieldName, fieldType, text, amount, currency string
		money, typedNil                                              bool
	}{
		{name: "default", document: `{}`},
		{name: "source_null", document: `null`},
		{name: "explicit_null", document: `{"name":"fixture","type":"text","value":null}`, fieldName: "fixture", fieldType: "text"},
		{name: "empty_display", document: `{"value":""}`},
		{name: "unicode_display", document: `{"name":"\ud83d\ude00","value":"literal � and \ud83d\ude00"}`, fieldName: "😀", text: "literal � and 😀"},
		{name: "absent_amount", document: `{"value":{"currency":"RUB"}}`, typedNil: true},
		{name: "exact_export_money", document: `{"value":{"amount":9007199254740993.0100,"currency":"�"}}`, money: true, amount: "9007199254740993.0100", currency: "�"},
		{name: "source_money", document: `{"value":{"amount":"-0.00","currencyCode":null,"currency":{"code":"\ud83d\ude00"}}}`, money: true, amount: "-0.00", currency: "😀"},
		{name: "luhn_financial", document: `{"value":{"amount":"4111111111111111.00","currency":"RUB"}}`, money: true, amount: "4111111111111111.00", currency: "RUB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, direct := range []bool{true, false} {
				data := []byte(tc.document)
				before := bytes.Clone(data)
				got := OperationDetailField{Name: "replace-me", Value: "old"}
				var err error
				if direct {
					err = got.UnmarshalJSON(data)
				} else {
					err = json.Unmarshal(data, &got)
				}
				if err != nil || got.Name != tc.fieldName || got.Type != tc.fieldType || !bytes.Equal(data, before) {
					t.Fatalf("valid original union document changed: %+v %v", got, err)
				}
				if tc.money || tc.typedNil {
					money, ok := got.Value.(*Money)
					if !ok || (tc.typedNil && money != nil) || (tc.money && (money == nil || money.Amount.String() != tc.amount || money.Currency != tc.currency)) {
						t.Fatalf("native Money union lost: %+v", got)
					}
				} else if tc.name == "empty_display" || tc.text != "" {
					if got.Value != tc.text {
						t.Fatal("display string union lost")
					}
				} else if got.Value != nil {
					t.Fatal("default/null union lost")
				}
			}
		})
	}
}

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
	dir := testPrivateDir(t)
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
	pe.SDKError()
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
	dir := testPrivateDir(t)
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
