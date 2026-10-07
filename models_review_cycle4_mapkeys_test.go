package sber_test

import (
	"encoding"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	sber "github.com/vasyza/sber-go"
)

// Every marker is synthetic. Poisoned public payloads must stay behind the
// comparable key's authoritative redactor, just like private payloads.
type cycle4TextKey struct {
	Name   string
	Hidden *sber.BankAccount
	mode   uint8
}

func (k cycle4TextKey) MarshalText() ([]byte, error) {
	switch k.mode {
	case 1:
		return []byte("original-invalid\xff"), nil
	case 2:
		return nil, errors.New("cycle4-private-cause")
	}
	return []byte(k.Name), nil
}

type cycle4PromotedTextKey struct{ cycle4TextKey }
type cycle4AppendKey struct{ cycle4TextKey }

func (k cycle4AppendKey) AppendText(b []byte) ([]byte, error) {
	return append(b, "append-"+k.Name...), nil
}

type cycle4AllKeyMethods struct{ cycle4AppendKey }

func (cycle4AllKeyMethods) MarshalJSONTo(*jsontext.Encoder) error {
	panic("JSON value method is not key-facing")
}
func (cycle4AllKeyMethods) MarshalJSON() ([]byte, error) {
	panic("legacy JSON value method is not key-facing")
}

type cycle4StringTextKey string

func (k cycle4StringTextKey) MarshalText() ([]byte, error) { return []byte("string-key-redacted"), nil }

type cycle4PointerTextKey struct{ Secret string }

func (*cycle4PointerTextKey) MarshalText() ([]byte, error) {
	return []byte("pointer-key-redacted"), nil
}

type cycle4SourceKey struct {
	Name string
	N    int
}
type cycle4RepairingKey struct{ ID string }

func (cycle4RepairingKey) String() string { return "repaired" }

func cycle4KeyOracle(t *testing.T, input any) any {
	t.Helper()
	b, err := jsonv2.Marshal(input, json.DefaultOptionsV1(), jsontext.AllowInvalidUTF8(false), jsontext.AllowDuplicateNames(false))
	if err != nil {
		t.Fatalf("official key-facing oracle setup failed: %v", err)
	}
	return independentDecode(t, b)
}

func TestModelsReviewCycle4MapKeyAuthority(t *testing.T) {
	poison := sber.NewBankAccount(sber.Account{ID: "original-invalid\xff"}, nil, nil)
	k := cycle4TextKey{Name: "key-redacted", Hidden: poison}
	// Compare with the pinned official key context, not a value-method call.
	for _, tc := range []struct {
		name  string
		input any
	}{
		{"direct", map[cycle4TextKey]int{k: 1}},
		{"promoted", map[cycle4PromotedTextKey]int{{k}: 1}},
		{"append_over_text", map[cycle4AppendKey]int{{k}: 1}},
		{"all_methods_key_priority", map[cycle4AllKeyMethods]int{{cycle4AppendKey{k}}: 1}},
		{"named_string", map[cycle4StringTextKey]int{"synthetic-private": 1}},
		{"pointer", map[*cycle4PointerTextKey]int{&cycle4PointerTextKey{"synthetic-private\xff"}: 1}},
		{"nil_text_pointer", map[*cycle4PointerTextKey]int{nil: 1}},
		{"interface_key", map[any]int{k: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expected := cycle4KeyOracle(t, tc.input)
			independentAssert(t, tc.input, expected)
			independentAssert(t, independentFallback{Payload: tc.input}, map[string]any{"payload": expected})
		})
	}
	// Redaction can legitimately hide malformed credential implementation text.
	if poison.ID() != "original-invalid\xff" {
		t.Fatal("raw credential payload changed")
	}
}

func TestModelsReviewCycle4MapKeyOutputIntegrity(t *testing.T) {
	for _, input := range []any{
		map[cycle4TextKey]int{{Name: "ignored", mode: 1}: 1},
		map[cycle4TextKey]int{{Name: "ignored", mode: 2}: 1},
		map[cycle4TextKey]int{{Name: "same", mode: 0}: 1, {Name: "same", Hidden: &sber.BankAccount{}}: 2},
		map[cycle4TextKey]int{{Name: "4111 1111 1111 1111"}: 1, {Name: "4111-1111-1111-1111"}: 2},
	} {
		independentReject(t, input)
	}
	independentAssert(t, map[cycle4TextKey]int{{Name: independentID}: 1}, map[string]any{"•••• 1111": 1})
	independentAssert(t, map[cycle4TextKey]int{{Name: "literal � 😀"}: 1}, map[string]any{"literal � 😀": 1})
	// No fmt repair of native noncustom identity before its UTF-8 gate.
	independentReject(t, map[cycle4RepairingKey]int{{ID: "original-invalid\xff"}: 1})
}

func TestModelsReviewCycle4NilTextInterfaceSourceKey(t *testing.T) {
	independentAssert(t, map[encoding.TextMarshaler]int{nil: 1}, map[string]any{"": 1})
	independentAssert(t, map[encoding.TextAppender]int{nil: 1}, map[string]any{"": 1})
}

func TestModelsReviewCycle4SupportedSourceKeysRemain(t *testing.T) {
	for _, tc := range []struct {
		input    any
		expected any
	}{
		{map[int]string{-7: "value"}, map[string]any{"-7": "value"}},
		{map[uint64]string{9007199254740993: "value"}, map[string]any{"9007199254740993": "value"}},
		{map[bool]int{true: 1}, map[string]any{"True": 1}},
		{map[[2]int]int{{2, 3}: 1}, map[string]any{"[2 3]": 1}},
		{map[cycle4SourceKey]int{{Name: "public", N: 7}: 1}, map[string]any{"{public 7}": 1}},
		{map[any]int{nil: 1}, map[string]any{"": 1}},
		{map[string]*sber.BankAccount{"account": sber.NewBankAccount(sber.Account{ID: independentID}, nil, nil)}, map[string]any{"account": sber.Account{ID: independentID}}},
	} {
		independentAssert(t, tc.input, tc.expected)
	}
	independentReject(t, map[any]int{1: 1, "1": 2})
	original := map[cycle4TextKey]int{{Name: "unchanged"}: 1}
	copyBefore := reflect.ValueOf(original).MapKeys()[0].Interface()
	independentAssert(t, original, map[string]any{"unchanged": 1})
	if !reflect.DeepEqual(copyBefore, reflect.ValueOf(original).MapKeys()[0].Interface()) {
		t.Fatal("native key mutated")
	}
}
