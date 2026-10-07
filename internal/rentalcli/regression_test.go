package rentalcli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/vasyza/sber-go/rental"
)

// Synthetic proof is an owner assertion fixture, never bank history.
func syntheticProofDocument(t testing.TB) map[string]any {
	t.Helper()
	in := syntheticInput()
	in.Evidence = []rental.CollectionEvidence{{
		TenantID: in.Tenants[0].ID, CoverageStart: in.Tenants[0].LedgerStart,
		CoverageThrough: in.AsOf, ObservedAt: in.AsOf,
		Complete: true, OwnerReconciled: true,
		HasGaps: false, Truncated: false, PageUncertain: false,
	}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	return document
}

func documentBytes(t testing.TB, document map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func firstObject(document map[string]any, collection string) map[string]any {
	return document[collection].([]any)[0].(map[string]any)
}

func assertSchemaRejected(t testing.TB, data []byte) {
	t.Helper()
	before := bytes.Clone(data)
	var out, diagnostics bytes.Buffer
	code := Run(bytes.NewReader(data), &out, &diagnostics)
	if code != 3 || out.Len() != 0 || diagnostics.String() != "The preview input format is not valid.\n" {
		t.Fatalf("expected static schema failure with no decisions; exit=%d stdout=%s stderr=%q", code, out.Bytes(), diagnostics.String())
	}
	if !bytes.Equal(data, before) {
		t.Fatal("preview modified input bytes")
	}
}

type previewDecisions struct {
	BankChecked *bool                      `json:"bank_authorization_checked"`
	Reminders   *bool                      `json:"reminders_enabled"`
	AsOf        string                     `json:"as_of"`
	Periods     []rental.PeriodResult      `json:"periods"`
	Allocations []rental.Allocation        `json:"allocations"`
	Candidates  []rental.ReminderCandidate `json:"candidate_decisions"`
	Review      []rental.ReviewItem        `json:"owner_review"`
	Totals      []rental.TenantTotal       `json:"totals"`
}

func decodeDecisions(t testing.TB, data []byte) previewDecisions {
	t.Helper()
	var result previewDecisions
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.BankChecked == nil || result.Reminders == nil || *result.BankChecked || *result.Reminders {
		t.Fatal("preview missing false live flags")
	}
	return result
}

func previewDocument(t testing.TB, document map[string]any) previewDecisions {
	t.Helper()
	data := documentBytes(t, document)
	before := bytes.Clone(data)
	var out, diagnostics bytes.Buffer
	if code := Run(bytes.NewReader(data), &out, &diagnostics); code != 0 || diagnostics.Len() != 0 {
		t.Fatalf("expected successful preview; exit=%d stderr=%q", code, diagnostics.String())
	}
	if !bytes.Equal(data, before) {
		t.Fatal("preview modified input bytes")
	}
	return decodeDecisions(t, out.Bytes())
}

func assertDecision(t testing.TB, result previewDecisions, state rental.State, candidates, allocations int) {
	t.Helper()
	if len(result.Periods) != 1 || result.Periods[0].State != state || len(result.Candidates) != candidates || len(result.Allocations) != allocations {
		t.Fatalf("expected state=%s candidates=%d allocations=%d; got %+v", state, candidates, allocations, result)
	}
}
