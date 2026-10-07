package sber_test

import (
	"testing"
	"unicode/utf8"

	sber "github.com/vasyza/sber-go"
)

func FuzzModelsReviewCycle4AuthoritativeKey(f *testing.F) {
	for _, row := range []struct {
		text string
		mode uint8
	}{{"key", 0}, {independentID, 0}, {"literal � 😀", 0}, {"original-invalid\xff", 0}, {"key", 1}, {"key", 2}} {
		f.Add(row.text, row.mode)
	}
	f.Fuzz(func(t *testing.T, text string, mode uint8) {
		if len(text) > 512 {
			return
		}
		k := cycle4TextKey{Name: text, Hidden: sber.NewBankAccount(sber.Account{ID: "private-invalid\xff"}, nil, nil), mode: mode % 3}
		input := map[cycle4TextKey]*sber.Money{k: independentMoney(t, independentAmount, independentID)}
		if k.mode != 0 || !utf8.ValidString(text) {
			independentReject(t, input)
			return
		}
		expected := map[string]any{sber.RedactPAN(text): independentMoney(t, independentAmount, independentID)}
		independentAssert(t, input, expected)
		independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
	})
}

func FuzzModelsReviewCycle4ForeignPropertyPolicy(f *testing.F) {
	for _, s := range []string{"label", independentID, independentPAN, "literal � 😀", "original-invalid\xff"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, label string) {
		if len(label) > 512 {
			return
		}
		m := independentMoney(t, independentAmount, independentID)
		input := struct {
			Money *sber.Money `json:"4111111111111111"`
			Label string      `json:"label" sber:"literal"`
		}{m, label}
		if !utf8.ValidString(label) {
			independentReject(t, input)
			return
		}
		expected := map[string]any{"•••• 1111": m, "label": sber.RedactPAN(label)}
		independentAssert(t, input, expected)
		independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
		collision := struct {
			Money *sber.Money `json:"4111111111111111"`
			Label string      `json:"•••• 1111"`
		}{m, label}
		independentReject(t, collision)
	})
}

func FuzzModelsReviewCycle4FiniteSliceViews(f *testing.F) {
	for _, n := range []uint8{0, 1, 2, 7, 255} {
		f.Add(n)
	}
	f.Fuzz(func(t *testing.T, size uint8) {
		n := 1 + int(size%8)
		a := sber.NewBankAccount(sber.Account{ID: independentID, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
		input := make([]any, n+1)
		expected := make([]any, n+1)
		prefix := make([]any, n)
		for i := 0; i < n; i++ {
			input[i] = a
			expected[i] = a.Snapshot()
			prefix[i] = a.Snapshot()
		}
		input[n] = input[:n:n]
		expected[n] = prefix
		independentAssert(t, input, expected)
		independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
		empty := make([]any, n+1)
		wantEmpty := make([]any, n+1)
		empty[0] = empty[:0:0]
		wantEmpty[0] = []any{}
		for i := 1; i <= n; i++ {
			empty[i] = a
			wantEmpty[i] = a.Snapshot()
		}
		independentAssert(t, empty, wantEmpty)
		cycle := make([]any, n+1)
		cycle[0] = cycle[:1:1]
		independentReject(t, cycle)
	})
}

func FuzzModelsReviewCycle4OmitEmptyShape(f *testing.F) {
	for _, row := range []struct {
		id   string
		mode uint8
	}{{independentID, 0}, {independentID, 1}, {independentID, 2}, {"literal � 😀", 2}, {"original-invalid\xff", 2}} {
		f.Add(row.id, row.mode)
	}
	f.Fuzz(func(t *testing.T, id string, mode uint8) {
		if len(id) > 512 {
			return
		}
		input := struct {
			Cards    []*sber.BankCard             `json:"cards,omitempty"`
			Accounts map[string]*sber.BankAccount `json:"accounts,omitempty"`
			Keep     []any                        `json:"keep"`
		}{Keep: []any{}}
		expected := map[string]any{"keep": []any{}}
		switch mode % 3 {
		case 1:
			input.Cards = []*sber.BankCard{}
			input.Accounts = map[string]*sber.BankAccount{}
		case 2:
			a := sber.NewBankAccount(sber.Account{ID: id, Balance: independentMoney(t, independentAmount, "RUB")}, nil, nil)
			input.Cards = []*sber.BankCard{nil}
			input.Accounts = map[string]*sber.BankAccount{"a": a}
			expected["cards"] = []any{nil}
			expected["accounts"] = map[string]any{"a": a.Snapshot()}
			if !utf8.ValidString(id) {
				independentReject(t, input)
				return
			}
		}
		independentAssert(t, input, expected)
		independentAssert(t, independentFallback{Payload: input}, map[string]any{"payload": expected})
	})
}
