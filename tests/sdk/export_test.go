package sber_test

import (
	"bytes"
	"context"
	"encoding"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"unicode/utf8"

	sber "github.com/vasyza/sber-go"
)

// A comparable private credential layout: its key-facing serializer is authoritative.
type independentOpaqueTextKey struct {
	secret string
	hidden *sber.BankAccount
}

func (independentOpaqueTextKey) MarshalText() ([]byte, error) { return []byte("<key-redacted>"), nil }

type independentEmbeddedTextKey struct{ independentOpaqueTextKey }

func TestIndependentCycle3MapKeyCredentialRedactorBoundary(t *testing.T) {
	a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	expected := map[string]any{"<key-redacted>": a.Snapshot()}
	k := independentOpaqueTextKey{secret: "independent-private-credential", hidden: a}
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"private_text_key", map[independentOpaqueTextKey]*sber.BankAccount{k: a}},
		{"promoted_text_key", map[independentEmbeddedTextKey]*sber.BankAccount{{k}: a}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standard, err := json.Marshal(tc.in)
			if err != nil || bytes.Contains(standard, []byte(k.secret)) {
				t.Fatal("official key redactor control failed")
			}
			independentAssert(t, tc.in, expected)
			out, err := sber.ExportJSON(tc.in)
			if err == nil && bytes.Contains(out, []byte(k.secret)) {
				t.Errorf("private credential map-key bytes escaped: %s", out)
			}
			// The official package fallback honors the very same key redactor.
			independentAssert(t, independentFallback{Payload: tc.in}, map[string]any{"payload": expected})
		})
	}
}

func TestIndependentCycle3OmittedEmptyFinancialContainerFields(t *testing.T) {
	input := struct {
		Cards    []*sber.BankCard             `json:"cards,omitempty"`
		Accounts map[string]*sber.BankAccount `json:"accounts,omitempty"`
		Keep     []any                        `json:"keep"`
		Omitted  *sber.BankPortfolio          `json:"omitted,omitempty"`
	}{Cards: []*sber.BankCard{}, Accounts: map[string]*sber.BankAccount{}, Keep: []any{}}
	expected := map[string]any{"keep": []any{}}
	standard, err := json.Marshal(input)
	if err != nil || string(standard) != `{"keep":[]}` {
		t.Fatal("independent omitempty control failed")
	}
	independentAssert(t, input, expected)
	independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
}

func TestIndependentCycle3NativeTagKeyCollisionsAfterMasking(t *testing.T) {
	input := struct {
		A int `json:"4111 1111 1111 1111"`
		B int `json:"4111-1111-1111-1111"`
	}{1, 2}
	independentReject(t, input)
	independentReject(t, independentFallback{Payload: input})
}

func TestIndependentCycle3BoundLiteralResourceAndRelations(t *testing.T) {
	m := independentMoney(t, independentAmount, "RUB")
	for _, raw := range []sber.Products{
		{Accounts: []sber.Account{{ID: independentID, Kind: "ctaccount", Balance: m}, {ID: independentID, Kind: "account"}}, Cards: []sber.Card{{ID: independentID, AccountID: func() *string { s := independentID; return &s }(), Balance: m}}},
		{Accounts: []sber.Account{{ID: independentID, Kind: "ctaccount"}, {ID: independentID, Kind: "ctaccount"}}, Cards: []sber.Card{{ID: independentID, AccountID: func() *string { s := independentID; return &s }(), Balance: m}}},
	} {
		r := &independentRequester{Secret: "independent-private\xff"}
		p := sber.NewBankPortfolio(raw, r)
		c := p.Cards()[0]
		resource, err := c.OperationResource()
		if err != nil || resource != "card:"+independentID {
			t.Fatal("literal card resource changed")
		}
		transfer, err := c.TransferResource()
		if err != nil || transfer != resource {
			t.Fatal("literal transfer resource changed")
		}
		for _, a := range p.Accounts() {
			op, err := a.OperationResource()
			if err != nil || (a.Kind() == "ctaccount" && op != "ct-account:"+independentID) || (a.Kind() == "account" && op != "account:"+independentID) {
				t.Fatal("literal account resource changed")
			}
		}
		if raw.Accounts[1].Kind == "account" {
			if c.Account() != p.Accounts()[0] || len(p.Accounts()[0].Cards()) != 1 || len(p.Accounts()[1].Cards()) != 0 {
				t.Fatal("noncurrent account collision erased current relation")
			}
		} else if c.Account() != nil {
			t.Fatal("ambiguous current account relation guessed")
		}
		independentAssert(t, p, raw)
		if r.MarshalCalls != 0 || len(r.Reads) != 0 {
			t.Fatal("passive financial seam reached requester")
		}
	}
	// A synthetic requester is the only I/O boundary; no concrete bank client/network.
	response, err := sber.DecodeJSON(bytes.NewBufferString(`{"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":4111111111111111,"name":"label","number":"00001","balance":{"amount":4111111111111111.50,"currencyCode":"RUB"}}]},"accounts":{"data":[]},"cardsInWallet":{"data":[]}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	r := &independentRequester{Response: response, Secret: "independent-private\xff"}
	resources := sber.NewResources(r)
	p, err := resources.Portfolio(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Reads) != 1 || len(p.Accounts()) != 1 || p.Accounts()[0].ID() != independentID || p.Accounts()[0].Balance().Amount.String() != independentAmount {
		t.Fatalf("synthetic bound response lost numeric literal identity/amount: %v", err)
	}
	for _, shape := range []any{p, *p, p.Accounts()[0], *p.Accounts()[0], independentStream{Payload: p}} {
		out, err := sber.ExportJSON(shape)
		if err != nil || !bytes.Contains(out, []byte(independentID)) || !bytes.Contains(out, []byte(independentAmount)) {
			t.Fatal("requester-bound reached export changed literal ID/amount")
		}
	}
	if len(r.Reads) != 1 || r.MarshalCalls != 0 {
		t.Fatal("export called binding again")
	}
}

func FuzzIndependentCycle3NativeProvenance(f *testing.F) {
	for _, row := range []struct {
		id, currency string
		mode         uint8
	}{{independentID, independentID, 0}, {"literal � 😀", "RUB", 1}, {"native-invalid\xff", "RUB", 2}, {independentID, "native-invalid\xfe", 3}, {"", "", 4}} {
		f.Add(row.id, row.currency, row.mode)
	}
	f.Fuzz(func(t *testing.T, id, currency string, mode uint8) {
		if len(id) > 512 || len(currency) > 512 {
			return
		}
		m := independentMoney(t, independentAmount, currency)
		raw := sber.Card{ID: id, AccountID: &id, Balance: m, Name: independentPAN}
		p := sber.NewBankPortfolio(sber.Products{Cards: []sber.Card{raw}}, nil)
		want := raw
		want.Name = independentMasked
		var input, expected any
		switch mode % 5 {
		case 0:
			input = p.Cards()[0]
			expected = want
		case 1:
			input = *p.Cards()[0]
			expected = want
		case 2:
			input = map[string]any{"c": p.Cards()[0], "raw": raw}
			expected = map[string]any{"c": want, "raw": want}
		case 3:
			input = independentStream{Payload: [1]*sber.BankCard{p.Cards()[0]}}
			expected = map[string]any{"~reached/0": []any{want}, "foreign": nil}
		case 4:
			input = independentFallback{Payload: struct {
				Card any `json:"card"`
			}{p.Cards()[0]}}
			expected = map[string]any{"payload": map[string]any{"card": want}}
		}
		if !utf8.ValidString(id) || !utf8.ValidString(currency) {
			independentReject(t, input)
		} else {
			independentAssert(t, input, expected)
		}
		if p.Cards()[0].ID() != id || p.Cards()[0].Balance().Currency != currency || p.Cards()[0].Balance().Amount.String() != independentAmount {
			t.Fatal("native mutation")
		}
	})
}

func FuzzIndependentCycle3OriginalNegativeDocuments(f *testing.F) {
	for i := uint8(0); i < 6; i++ {
		f.Add("original � 😀", i)
	}
	f.Fuzz(func(t *testing.T, seed string, mode uint8) {
		if len(seed) > 512 {
			return
		}
		encoded, _ := json.Marshal(seed)
		var raw []byte
		switch mode % 6 {
		case 0:
			raw = fmt.Appendf(nil, `{"ignored":{"x":1,"\u0078":2},"seed":%s}`, encoded)
		case 1:
			raw = fmt.Appendf(nil, `{"ignored":[{"x":1,"x":2}],"seed":%s}`, encoded)
		case 2:
			raw = fmt.Appendf(nil, `{"id":"\ud800","seed":%s}`, encoded)
		case 3:
			raw = append(append([]byte(`{"seed":`), encoded...), []byte(`} true`)...)
		case 4:
			raw = fmt.Appendf(nil, `{"bad":01,"seed":%s}`, encoded)
		case 5:
			raw = append([]byte(`{"bad":"`), 0xff)
			raw = append(raw, []byte(`"}`)...)
		}
		before := bytes.Clone(raw)
		independentReject(t, independentRaw(raw))
		independentReject(t, independentStream{Payload: sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, Raw: raw})
		if !bytes.Equal(before, raw) {
			t.Fatal("negative original document mutated")
		}
	})
}

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
		{"pointer", map[*cycle4PointerTextKey]int{{"synthetic-private\xff"}: 1}},
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

type cycle4ZeroStruct struct{}

func (cycle4ZeroStruct) IsZero() bool                 { return true }
func (cycle4ZeroStruct) MarshalJSON() ([]byte, error) { return []byte(`"<zero-struct-retained>"`), nil }

type cycle4EmptyCustomSlice []string

var cycle4EmptySliceCalls int

func (cycle4EmptyCustomSlice) MarshalJSON() ([]byte, error) {
	cycle4EmptySliceCalls++
	return []byte(`"<not-called-for-empty>"`), nil
}

func TestModelsReviewCycle4OmitEmptyEncoderSemantics(t *testing.T) {
	zero := 0
	input := struct {
		Cards      []*sber.BankCard             `json:"cards,omitempty"`
		Accounts   map[string]*sber.BankAccount `json:"accounts,omitempty"`
		KeptEmpty  []any                        `json:"kept_empty"`
		KeptNil    []any                        `json:"kept_nil"`
		ZeroStruct struct {
			N int `json:"n"`
		} `json:"zero_struct,omitempty"`
		ZeroArray      [1]int                 `json:"zero_array,omitempty"`
		EmptyArray     [0]int                 `json:"empty_array,omitempty"`
		ZeroCustom     cycle4ZeroStruct       `json:"zero_custom,omitempty"`
		EmptyCustom    cycle4EmptyCustomSlice `json:"empty_custom,omitempty"`
		TypedNil       any                    `json:"typed_nil,omitempty"`
		EmptyInterface any                    `json:"empty_interface,omitempty"`
		Pointer        *int                   `json:"pointer,omitempty"`
		False          bool                   `json:"false,omitempty"`
		Zero           int                    `json:"zero,omitempty"`
		ZeroFloat      float64                `json:"zero_float,omitempty"`
		EmptyText      string                 `json:"empty_text,omitempty"`
		FakeOption     string                 `json:"fake_option"`
		FakeSuffix     string                 `json:"fake_suffix"`
	}{Cards: []*sber.BankCard{}, Accounts: map[string]*sber.BankAccount{}, KeptEmpty: []any{}, EmptyCustom: cycle4EmptyCustomSlice{}, TypedNil: (*sber.BankAccount)(nil), Pointer: &zero}
	// Construct the intentionally unknown tag options as runtime test data.
	fields := reflect.VisibleFields(reflect.TypeOf(input))
	for i := range fields {
		switch fields[i].Name {
		case "FakeOption":
			fields[i].Tag = `json:"fake_option,notomitempty"`
		case "FakeSuffix":
			fields[i].Tag = `json:"fake_suffix,omitempty_suffix"`
		}
	}
	taggedType := reflect.StructOf(fields)
	tagged := reflect.ValueOf(input).Convert(taggedType)
	taggedPointer := reflect.New(taggedType)
	taggedPointer.Elem().Set(tagged)
	expected := map[string]any{
		"kept_empty": []any{}, "kept_nil": nil, "zero_struct": map[string]any{"n": 0}, "zero_array": []int{0},
		"zero_custom": "<zero-struct-retained>", "typed_nil": nil, "pointer": 0, "fake_option": "", "fake_suffix": "",
	}
	// The actual selected encoder, as well as legacy json.Marshal, agree.
	official, _ := json.Marshal(tagged.Interface())
	independentAssert(t, independentRaw(official), expected)
	independentAssert(t, independentFallback{Payload: tagged.Interface()}, map[string]any{"payload": expected})
	cycle4EmptySliceCalls = 0
	independentAssert(t, tagged.Interface(), expected)
	independentAssert(t, taggedPointer.Interface(), expected)
	if cycle4EmptySliceCalls != 0 {
		t.Fatal("omitempty called a custom serializer on an empty slice")
	}
}

func TestModelsReviewCycle4OmitEmptyNilEmptyNonemptyContainers(t *testing.T) {
	a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	for _, tc := range []struct {
		name     string
		cards    []*sber.BankCard
		accounts map[string]*sber.BankAccount
		want     any
	}{
		{"nil", nil, nil, map[string]any{"keep": []any{}}},
		{"explicit_empty", []*sber.BankCard{}, map[string]*sber.BankAccount{}, map[string]any{"keep": []any{}}},
		{"nonempty", []*sber.BankCard{nil}, map[string]*sber.BankAccount{"a": a}, map[string]any{"cards": []any{nil}, "accounts": map[string]any{"a": a.Snapshot()}, "keep": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := struct {
				Cards    []*sber.BankCard             `json:"cards,omitempty"`
				Accounts map[string]*sber.BankAccount `json:"accounts,omitempty"`
				Keep     []any                        `json:"keep"`
			}{tc.cards, tc.accounts, []any{}}
			independentAssert(t, input, tc.want)
			independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": tc.want})
		})
	}
}

func TestModelsReviewCycle4ForeignPropertyNames(t *testing.T) {
	money := independentMoney(t, independentAmount, independentID)
	input := struct {
		Account sber.Account `json:"4111 1111 1111 1111"`
		Money   *sber.Money  `json:"financial"`
		Forged  string       `json:"forged" sber:"literal"`
	}{Account: sber.Account{ID: independentID, Name: independentPAN, Balance: money}, Money: money, Forged: independentID}
	expected := map[string]any{
		"•••• 1111": sber.Account{ID: independentID, Name: independentMasked, Balance: money},
		"financial": money, "forged": "•••• 1111",
	}
	for _, tc := range []struct {
		name     string
		in, want any
	}{
		{"value", input, expected}, {"pointer", &input, expected},
		{"stream", independentStream{Payload: input}, map[string]any{"~reached/0": expected, "foreign": nil}},
		{"fallback", independentFallback{Payload: input}, map[string]any{"payload": expected}},
	} {
		t.Run(tc.name, func(t *testing.T) { independentAssert(t, tc.in, tc.want) })
	}
	if input.Account.ID != independentID || input.Money.Amount.String() != independentAmount || input.Forged != independentID {
		t.Fatal("property-name policy mutated values")
	}
}

func TestModelsReviewCycle4PropertyNameCollisions(t *testing.T) {
	for _, input := range []any{
		struct {
			A int `json:"4111 1111 1111 1111"`
			B int `json:"4111-1111-1111-1111"`
		}{1, 2},
		struct {
			A int `json:"4111111111111111"`
			B int `json:"•••• 1111"`
		}{1, 2},
		reflect.ValueOf(struct{ A, B int }{1, 2}).Convert(reflect.StructOf([]reflect.StructField{
			{Name: "A", Type: reflect.TypeFor[int](), Tag: `json:"same"`},
			{Name: "B", Type: reflect.TypeFor[int](), Tag: `json:"same"`},
		})).Interface(),
	} {
		independentReject(t, input)
	}
	// Raw name integrity precedes tag unquoting and display masking.
	typ := reflect.StructOf([]reflect.StructField{{Name: "Value", Type: reflect.TypeFor[string](), Tag: reflect.StructTag("json:\"original-invalid\xff\"")}})
	independentReject(t, reflect.New(typ).Elem().Interface())
	valid := struct {
		Value string `json:"literal � 😀"`
	}{"valid � 😀"}
	independentAssert(t, valid, map[string]any{"literal � 😀": "valid � 😀"})
	noPAN := struct {
		Value string `json:"4111111111111112"`
	}{"not-a-pan"}
	independentAssert(t, noPAN, map[string]any{"4111111111111112": "not-a-pan"})
}

type cycle4Slice []any

func TestModelsReviewCycle4FiniteBackingViews(t *testing.T) {
	a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
	prefix := make([]any, 2, 4)
	prefix[0] = a
	prefix[1] = prefix[:1:1]
	empty := make([]any, 2, 4)
	empty[0] = empty[:0:0]
	empty[1] = a
	shifted := make([]any, 3)
	shifted[0] = shifted[1:2]
	shifted[1] = a
	shifted[2] = shifted[1:2]
	named := make(cycle4Slice, 2)
	named[0] = a
	named[1] = named[:1]
	w := a.Snapshot()
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"prefix_cap_limited", prefix, []any{w, []any{w}}},
		{"empty_cap_limited", empty, []any{[]any{}, w}},
		{"shifted_shared_siblings", shifted, []any{[]any{w}, w, []any{w}}},
		{"named_prefix", named, []any{w, []any{w}}},
		{"prefix_pointer", &prefix, []any{w, []any{w}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			independentAssert(t, tc.input, tc.want)
			independentAssert(t, independentFallback{Payload: tc.input}, map[string]any{"payload": tc.want})
		})
	}
	if prefix[0] != a || empty[1] != a || a.ID() != independentID {
		t.Fatal("view traversal mutated native input")
	}
}

func TestModelsReviewCycle4ActualCyclesRemainRejected(t *testing.T) {
	self := make([]any, 1)
	self[0] = self
	named := make(cycle4Slice, 1)
	named[0] = named
	// Unequal active lengths can still eventually repeat a real view.
	prefix := make([]any, 2)
	prefix[0] = prefix[:1]
	prefix[1] = 42
	capacity := make([]any, 1, 4)
	capacity[0] = capacity[:1:1]
	pairA := make([]any, 1)
	pairB := make([]any, 1)
	pairA[0] = pairB
	pairB[0] = pairA
	m := map[string]any{}
	m["self"] = m
	o := sber.OperationDetail{UOHID: independentID}
	o.Fields = []sber.OperationDetailField{{Value: &o}}
	for _, tc := range []struct {
		name string
		in   any
	}{
		{"self", self}, {"named", named}, {"nested_prefix_cycle", prefix}, {"capacity_not_identity", capacity},
		{"mutual", pairA}, {"map", m}, {"native_union", &o},
	} {
		t.Run(tc.name, func(t *testing.T) { independentReject(t, tc.in) })
	}
	independentAssert(t, []any{[]any{}, []any{}, nil}, []any{[]any{}, []any{}, nil})
}
