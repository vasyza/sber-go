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

func TestCycle4SourceRequestWallApplication(t *testing.T) {
	raw, e := os.ReadFile("testdata/datetime-cycle4/windows.json")
	if e != nil {
		t.Fatal(e)
	}
	var rows []struct {
		ID    string         `json:"id"`
		From  string         `json:"from_text"`
		To    string         `json:"to_text"`
		Valid bool           `json:"valid"`
		Body  map[string]any `json:"body"`
	}
	if e = json.Unmarshal(raw, &rows); e != nil {
		t.Fatal(e)
	}
	if len(rows) != 1155 {
		t.Fatalf("complete original Page corpus changed: %d", len(rows))
	}
	for _, r := range rows {
		t.Run(r.ID, func(t *testing.T) {
			for _, application := range []string{"page", "list", "collect", "iter"} {
				t.Run(application, func(t *testing.T) {
					transport := &cycle4Offline{}
					api := sber.NewOperationsAPI(transport)
					q := sber.OperationsQuery{Resource: "card:fixture", Limit: 2, MaxPages: 1, From: r.From, To: r.To}
					var err error
					switch application {
					case "page":
						_, err = api.Page(context.Background(), sber.OperationsPageOptions{Resource: q.Resource, Limit: q.Limit, From: q.From, To: q.To})
					case "list":
						_, err = api.List(context.Background(), q)
					case "iter":
						for _, e := range api.Iter(context.Background(), q) {
							if e != nil {
								err = e
							}
						}
					case "collect":
						res, e := api.Collect(context.Background(), q)
						err = e
						m := res.Metadata
						if m.BankCapProven || m.WindowCompleteness != "unknown" {
							t.Fatal("final empty fixture is not independent coverage proof")
						}
						if r.Valid {
							if m.RequestedFrom != r.Body["from"] && !(r.Body["from"] == nil && m.RequestedFrom == "") {
								t.Fatal("from metadata differs")
							}
							if m.RequestedTo != r.Body["to"] && !(r.Body["to"] == nil && m.RequestedTo == "") {
								t.Fatal("to metadata differs")
							}
							if m.PagesRead != 1 || !m.PaginationExhausted {
								t.Fatal("metadata lost")
							}
						} else if m.PagesRead != 0 || m.RequestedFrom != "" || m.RequestedTo != "" {
							t.Fatal("invalid request published successful metadata")
						}
					}
					if (err == nil) != r.Valid {
						t.Fatalf("canonical wall-validation %q..%q: err=%v want accepted=%t", r.From, r.To, err, r.Valid)
					}
					if r.Valid {
						if len(transport.calls) != 1 {
							t.Fatal("dispatch count")
						}
						body := transport.calls[0]
						canonical, e := json.Marshal(body)
						if e != nil {
							t.Fatal(e)
						}
						var converted map[string]any
						if e = json.Unmarshal(canonical, &converted); e != nil {
							t.Fatal(e)
						}
						if !reflect.DeepEqual(converted, r.Body) {
							t.Fatalf("unchanged source Page body mismatch %s vs %v", canonical, r.Body)
						}
					} else if len(transport.calls) != 0 {
						t.Fatal("source-invalid wall window dispatched")
					}
					if transport.forbidden != 0 {
						t.Fatal("forbidden call")
					}
				})
			}
		})
	}
}
func TestCycle4RequestValidationBeforeSecondTruncation(t *testing.T) {
	r := &cycle4Offline{}
	_, e := sber.NewOperationsAPI(r).Page(context.Background(), sber.OperationsPageOptions{Limit: 2, From: "2024-01-01T00:00:00.9", To: "2024-01-01T00:00:00.1"})
	if e == nil || len(r.calls) != 0 {
		t.Fatal(fmt.Sprint("microsecond wall order was erased by request formatting", e, r.calls))
	}
}
