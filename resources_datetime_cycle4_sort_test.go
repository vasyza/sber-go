package sber_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	sber "github.com/vasyza/sber-go"
)

func TestCycle4CanonicalSortApplication(t *testing.T) {
	raw, e := os.ReadFile("testdata/datetime-cycle4/sorts.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		ID      string   `json:"id"`
		Texts   []string `json:"texts"`
		Indexes []int    `json:"sorted_indexes"`
		Pages   int      `json:"pages"`
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 12662 {
		t.Fatalf("full relational corpus changed: %d", len(rows))
	}
	for _, r := range rows {
		t.Run(r.ID, func(t *testing.T) {
			want := []string{}
			for _, i := range r.Indexes {
				want = append(want, fmt.Sprintf("synthetic-%d", i))
			}
			for _, application := range []string{"list", "collect"} {
				transport := &cycle4Offline{dates: append([]string(nil), r.Texts...)}
				api := sber.NewOperationsAPI(transport)
				q := sber.OperationsQuery{Resource: "card:fixture", Limit: 100, MaxPages: 100, From: "0001-01-01", To: "9999-12-31"}
				var ops []sber.Operation
				var err error
				if application == "list" {
					ops, err = api.List(context.Background(), q)
				} else {
					res, e := api.Collect(context.Background(), q)
					ops = res.Operations
					err = e
					if res.Metadata.BankCapProven || res.Metadata.WindowCompleteness != "unknown" {
						t.Fatal("completeness upgraded by sorting")
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				got := []string{}
				for _, op := range ops {
					got = append(got, op.ID)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("%s must reproduce actual canonical sorted algorithm (not an arbitrary comparator sort): texts=%q got=%v want=%v", application, r.Texts, got, want)
				}
				if len(transport.calls) != r.Pages || transport.forbidden != 0 {
					t.Fatal("application pagination or forbidden boundary changed")
				}
				if !reflect.DeepEqual(transport.dates, r.Texts) && len(r.Texts) > 0 {
					t.Fatal("input changed")
				}
			}
		})
	}
}
