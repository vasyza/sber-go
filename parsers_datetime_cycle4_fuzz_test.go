package sber_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	sber "github.com/vasyza/sber-go"
	"os"
	"reflect"
	"testing"
	"time"
)

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
	data, e := os.ReadFile("testdata/datetime-cycle4/sorts.json")
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
	data, e := os.ReadFile("testdata/datetime-cycle4/bank-dates.json")
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
