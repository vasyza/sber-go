package sber_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	sber "github.com/vasyza/sber-go"
)

var _ func(string) (sber.SourceDateTime, error) = sber.ParseSourceDateTime
var _ func(string, string) (sber.TimeFilter, error) = sber.NewTimeFilter
var _ func(string, string) (sber.TimeFilter, error) = sber.NewSourceTimeFilter
var _ func(string) (time.Time, error) = sber.ParseSourceDate
var _ func(string) time.Time = sber.OperationSortKey
var _ func(string, string) int = sber.SourceOperationCompare
var _ func([]sber.Operation) []sber.Operation = sber.SortSourceOperations

func TestCycle4SourceComparisonAndSortCopy(t *testing.T) {
	raw, e := os.ReadFile("testdata/datetime-cycle4/sorts.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		ID      string   `json:"id"`
		Texts   []string `json:"texts"`
		Indexes []int    `json:"sorted_indexes"`
		Pairs   []struct {
			I, J       int
			Order      int
			Lt, Gt, Eq bool
			Same       bool `json:"same_tzinfo"`
		} `json:"pairs"`
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	relations := 0
	eqexceptions := 0
	for _, r := range rows {
		for _, p := range r.Pairs {
			relations++
			if !p.Eq && !p.Lt && !p.Gt {
				eqexceptions++
			}
			a, b := r.Texts[p.I], r.Texts[p.J]
			ka, kb := sber.OperationSortKey(a), sber.OperationSortKey(b)
			got := sber.SourceOperationCompare(a, b)
			if got != p.Order || sber.SourceOperationCompare(b, a) != -p.Order {
				t.Fatalf("%s actual source comparison %q/%q got=%d want=%d", r.ID, a, b, got, p.Order)
			}
			if !sber.OperationSortKey(a).Equal(ka) || !sber.OperationSortKey(b).Equal(kb) || sber.OperationSortKey(a).Format("2006-01-02T15:04:05.999999999Z07:00:00") != ka.Format("2006-01-02T15:04:05.999999999Z07:00:00") || sber.OperationSortKey(b).Format("2006-01-02T15:04:05.999999999Z07:00:00") != kb.Format("2006-01-02T15:04:05.999999999Z07:00:00") {
				t.Fatal("native key mutated to simulate source relation")
			}
		}
		ops := make([]sber.Operation, len(r.Texts))
		for i, text := range r.Texts {
			ops[i] = sber.Operation{ID: fmt.Sprint(i), Date: text}
		}
		before := append([]sber.Operation{}, ops...)
		out := sber.SortSourceOperations(ops)
		if !reflect.DeepEqual(ops, before) || len(out) != len(ops) {
			t.Fatal("copy API mutated input")
		}
		for i, idx := range r.Indexes {
			if out[i].ID != fmt.Sprint(idx) {
				t.Fatalf("%s direct canonical sort mismatch", r.ID)
			}
		}
	}
	if relations != 30847 || eqexceptions != 2964 {
		t.Fatalf("relational denominator lost: %d / %d", relations, eqexceptions)
	}
}
func TestCycle4ExplicitSourceConstructors(t *testing.T) {
	day, e := sber.ParseSourceDate("2024W09423")
	if e != nil || day.Format("2006-01-02") != "2024-02-29" {
		t.Fatal("independent source DATE meaning missing")
	}
	dt, e := sber.ParseSourceDateTime("2024W09423")
	if e != nil || dt.ISOFormat() != "2024-02-26T23:00:00+03:00" || !dt.DateOnly() {
		t.Fatal("independent DATETIME meaning or DATE acceptability lost")
	}
	f, e := sber.NewSourceTimeFilter("2010-03-28T02:45:00", "2010-03-28T03:15:00")
	if e != nil || f.From == nil || f.To == nil || !f.From.After(*f.To) {
		t.Fatal("source constructor must retain reversed truthful instants in an accepted wall window")
	}
	if _, e = sber.NewSourceTimeFilter("2010-03-28T03:15:00", "2010-03-28T02:45:00"); e == nil {
		t.Fatal("source constructor accepted reversed wall window")
	}
	if f.Contains("2010-03-28T03:00:00") {
		t.Fatal("source request constructor must not silently change native Contains instant policy")
	}
	a, b, c := "2010-03-28T02:45:00", "2010-03-28T03:15:00", "2010-03-27T23:30:00Z"
	if sber.SourceOperationCompare(a, b) != -1 || sber.SourceOperationCompare(b, c) != -1 || sber.SourceOperationCompare(c, a) != -1 {
		t.Fatal("actual source strict comparison cycle was total-ordered away")
	}
}
