package sber_test

import (
	"context"
	"testing"

	sber "github.com/vasyza/sber-go"
)

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
