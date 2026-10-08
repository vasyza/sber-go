package sber_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	raw, e := os.ReadFile("../../testdata/datetime/sorts.json")
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

func TestCycle4CanonicalBankStrptime(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/datetime/bank-dates.json")
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

// Expected values are from actual Python3.12.3 independent DATE/DATETIME and
// unchanged original Page execution, retained in legacy-fuzz-source-witness.json.
// This does not weaken or replace the sealed foreign cycle3 fuzz assertion.
func TestCycle4LegacyFuzzIndependentDateWitness(t *testing.T) {
	const raw = "0001010100"
	if _, e := sber.ParseSourceDateTime(raw); e == nil {
		t.Fatal("invalid DATETIME admitted")
	}
	if !sber.OperationSortKey(raw).IsZero() || (sber.TimeFilter{}).Contains(raw) {
		t.Fatal("invalid DATETIME acquired native key")
	}
	day, e := sber.ParseSourceDate(raw)
	if e != nil || day.Format("2006-01-02") != "0001-01-01" {
		t.Fatal("valid independent DATE rejected")
	}
	for _, constructor := range []func(string, string) (sber.TimeFilter, error){sber.NewTimeFilter, sber.NewSourceTimeFilter} {
		f, e := constructor(raw, raw)
		if e != nil {
			t.Fatal("source-valid DATE rejected to appease invalid-DATETIME implication")
		}
		a, b, e := f.SourceRequestBounds()
		if e != nil || a != "01.01.1T00:00:00" || b != "01.01.1T23:59:59" || f.Contains(raw) {
			t.Fatal("independent DATE window or native DATETIME policy changed")
		}
	}
	r := &cycle4Offline{}
	out, e := sber.NewOperationsAPI(r).Collect(context.Background(), sber.OperationsQuery{Resource: "card:fixture", Limit: 2, MaxPages: 1, From: raw, To: raw})
	if e != nil || len(r.calls) != 1 || r.calls[0]["from"] != "01.01.1T00:00:00" || r.calls[0]["to"] != "01.01.1T23:59:59" || out.Metadata.WindowCompleteness != "unknown" || out.Metadata.BankCapProven {
		t.Fatal("source-valid DATE application window or uncertainty changed")
	}
}

type cycle4Offline struct {
	calls           []map[string]any
	dates           []string
	page, forbidden int
}

func (r *cycle4Offline) PostRead(_ context.Context, path string, body map[string]any) (map[string]any, error) {
	if path != sber.OperationsPath {
		return nil, errors.New("unexpected path")
	}
	r.calls = append(r.calls, body)
	start := r.page * 100
	end := start + 100
	if end > len(r.dates) {
		end = len(r.dates)
	}
	r.page++
	ops := []any{}
	if start < len(r.dates) {
		for i := start; i < end; i++ {
			ops = append(ops, map[string]any{"uohId": fmt.Sprintf("synthetic-%d", i), "date": r.dates[i]})
		}
		if end < len(r.dates) {
			ops = append(ops, map[string]any{"uohId": fmt.Sprintf("synthetic-%d", end), "date": r.dates[end]})
		}
	}
	return map[string]any{"body": map[string]any{"operations": ops}}, nil
}
func (r *cycle4Offline) Mutate(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error) {
	r.forbidden++
	return nil, errors.New("forbidden")
}
func (r *cycle4Offline) MutationSequence(context.Context, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	r.forbidden++
	return errors.New("forbidden")
}
func (r *cycle4Offline) ExportSession() (sber.SessionBundle, error) {
	r.forbidden++
	return sber.SessionBundle{}, errors.New("forbidden")
}
func (r *cycle4Offline) ExportCredentials() (sber.SberCredentials, error) {
	r.forbidden++
	return sber.SberCredentials{}, errors.New("forbidden")
}
func (r *cycle4Offline) WarmUp(context.Context, bool) error {
	r.forbidden++
	return errors.New("forbidden")
}

func TestCycle4IndependentDateInference(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/datetime/dates.json")
	if e != nil {
		t.Fatal(e)
	}
	var values []struct {
		ID            string `json:"id"`
		Text          string `json:"text"`
		DateValid     bool   `json:"date_valid"`
		DatetimeValid bool   `json:"datetime_valid"`
		BoundsValid   bool   `json:"bounds_valid"`
		DateISO       string `json:"date_iso"`
		SourceISO     string `json:"source_iso"`
		FromRequest   string `json:"from_request"`
		ToRequest     string `json:"to_request"`
	}
	if e = json.Unmarshal(raw, &values); e != nil {
		t.Fatal(e)
	}
	if len(values) != 66 {
		t.Fatalf("complete canonical fixture changed: %d", len(values))
	}
	for _, r := range values {
		t.Run(r.ID, func(t *testing.T) {
			source, err := sber.ParseSourceDateTime(r.Text)
			if (err == nil) != r.DatetimeValid {
				t.Fatalf("independent DATETIME scanner %q: %v", r.Text, err)
			}
			if err == nil && source.ISOFormat() != r.SourceISO {
				t.Fatalf("datetime meaning changed: %q", source.ISOFormat())
			}
			if err == nil && source.DateOnly() != r.DateValid {
				t.Errorf("DATE validity must be independently inferred for %q", r.Text)
			}
			f, err := sber.NewTimeFilter(r.Text, r.Text)
			if (err == nil) != r.BoundsValid {
				t.Fatalf("date-first bounds %q: %v, want valid=%t", r.Text, err, r.BoundsValid)
			}
			if err == nil {
				a, b, err := f.SourceRequestBounds()
				if err != nil || a != r.FromRequest || b != r.ToRequest {
					t.Errorf("source DATE-first bounds %q: %q/%q %v, want %q/%q", r.Text, a, b, err, r.FromRequest, r.ToRequest)
				}
			}
			transport := &cycle4Offline{}
			_, err = sber.NewOperationsAPI(transport).Page(context.Background(), sber.OperationsPageOptions{Resource: "card:fixture", Limit: 2, From: r.Text, To: r.Text})
			if (err == nil) != r.BoundsValid {
				t.Fatalf("real Page validity %q: %v", r.Text, err)
			}
			if err == nil && (len(transport.calls) != 1 || transport.calls[0]["from"] != r.FromRequest || transport.calls[0]["to"] != r.ToRequest) {
				t.Fatalf("real request lost independent date meaning %q: %v", r.Text, transport.calls)
			}
			if transport.forbidden != 0 {
				t.Fatal("forbidden call")
			}
		})
	}
}
func TestCycle4NativeInstantWindowCompatibility(t *testing.T) {
	_, e := sber.NewTimeFilter("2010-03-28T02:45:00", "2010-03-28T03:15:00")
	if e == nil {
		t.Fatal("native instant-order constructor must remain rejected")
	}
	f, e := sber.NewTimeFilter("2010-03-28T03:15:00", "2010-03-28T02:45:00")
	if e != nil || f.From == nil || f.To == nil {
		t.Fatalf("native instant-order constructor changed: %v", e)
	}
	native := time.Date(2024, 1, 1, 0, 0, 0, 123456789, time.UTC)
	before := native
	f = sber.TimeFilter{From: &native, To: &native}
	_, _, e = f.SourceRequestBounds()
	if e != nil || native != before || f.Contains("2024-01-01T00:00:00.1234569Z") {
		t.Fatal("native precision or bytes changed")
	}
}

func TestCycle4CanonicalDefaultWindowCivilClock(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/datetime/defaults.json")
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

func TestCycle4SourceRequestWallApplication(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/datetime/windows.json")
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
							if m.RequestedFrom != r.Body["from"] && (r.Body["from"] != nil || m.RequestedFrom != "") {
								t.Fatal("from metadata differs")
							}
							if m.RequestedTo != r.Body["to"] && (r.Body["to"] != nil || m.RequestedTo != "") {
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

func TestCycle4CanonicalSortApplication(t *testing.T) {
	raw, e := os.ReadFile("../../testdata/datetime/sorts.json")
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
