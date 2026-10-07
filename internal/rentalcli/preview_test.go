package rentalcli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

// All data here is SYNTHETIC, not owner contracts or actual payments.
func syntheticInput() rental.Input {
	instant := func(value string) time.Time {
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			panic(err)
		}
		return t
	}
	start := instant("2026-01-01T00:00:00Z")
	return rental.Input{
		AsOf:     instant("2026-03-01T00:00:00Z"),
		Tenants:  []rental.Tenant{{ID: "SYNTHETIC-ledger", Currency: "RUB", LedgerStart: start}},
		Periods:  []rental.Period{{ID: "SYNTHETIC-period", TenantID: "SYNTHETIC-ledger", Start: start, End: instant("2026-02-01T00:00:00Z"), DueAt: start, Price: rental.Money{Minor: 250000, Currency: "RUB"}}},
		Receipts: []rental.Receipt{{ID: "SYNTHETIC-cash", TenantID: "SYNTHETIC-ledger", ReceivedAt: start, Method: rental.Cash, Confirmed: true, Amount: rental.Money{Minor: 250000, Currency: "RUB"}}},
	}
}

func TestPreviewEmitsExplicitPaidDecisionWithoutLiveClaims(t *testing.T) {
	input, err := json.Marshal(syntheticInput())
	if err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := Run(bytes.NewReader(input), &out, &diagnostics); code != 0 {
		t.Fatalf("preview exit %d", code)
	}
	var value struct {
		BankChecked bool                  `json:"bank_authorization_checked"`
		Reminders   bool                  `json:"reminders_enabled"`
		Periods     []rental.PeriodResult `json:"periods"`
	}
	if err := json.Unmarshal(out.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.BankChecked || value.Reminders || len(value.Periods) != 1 || value.Periods[0].State != rental.Paid {
		t.Fatal("incorrect preview or live claims")
	}
}

func TestPreviewRejectsUnknownKeysAndStructCaseAliases(t *testing.T) {
	data, err := json.Marshal(syntheticInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(string(data), `"Confirmed":true`, `"Confirmed":false,"confirmed":true`, 1),
		strings.Replace(string(data), `"AsOf":`, `"asof":`, 1),
		strings.Replace(string(data), `{`, `{"password":"SYNTHETIC-never-echo",`, 1),
	} {
		var out, diagnostics bytes.Buffer
		if code := Run(strings.NewReader(bad), &out, &diagnostics); code != 3 {
			t.Fatalf("unsafe schema accepted, exit %d", code)
		}
		if out.Len() != 0 || strings.Contains(diagnostics.String(), "SYNTHETIC-never-echo") {
			t.Fatal("unsafe schema printed private input")
		}
	}
}

func TestPreviewRejectsOversizedWholeDocument(t *testing.T) {
	data, err := json.Marshal(syntheticInput())
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Repeat(" ", 1<<20) + string(data)
	var out, diagnostics bytes.Buffer
	if code := Run(strings.NewReader(raw), &out, &diagnostics); code != 3 {
		t.Fatalf("oversized document accepted, exit %d", code)
	}
	if out.Len() != 0 {
		t.Fatal("oversized document emitted decisions")
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }
func TestPreviewDoesNotReportSuccessForShortOutput(t *testing.T) {
	data, err := json.Marshal(syntheticInput())
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	if code := Run(bytes.NewReader(data), shortWriter{}, &diagnostics); code != 5 {
		t.Fatalf("short output accepted, exit %d", code)
	}
}

func TestPreviewUnknownHistoryNeverHasCandidates(t *testing.T) {
	in := syntheticInput()
	in.Receipts = nil
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := Run(bytes.NewReader(data), &out, &diagnostics); code != 0 {
		t.Fatalf("unknown preview exit %d", code)
	}
	var value struct {
		Periods    []rental.PeriodResult      `json:"periods"`
		Candidates []rental.ReminderCandidate `json:"candidate_decisions"`
	}
	if err := json.Unmarshal(out.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Periods) != 1 || value.Periods[0].State != rental.Unknown || len(value.Candidates) != 0 {
		t.Fatal("unknown history became debt candidate")
	}
}

func TestPreviewInvalidDocumentsNeverEmitPartialDecisions(t *testing.T) {
	data, err := json.Marshal(syntheticInput())
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{`{}`, `{"AsOf":"bad"}`, string(data) + `{}`, strings.Replace(string(data), `"Confirmed":true`, `"Confirmed":true,"Confirmed":false`, 1), strings.Replace(string(data), `"Minor":250000`, `"Minor":250000.5`, 1), strings.Replace(string(data), `"Minor":250000`, `"Minor":9223372036854775808`, 1), strings.Replace(string(data), `"ID":"SYNTHETIC-cash"`, `"ID":"\ud800"`, 1), strings.Replace(string(data), `"Confirmed":true`, `"Confirmed":null`, 1)}
	for _, raw := range cases {
		var out, diagnostics bytes.Buffer
		if code := Run(strings.NewReader(raw), &out, &diagnostics); code == 0 {
			t.Fatal("invalid input succeeded")
		}
		if out.Len() != 0 {
			t.Fatal("invalid input emitted partial decisions")
		}
	}
}

func FuzzPreviewNeverClaimsLiveIntegration(f *testing.F) {
	data, _ := json.Marshal(syntheticInput())
	f.Add(data)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"Confirmed":true,"confirmed":false}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 4096 {
			t.Skip()
		}
		var out, diagnostics bytes.Buffer
		code := Run(bytes.NewReader(input), &out, &diagnostics)
		if code != 0 {
			if out.Len() != 0 {
				t.Fatal("error emitted partial decisions")
			}
			return
		}
		var value map[string]any
		if err := json.Unmarshal(out.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if value["reminders_enabled"] != false || value["bank_authorization_checked"] != false {
			t.Fatal("preview claimed live authority")
		}
	})
}
