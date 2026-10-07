package sber_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sber "github.com/vasyza/sber-go"
	"os"
	"testing"
	"time"
)

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

type cycle4DateRow struct {
	ID, Text, DateISO, DatetimeISO, SourceISO, FromRequest, ToRequest string
	DateValid, DatetimeValid, BoundsValid                             bool
}

func TestCycle4IndependentDateInference(t *testing.T) {
	raw, e := os.ReadFile("testdata/datetime-cycle4/dates.json")
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
