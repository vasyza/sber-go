package sber_test

import (
	"bytes"
	"context"
	"encoding/json"
	sber "github.com/vasyza/sber-go"
	"os"
	"testing"
)

func TestCycle4CanonicalBankStrptime(t *testing.T) {
	raw, e := os.ReadFile("testdata/datetime-cycle4/bank-dates.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		ID         string `json:"id"`
		Raw        string `json:"raw"`
		Normalized string `json:"normalized"`
		Valid      bool   `json:"valid"`
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 3044 {
		t.Fatalf("full bank grammar denominator changed: %d", len(rows))
	}
	for _, r := range rows {
		t.Run(r.ID, func(t *testing.T) {
			payload := map[string]any{"body": map[string]any{"operations": []any{map[string]any{"uohId": "synthetic", "date": r.Raw}}}}
			before, e := json.Marshal(payload)
			if e != nil {
				t.Fatal(e)
			}
			ops, e := sber.ParseOperations(payload)
			if e != nil || len(ops) != 1 {
				t.Fatalf("bank parser %v", e)
			}
			after, _ := json.Marshal(payload)
			if !bytes.Equal(before, after) {
				t.Fatal("input map mutated")
			}
			if ops[0].Date != r.Normalized {
				t.Fatalf("actual original strptime %q: got %q want %q", r.Raw, ops[0].Date, r.Normalized)
			}
			if r.Valid && sber.OperationSortKey(ops[0].Date).IsZero() {
				t.Fatal("source-valid normalized bank date became invalid key")
			}
			decoded, e := sber.DecodeJSON(bytes.NewReader(before))
			if e != nil {
				t.Fatal(e)
			}
			viaJSON, e := sber.ParseOperations(decoded)
			if e != nil || len(viaJSON) != 1 || viaJSON[0].Date != r.Normalized {
				t.Fatal("strict JSON->operation binding lost grammar")
			}
			transport := &cycle4Offline{dates: []string{r.Raw}}
			page, e := sber.NewOperationsAPI(transport).Page(context.Background(), sber.OperationsPageOptions{Limit: 100, From: "0001-01-01", To: "9999-12-31"})
			if e != nil || len(page.Operations) != 1 || page.Operations[0].Date != r.Normalized {
				t.Fatal("Page bank date binding lost grammar")
			}
			if transport.forbidden != 0 {
				t.Fatal("forbidden call")
			}
		})
	}
}
