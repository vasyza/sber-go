package sber_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
	"unicode/utf8"

	sber "github.com/vasyza/sber-sdk"
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

func FuzzCycle4SourceDateValues(f *testing.F) {
	for _, s := range []string{"2024W09423", "20240229xx", "2024W094é", "2010-03-28T02:45:00", "2014-10-26T01:45:00", "2026-08-01T12:30:00.1234569+03:00:00.5000009", "0001-01-01T00:00:00+23:59", "9999-12-31T23:59:59Z", "invalid", ""} {
		f.Add(s, s)
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		oldA, oldB := a, b
		v, e := sber.ParseSourceDateTime(a)
		copyValue := v
		if (e == nil) != v.Valid() {
			t.Fatal("validity inconsistent")
		}
		if v.Valid() {
			if v.CivilTime().Nanosecond()%1000 != 0 || v.UTCOffset()%time.Microsecond != 0 {
				t.Fatal("source precision changed")
			}
			if !v.Native().Equal(v.CivilTime().Add(-v.UTCOffset())) {
				t.Fatal("truthful native instant lost")
			}
			if v.UTCOffset()%time.Second != 0 && v.Native().Location() != time.UTC {
				t.Fatal("fractional offset faked in native zone")
			}
			if v != copyValue {
				t.Fatal("source immutable value changed")
			}
		}
		day, de := sber.ParseSourceDate(a)
		if de == nil && (day.Year() < 1 || day.Year() > 9999 || day.Location() != time.UTC || day.Hour() != 0 || day.Nanosecond() != 0) {
			t.Fatal("invalid civil DATE result")
		}
		native, ne := sber.NewTimeFilter(a, b)
		source, se := sber.NewSourceTimeFilter(a, b)
		if ne != nil && (native.From != nil || native.To != nil) {
			t.Fatal("partial native bounds published")
		}
		if se != nil && (source.From != nil || source.To != nil) {
			t.Fatal("partial source bounds published")
		}
		if ne == nil {
			before := native
			_, _, _ = native.SourceRequestBounds()
			_, _, _ = native.SourceISOBounds()
			if !reflect.DeepEqual(before, native) {
				t.Fatal("native bounds mutated")
			}
		}
		if a != oldA || b != oldB {
			t.Fatal("input mutated")
		}
		if sber.SourceOperationCompare(a, b) != -sber.SourceOperationCompare(b, a) {
			t.Fatal("source relation lost antisymmetric order")
		}
	})
}
func FuzzCycle4CanonicalSortControls(f *testing.F) {
	data, e := os.ReadFile("../../testdata/datetime/sorts.json")
	if e != nil {
		f.Fatal(e)
	}
	var rows []struct {
		Texts   []string `json:"texts"`
		Indexes []int    `json:"sorted_indexes"`
	}
	if e = json.Unmarshal(data, &rows); e != nil {
		f.Fatal(e)
	}
	for _, n := range []uint32{0, 37, 159, 160, 1560, 6000, 11854, 12400, 12661} {
		f.Add(n, []byte{0, 1, 2, 1})
	}
	f.Fuzz(func(t *testing.T, selector uint32, spell []byte) {
		r := rows[int(selector%uint32(len(rows)))]
		ops := make([]sber.Operation, len(r.Texts))
		for i, s := range r.Texts {
			ops[i] = sber.Operation{ID: fmt.Sprint(i), Date: s}
		}
		before := append([]sber.Operation{}, ops...)
		out := sber.SortSourceOperations(ops)
		if !reflect.DeepEqual(before, ops) || len(out) != len(ops) {
			t.Fatal("input/permutation lost")
		}
		for i, n := range r.Indexes {
			if out[i].ID != fmt.Sprint(n) {
				t.Fatal("canonical frozen schedule changed")
			}
		}
		// Newly mutated lists test preservation/safety only, not oracle parity.
		if len(spell) > 2048 {
			spell = spell[:2048]
		}
		pool := []string{"2010-03-28T02:45:00", "2010-03-28T03:15:00", "2010-03-27T23:30:00Z", "invalid", "0001-01-01", "9999-12-31T23:59:59Z"}
		mutant := make([]sber.Operation, len(spell))
		for i, n := range spell {
			mutant[i] = sber.Operation{ID: fmt.Sprint(i), Date: pool[int(n)%len(pool)]}
		}
		old := append([]sber.Operation{}, mutant...)
		sorted := sber.SortSourceOperations(mutant)
		if !reflect.DeepEqual(old, mutant) || len(sorted) != len(mutant) {
			t.Fatal("mutated list lost input")
		}
		seen := map[string]bool{}
		for _, op := range sorted {
			if seen[op.ID] {
				t.Fatal("duplicated item")
			}
			seen[op.ID] = true
		}
	})
}
func FuzzCycle4CanonicalBankControls(f *testing.F) {
	data, e := os.ReadFile("../../testdata/datetime/bank-dates.json")
	if e != nil {
		f.Fatal(e)
	}
	var rows []struct {
		Raw        string `json:"raw"`
		Normalized string `json:"normalized"`
	}
	if e = json.Unmarshal(data, &rows); e != nil {
		f.Fatal(e)
	}
	for _, n := range []uint32{0, 1, 2, 3, 1900, 2998, 3043} {
		f.Add(n, " 1.2.2024T3:4:5")
	}
	f.Fuzz(func(t *testing.T, selector uint32, mutation string) {
		r := rows[int(selector%uint32(len(rows)))]
		for i, text := range []string{r.Raw, mutation} {
			payload := map[string]any{"body": map[string]any{"operations": []any{map[string]any{"uohId": "fixture", "date": text}}}}
			before, _ := json.Marshal(payload)
			ops, e := sber.ParseOperations(payload)
			if e != nil || len(ops) != 1 {
				t.Fatal("parser failure")
			}
			after, _ := json.Marshal(payload)
			if !bytes.Equal(before, after) {
				t.Fatal("input mutation")
			}
			if i == 0 && ops[0].Date != r.Normalized {
				t.Fatal("canonical bank grammar changed")
			}
		}
	})
}
