package sber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
			raw = []byte(fmt.Sprintf(`{"ignored":{"x":1,"\u0078":2},"seed":%s}`, encoded))
		case 1:
			raw = []byte(fmt.Sprintf(`{"ignored":[{"x":1,"x":2}],"seed":%s}`, encoded))
		case 2:
			raw = []byte(fmt.Sprintf(`{"id":"\ud800","seed":%s}`, encoded))
		case 3:
			raw = append(append([]byte(`{"seed":`), encoded...), []byte(`} true`)...)
		case 4:
			raw = []byte(fmt.Sprintf(`{"bad":01,"seed":%s}`, encoded))
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
