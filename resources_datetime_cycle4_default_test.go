package sber_test

import (
	"context"
	"encoding/json"
	sber "github.com/vasyza/sber-go"
	"os"
	"testing"
	"time"
)

func TestCycle4CanonicalDefaultWindowCivilClock(t *testing.T) {
	raw, e := os.ReadFile("testdata/datetime-cycle4/defaults.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		ID, Now string
		Valid   bool
		Body    map[string]any
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 6 {
		t.Fatal("complete frozen-clock corpus lost")
	}
	for _, r := range rows {
		t.Run(r.ID, func(t *testing.T) {
			now, e := time.Parse(time.RFC3339Nano, r.Now)
			if e != nil {
				t.Fatal(e)
			}
			before := now
			calls := 0
			tr := &cycle4Offline{}
			api := sber.NewOperationsAPI(tr, sber.ResourceOptions{Now: func() time.Time { calls++; return now }})
			out, e := api.Collect(context.Background(), sber.OperationsQuery{Resource: "card:fixture", Limit: 2, MaxPages: 1})
			if (e == nil) != r.Valid {
				t.Fatalf("source default validity %s: %v", r.Now, e)
			}
			if r.Valid {
				if len(tr.calls) != 1 || tr.calls[0]["from"] != r.Body["from"] || out.Metadata.RequestedFrom != r.Body["from"] {
					t.Fatalf("actual source default_from strips timezone before 120 civil days: %v vs %v", tr.calls, r.Body)
				}
			} else if len(tr.calls) != 0 {
				t.Fatal("source overflow default dispatched")
			}
			if calls != 1 || now != before || tr.forbidden != 0 || out.Metadata.BankCapProven || out.Metadata.WindowCompleteness != "unknown" {
				t.Fatal("clock freeze/input/uncertainty policy changed")
			}
		})
	}
}
