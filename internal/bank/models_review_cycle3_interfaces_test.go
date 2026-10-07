package bank

import (
	"encoding/json/jsontext"
	"errors"
	"testing"
)

// The full pinned JSON serializer interface set is exercised using synthetic
// financial payloads that a FOREIGN redactor intentionally does not export.
type cycle3TextOnlySecret struct {
	Financial any
	Password  string
}

func (cycle3TextOnlySecret) MarshalText() ([]byte, error) { return []byte("<text-redacted>"), nil }

type cycle3TextAppendSecret struct {
	Financial any
	Password  string
}

func (*cycle3TextAppendSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("<append-redacted>")...), nil
}

type cycle3TextBothSecret struct {
	Financial any
	Password  string
}

func (*cycle3TextBothSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("<append-over-text-redacted>")...), nil
}
func (cycle3TextBothSecret) MarshalText() ([]byte, error) {
	return []byte("synthetic-lower-priority-private-text"), nil
}

type cycle3JSONLegacyTextSecret struct {
	Financial any
	Password  string
}

func (cycle3JSONLegacyTextSecret) MarshalJSON() ([]byte, error) {
	return []byte(`"<legacy-json-over-text-redacted>"`), nil
}
func (*cycle3JSONLegacyTextSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("synthetic-lower-priority-private-append")...), nil
}
func (cycle3JSONLegacyTextSecret) MarshalText() ([]byte, error) {
	return []byte("synthetic-lower-priority-private-text"), nil
}

type cycle3JSONStreamAllSecret struct {
	Financial any
	Password  string
}

func (*cycle3JSONStreamAllSecret) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteToken(jsontext.String("<stream-over-all-redacted>"))
}
func (cycle3JSONStreamAllSecret) MarshalJSON() ([]byte, error) {
	return []byte(`"synthetic-lower-priority-private-json"`), nil
}
func (*cycle3JSONStreamAllSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("synthetic-lower-priority-private-append")...), nil
}
func (cycle3JSONStreamAllSecret) MarshalText() ([]byte, error) {
	return []byte("synthetic-lower-priority-private-text"), nil
}

func TestModelCycle3FinancialContextHonorsAllForeignSerializerInterfaces(t *testing.T) {
	m := cycle3FinancialAmount(t)
	p := NewBankPortfolio(Products{Accounts: []Account{{ID: cycle3LiteralID, Balance: m}}, Cards: []Card{{ID: cycle3LiteralID, Balance: m}}}, nil)
	for _, payload := range []any{Account{ID: "synthetic_native_\xff"}, m, p, *p, p.Accounts()[0], *p.Accounts()[0], p.Cards()[0], *p.Cards()[0], []any{m, p.Cards()[0]}} {
		for _, tc := range []struct {
			name            string
			input, expected any
		}{
			{"text", cycle3TextOnlySecret{payload, "synthetic-private-\xff"}, "<text-redacted>"},
			{"append", &cycle3TextAppendSecret{payload, "synthetic-private-\xff"}, "<append-redacted>"},
			{"append_over_text", &cycle3TextBothSecret{payload, "synthetic-private-\xff"}, "<append-over-text-redacted>"},
			{"legacy_over_text", &cycle3JSONLegacyTextSecret{payload, "synthetic-private-\xff"}, "<legacy-json-over-text-redacted>"},
			{"stream_over_all", &cycle3JSONStreamAllSecret{payload, "synthetic-private-\xff"}, "<stream-over-all-redacted>"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cycle3AssertGenericJSON(t, tc.input, tc.expected)
				cycle3AssertGenericJSON(t, []any{tc.input}, []any{tc.expected})
				cycle3AssertGenericJSON(t, map[string]any{"credentials": tc.input}, map[string]any{"credentials": tc.expected})
				cycle3AssertGenericJSON(t, cycle3TypedStreamFallback{Payload: tc.input}, map[string]any{"payload": tc.expected, "display": ""})
				cycle3AssertGenericJSON(t, OperationDetail{UOHID: cycle3LiteralID, Fields: []OperationDetailField{{Value: tc.input}, {Value: m}}}, OperationDetail{UOHID: cycle3LiteralID, Fields: []OperationDetailField{{Value: tc.expected}, {Value: m}}})
			})
		}
	}
	for _, tc := range []struct {
		name            string
		input, expected any
	}{
		{"addressable_struct", &struct {
			Credential cycle3TextAppendSecret `json:"credential"`
		}{cycle3TextAppendSecret{p, "synthetic-private-\xff"}}, map[string]any{"credential": "<append-redacted>"}},
		{"addressable_slice", []cycle3TextAppendSecret{{p, "synthetic-private-\xff"}}, []any{"<append-redacted>"}},
		{"addressable_array", &[1]cycle3TextAppendSecret{{p, "synthetic-private-\xff"}}, []any{"<append-redacted>"}},
		{"nil_text_pointer", (*cycle3TextAppendSecret)(nil), nil},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.expected) })
	}
}

type cycle3TextFailureFloat float64

func (cycle3TextFailureFloat) MarshalText() ([]byte, error) {
	return nil, errors.New("synthetic-private-text-error")
}

type cycle3AppendFailure struct{ Data []byte }

func (v cycle3AppendFailure) AppendText(b []byte) ([]byte, error) { return append(b, v.Data...), nil }

func TestModelCycle3TextErrorsDoNotGainDeferredStreamingBypass(t *testing.T) {
	v := cycle3TextFailureFloat(1.25)
	for _, input := range []any{v, &v, cycle3AppendFailure{Data: []byte{0xff}}, cycle3AppendFailure{Data: []byte{0xfe}}} {
		out, err := ExportJSON(input)
		var pe *ParseError
		if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
			t.Fatal("text serializer error repaired/leaked")
		}
		normalized, err := JSONable(input)
		if normalized != nil || !errors.As(err, &pe) {
			t.Fatal("text error incorrectly acquired streaming-only deferred compatibility")
		}
	}
}
