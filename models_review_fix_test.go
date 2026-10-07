package sber

import (
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"strings"
	"testing"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// Synthetic credentials: pointer-only redaction must run on the addressable
// original field/element, not the plain reflected struct or a copied value.
func TestModelReviewAddressablePointerJSONMarshaler(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"struct_field", &struct {
			Credentials pointerOnlySecret `json:"credentials"`
		}{Credentials: pointerOnlySecret{Password: "fixture-private-password"}}},
		{"slice_element", []pointerOnlySecret{{Password: "fixture-private-password"}}},
		{"array_element", &[1]pointerOnlySecret{{Password: "fixture-private-password"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standard, err := json.Marshal(tc.value)
			if err != nil || strings.Contains(string(standard), "fixture-private-password") {
				t.Fatalf("invalid synthetic redaction control: %s %v", standard, err)
			}
			data, err := ExportJSON(tc.value)
			if err != nil || strings.Contains(string(data), "fixture-private-password") || !strings.Contains(string(data), "<redacted>") {
				t.Fatalf("addressable pointer marshaler bypass/leak: %s %v", data, err)
			}
		})
	}
	// Typed financial values must still bypass generic string PAN masking.
	amount, err := ParseDecimal("4111111111111111.00")
	if err != nil {
		t.Fatal(err)
	}
	financial := &struct {
		Money   Money   `json:"money"`
		Decimal Decimal `json:"decimal"`
	}{Money: Money{Amount: amount, Currency: "RUB"}, Decimal: amount}
	data, err := ExportJSON(financial)
	if err != nil || strings.Count(string(data), "4111111111111111.00") != 2 || strings.Contains(string(data), "••••") {
		t.Fatalf("typed exact money was masked: %s %v", data, err)
	}
}

func TestModelReviewJSONNumberTokenGrammar(t *testing.T) {
	for _, text := range []string{`"fixture-private-number"`, `null`, `true`, `false`, `{}`, `[]`, ``, `+1`, `01`, `-01`, `.5`, `1.`, `1_000`, `１２`, `1,25`, ` 1`, "1\n", `1 2`, `NaN`, `Infinity`} {
		t.Run(text, func(t *testing.T) {
			_, err := JSONable(json.Number(text))
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
				t.Fatalf("nonnumeric JSON token accepted or unsafe error for %q: %v", text, err)
			}
			data, err := ExportJSON(json.Number(text))
			if data != nil || !errors.As(err, &pe) || strings.Contains(err.Error(), "fixture-private-number") {
				t.Fatalf("unsafe numeric export: %s %v", data, err)
			}
		})
	}
	for _, text := range []string{"0", "-0.00", "9007199254740993.0100", "1.2300e+1000", "-1E-1000", "4111111111111111"} {
		t.Run("valid_"+text, func(t *testing.T) {
			value, err := JSONable(json.Number(text))
			if err != nil || value != json.Number(text) {
				t.Fatalf("valid numeric lexeme lost: %v %v", value, err)
			}
			data, err := ExportJSON(json.Number(text))
			if err != nil || string(data) != text {
				t.Fatalf("numeric lexeme changed: %s %v; want %s", data, err, text)
			}
		})
	}
}

// This finite native scalar reaches the real Go 1.27 encoder after JSONable.
// A streaming marshaler's private error must not escape the export boundary.
type modelReviewEncoderError float64

func (modelReviewEncoderError) MarshalJSONTo(*jsontext.Encoder) error {
	return errors.New("fixture-private-encoder-error")
}

func TestModelReviewExportEncoderErrorSanitized(t *testing.T) {
	value := modelReviewEncoderError(1.25)
	if _, err := JSONable(value); err != nil {
		t.Fatalf("synthetic scalar did not reach encoder: %v", err)
	}
	if _, err := json.Marshal(value); err == nil || !strings.Contains(err.Error(), "fixture-private-encoder-error") {
		t.Fatalf("synthetic encoder failure control missing: %v", err)
	}
	data, err := ExportJSON(value)
	var pe *ParseError
	if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
		t.Fatalf("encoder diagnostic leak: %s %v", data, err)
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("private encoder cause retained")
	}
}

type modelReviewCustomJSON struct{ data []byte }

func (m modelReviewCustomJSON) MarshalJSON() ([]byte, error) { return m.data, nil }

func TestModelReviewCustomJSONStrictFullDocument(t *testing.T) {
	invalidUTF8 := append([]byte(`{"id":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"trailing_object", []byte(`{"safe":true}{"discarded":"fixture-private-trailing"}`)},
		{"trailing_scalar", []byte(`{"safe":true}false`)},
		{"decoded_duplicate", []byte(`{"safe":false,"\u0073afe":true}`)},
		{"nested_duplicate", []byte(`{"nested":{"x":1,"x":2}}`)},
		{"invalid_utf8", invalidUTF8},
		{"unpaired_high", []byte(`{"id":"\ud800"}`)},
		{"unpaired_low", []byte(`{"id":"\udfff"}`)},
		{"reversed_pair", []byte(`{"id":"\udfff\ud800"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := modelReviewCustomJSON{data: tc.data}
			_, err := JSONable(value)
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
				t.Fatalf("invalid custom JSON document accepted: %s, error=%v", tc.name, err)
			}
			data, err := ExportJSON(value)
			if data != nil || !errors.As(err, &pe) || errors.Unwrap(err) != nil {
				t.Fatalf("invalid custom export/cause retained: %s %v", data, err)
			}
		})
	}
	for _, text := range []string{` {"id":"\ud83d\ude00","literal":"�","n":9007199254740993.0100} `, `[{"x":1},{"x":2}]`, `true`, `null`, `1.2300e+1000`, `"safe"`} {
		t.Run("valid_"+text, func(t *testing.T) {
			data, err := ExportJSON(modelReviewCustomJSON{data: []byte(text)})
			if err != nil || !json.Valid(data) {
				t.Fatalf("valid complete custom JSON rejected: %s %v", data, err)
			}
			if strings.Contains(text, "9007199254740993.0100") && !strings.Contains(string(data), "9007199254740993.0100") {
				t.Fatalf("custom numeric lexeme lost: %s", data)
			}
		})
	}
}

func TestModelReviewMoneyJSONRejectsDecodedDuplicateProperties(t *testing.T) {
	amount, err := ParseDecimal("7.2500")
	if err != nil {
		t.Fatal(err)
	}
	original := Money{Amount: amount, Currency: "USD"}
	for _, text := range []string{
		`{"amount":true,"amount":"1.00","currency":"RUB"}`,
		`{"amount":"NaN","amount":"1.00","currency":"RUB"}`,
		`{"amount":null,"\u0061mount":"1.00"}`,
		`{"amount":"1.00","amount":"1.00"}`,
		`{"amount":"1.00","currency":"USD","\u0063urrency":"RUB"}`,
		`{"amount":"1.00","currency":{"code":"USD","\u0063ode":"RUB"}}`,
		`{"amount":"1.00","extra":[{"id":1,"id":2}]}`,
	} {
		t.Run(text, func(t *testing.T) {
			for _, unmarshal := range []func([]byte, *Money) error{
				func(data []byte, m *Money) error { return m.UnmarshalJSON(data) },
				func(data []byte, m *Money) error { return json.Unmarshal(data, m) },
			} {
				got := original
				err := unmarshal([]byte(text), &got)
				var pe *ParseError
				if !errors.As(err, &pe) || pe.Field() != "money" || err.Error() != "sber: invalid domain data" {
					t.Fatalf("decoded duplicate Money property accepted: %s, err=%v", text, err)
				}
				if got != original {
					t.Fatalf("failed Money decode mutated receiver: %v", got)
				}
			}
		})
	}
	var got Money
	if err := json.Unmarshal([]byte(`{"amount":9007199254740993.0100,"currency":{"code":"\ud83d\ude00"},"extra":[{"id":1},{"id":2}]}`), &got); err != nil || got.Amount.String() != "9007199254740993.0100" || got.Currency != "😀" {
		t.Fatalf("valid exact Money or Unicode pair lost: %v %v", got, err)
	}
}

func TestModelReviewExplicitMalformedMoneyObject(t *testing.T) {
	for _, text := range []string{`true`, `false`, `"NaN"`, `"not-money"`, `[]`, `[1]`, `0`, `1.25`} {
		t.Run(text, func(t *testing.T) {
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatalf("invalid synthetic JSON fixture: %v", err)
			}
			money, err := ParseMoney(value)
			var pe *ParseError
			if money != nil || !errors.As(err, &pe) || pe.Field() != "money" {
				t.Errorf("explicit malformed money object accepted: %s -> %v %v", text, money, err)
			}
			core := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"fixture","operationAmount":`+text+`}]}}`)
			if operations, err := ParseOperations(core); operations != nil || !errors.As(err, &pe) {
				t.Errorf("explicit malformed core money accepted: %s -> %v %v", text, operations, err)
			}
			// The soft optional metadata path must still tolerate the same shapes.
			optional := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"fixture","operationAmount":{"amount":1.2500,"currency":"RUB"},"billingAmount":`+text+`,"nationalAmount":`+text+`,"commission":`+text+`,"tips":`+text+`}]}}`)
			operations, err := ParseOperations(optional)
			if err != nil || len(operations) != 1 {
				t.Fatalf("optional metadata discarded valid operation: %v %v", operations, err)
			}
			op := operations[0]
			if op.Amount == nil || op.Amount.Amount.String() != "1.2500" || op.BillingAmount != nil || op.NationalAmount != nil || op.Commission != nil || op.Tips != nil {
				t.Fatalf("strict/soft money paths conflated: %v", op)
			}
		})
	}
	// Nil objects and absent amount keys stay absent; no balances are fabricated.
	for _, value := range []any{nil, map[string]any(nil), map[string]any{}, map[string]any{"currency": "RUB"}} {
		if money, err := ParseMoney(value); err != nil || money != nil {
			t.Fatalf("absent money was fabricated or rejected: %v %v", money, err)
		}
	}
}

func TestModelReviewAccountDefaultConstruction(t *testing.T) {
	amount, err := ParseDecimal("9007199254740993.0100")
	if err != nil {
		t.Fatal(err)
	}
	balance := &Money{Amount: amount, Currency: "RUB"}
	for _, input := range []Account{
		{},
		{ID: "fixture-account", Name: "fixture", Last4: "1234", State: "OPEN", Hidden: true, Arrested: true, Balance: balance},
		{ID: "fixture-savings", Balance: nil, Kind: "account"},
	} {
		// Interface discovery lets the initial missing construction path fail at
		// runtime, without a placeholder constructor or a compiler-only RED.
		factory, ok := any(input).(interface{ WithDefaults() Account })
		if !ok {
			t.Fatal("source-compatible Account default construction path missing")
		}
		got := factory.WithDefaults()
		want := input
		if want.Kind == "" {
			want.Kind = "ctaccount" // models.py:Account.kind source default.
		}
		if got != want || got.Balance != input.Balance {
			t.Fatalf("Account defaults changed supplied fields/IDs/balance: got=%v want=%v", got, want)
		}
		if input.Kind == "" && got.Kind != "ctaccount" {
			t.Fatalf("wrong source Account default: %q", got.Kind)
		}
	}
}

func TestModelReviewDecimalJSONNumberTokenGrammar(t *testing.T) {
	for _, text := range []string{"+1", "01", "-01", ".5", "1.", "1_000", "１２.５０", "1,25", " 1", "1\n"} {
		t.Run(text, func(t *testing.T) {
			// Source-compatible decimal strings are still allowed; a json.Number
			// explicitly claims to be a JSON number lexeme, not that string syntax.
			if _, err := ParseDecimal(text); err != nil {
				t.Fatalf("synthetic decimal string control rejected: %v", err)
			}
			_, err := ParseDecimal(json.Number(text))
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Field() != "amount" {
				t.Fatalf("invalid numeric token normalized into financial amount: %q %v", text, err)
			}
		})
	}
	for _, text := range []string{"0", "-0.00", "9007199254740993.0100", "1.25e-2"} {
		fromNumber, err := ParseDecimal(json.Number(text))
		fromString, stringErr := ParseDecimal(text)
		if err != nil || stringErr != nil || fromNumber != fromString {
			t.Fatalf("valid numeric token lost: %v %v", fromNumber, err)
		}
	}
}

// Post-GREEN boundary exploration uses independent strict document/decoder
// checks, not the production numeric regex, as the acceptance oracle.
func FuzzModelReviewJSONNumberGate(f *testing.F) {
	for _, text := range []string{"0", "-0.00", "9007199254740993.0100", "1e+1000", `"fixture-private-number"`, "null", " 1", "01", "+1", "１２"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		valid := false
		if strictjson.Validate([]byte(text)) == nil {
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			var decoded any
			if err := decoder.Decode(&decoded); err == nil {
				number, ok := decoded.(json.Number)
				valid = ok && string(number) == text
			}
		}
		data, err := ExportJSON(json.Number(text))
		if valid {
			if err != nil || string(data) != text {
				t.Fatalf("valid token changed: %q %q %v", text, data, err)
			}
			return
		}
		var pe *ParseError
		if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
			t.Fatalf("invalid token accepted or unsafe error: %q %v", data, err)
		}
	})
}

func FuzzModelReviewCustomDocumentGate(f *testing.F) {
	for _, data := range [][]byte{
		[]byte(`{"safe":true}`), []byte(`{"safe":true}{}`),
		[]byte(`{"x":1,"\u0078":2}`), []byte(`{"id":"\ud800"}`),
		[]byte(`{"id":"\ud83d\ude00","amount":9007199254740993.0100}`),
		[]byte(`[{"id":1},{"id":2}]`), {'"', 0xff, '"'},
	} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		valid := strictjson.Validate(data) == nil
		out, err := ExportJSON(modelReviewCustomJSON{data: data})
		if valid {
			if err != nil || strictjson.Validate(out) != nil {
				t.Fatalf("valid full custom document rejected: %v", err)
			}
			return
		}
		var pe *ParseError
		if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
			t.Fatalf("invalid full custom document accepted or unsafe error: %q %v", out, err)
		}
	})
}
