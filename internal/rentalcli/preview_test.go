package rentalcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/vasyza/sber-go/rental"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPreviewRejectsNonBooleanNegativeProofFlags(t *testing.T) {
	for _, field := range []string{"HasGaps", "Truncated", "PageUncertain"} {
		for _, value := range []any{nil, "false", json.Number("0"), []any{}, map[string]any{}} {
			t.Run(field+"/"+string(documentBytes(t, map[string]any{"value": value})), func(t *testing.T) {
				document := syntheticProofDocument(t)
				firstObject(document, "Evidence")[field] = value
				assertSchemaRejected(t, documentBytes(t, document))
			})
		}
	}
}

func TestPreviewExplicitNegativeProofFlagsPreserveSafeAndUnsafeDecisions(t *testing.T) {
	for _, field := range []string{"HasGaps", "Truncated", "PageUncertain"} {
		for _, unsafe := range []bool{false, true} {
			name := field + "/false"
			if unsafe {
				name = field + "/true"
			}
			t.Run(name, func(t *testing.T) {
				document := syntheticProofDocument(t)
				document["Receipts"] = nil
				firstObject(document, "Evidence")[field] = unsafe
				result := previewDocument(t, document)
				if !unsafe {
					assertDecision(t, result, rental.Due, 1, 0)
					return
				}
				assertDecision(t, result, rental.Unknown, 0, 0)
				reason := map[string]rental.Reason{"HasGaps": rental.EvidenceHasGaps, "Truncated": rental.EvidenceTruncated, "PageUncertain": rental.EvidencePageUncertain}[field]
				if !slices.Contains(result.Periods[0].Reasons, reason) {
					t.Fatalf("explicit unsafe assertion lost: %s", reason)
				}
			})
		}
	}
}

func TestPreviewMissingProofAndPositiveAssertionsStayUnknown(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
		reason rental.Reason
	}{
		{"evidence-absent", func(d map[string]any) { delete(d, "Evidence") }, rental.MissingEvidence},
		{"evidence-null", func(d map[string]any) { d["Evidence"] = nil }, rental.MissingEvidence},
		{"evidence-empty", func(d map[string]any) { d["Evidence"] = []any{} }, rental.MissingEvidence},
		{"complete-omitted", func(d map[string]any) { delete(firstObject(d, "Evidence"), "Complete") }, rental.EvidenceIncomplete},
		{"reconciled-omitted", func(d map[string]any) { delete(firstObject(d, "Evidence"), "OwnerReconciled") }, rental.OwnerReconciliationMissing},
		{"only-explicit-negatives", func(d map[string]any) {
			d["Evidence"] = []any{map[string]any{"TenantID": "SYNTHETIC-ledger", "HasGaps": false, "Truncated": false, "PageUncertain": false}}
		}, rental.EvidenceBoundariesMissing},
	}
	for _, test := range cases {
		for _, funded := range []bool{false, true} {
			name := test.name + "/unfunded"
			if funded {
				name = test.name + "/funded"
			}
			t.Run(name, func(t *testing.T) {
				document := syntheticProofDocument(t)
				if !funded {
					document["Receipts"] = nil
				}
				test.change(document)
				result := previewDocument(t, document)
				if funded {
					assertDecision(t, result, rental.Paid, 0, 1)
					return
				}
				assertDecision(t, result, rental.Unknown, 0, 0)
				if !slices.Contains(result.Periods[0].Reasons, test.reason) {
					t.Fatalf("missing explicit unknown reason %s", test.reason)
				}
			})
		}
	}
}

func TestPreviewMissingAndZeroProofBoundariesStayUnknown(t *testing.T) {
	for _, field := range []string{"CoverageStart", "CoverageThrough", "ObservedAt"} {
		for _, omit := range []bool{false, true} {
			name := field + "/zero"
			if omit {
				name = field + "/omitted"
			}
			t.Run(name, func(t *testing.T) {
				document := syntheticProofDocument(t)
				document["Receipts"] = nil
				proof := firstObject(document, "Evidence")
				if omit {
					delete(proof, field)
				} else {
					proof[field] = "0001-01-01T00:00:00Z"
				}
				result := previewDocument(t, document)
				assertDecision(t, result, rental.Unknown, 0, 0)
				if !slices.Contains(result.Periods[0].Reasons, rental.EvidenceBoundariesMissing) {
					t.Fatal("missing proof boundary became a fact")
				}
			})
		}
	}
}

func TestPreviewMalformedEvidenceCannotBeHiddenByPaidFundsOrAnotherRecord(t *testing.T) {
	for _, value := range []any{map[string]any{}, []any{nil}, []any{true}, []any{map[string]any{}}, "unknown"} {
		document := syntheticProofDocument(t)
		document["Evidence"] = value
		assertSchemaRejected(t, documentBytes(t, document))
	}
	for _, field := range []string{"HasGaps", "Truncated", "PageUncertain"} {
		t.Run("second-record/"+field, func(t *testing.T) {
			document := syntheticProofDocument(t)
			second := firstObject(syntheticProofDocument(t), "Evidence")
			second["TenantID"] = "SYNTHETIC-other-ledger"
			delete(second, field)
			document["Evidence"] = append(document["Evidence"].([]any), second)
			assertSchemaRejected(t, documentBytes(t, document))
		})
	}
}

func TestPreviewProofCaseAliasesAndDecodedDuplicateKeysRemainRejected(t *testing.T) {
	data := string(documentBytes(t, syntheticProofDocument(t)))
	for _, field := range []string{"HasGaps", "Truncated", "PageUncertain", "Complete", "OwnerReconciled"} {
		needle := `"` + field + `":`
		alias := `"` + strings.ToLower(field) + `":true,` + needle
		t.Run("case-alias/"+field, func(t *testing.T) {
			assertSchemaRejected(t, []byte(strings.Replace(data, needle, alias, 1)))
		})
	}
	for _, spell := range []string{`"HasGaps":true,`, `"Has\u0047aps":true,`} {
		t.Run("duplicate/"+spell, func(t *testing.T) {
			bad := []byte(strings.Replace(data, `"HasGaps":`, spell+`"HasGaps":`, 1))
			var out, diagnostics bytes.Buffer
			if code := Run(bytes.NewReader(bad), &out, &diagnostics); code != 3 || out.Len() != 0 || diagnostics.String() != "The preview JSON is not valid.\n" {
				t.Fatalf("decoded duplicate proof accepted: exit=%d stderr=%q", code, diagnostics.String())
			}
		})
	}
	// Escaping a canonical decoded key is permitted; byte round-trip equality
	// is not the rule, and is not an oracle for the duplicate tests above.
	document := syntheticProofDocument(t)
	document["Receipts"] = nil
	escaped := strings.Replace(string(documentBytes(t, document)), `"HasGaps":`, `"Has\u0047aps":`, 1)
	var out, diagnostics bytes.Buffer
	if code := Run(strings.NewReader(escaped), &out, &diagnostics); code != 0 || diagnostics.Len() != 0 {
		t.Fatalf("valid escaped canonical key rejected: %d", code)
	}
	assertDecision(t, decodeDecisions(t, out.Bytes()), rental.Due, 1, 0)
}

func TestPreviewRequiresExplicitNegativeProofFlags(t *testing.T) {
	for _, field := range []string{"HasGaps", "Truncated", "PageUncertain"} {
		t.Run(field, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document["Receipts"] = nil
			delete(firstObject(document, "Evidence"), field)
			assertSchemaRejected(t, documentBytes(t, document))
		})
	}
}

func TestPreviewRequiredContractTimesKeepNativeMissingAndZeroErrors(t *testing.T) {
	for position := range 6 {
		for _, omit := range []bool{false, true} {
			document := syntheticProofDocument(t)
			field := timestampPositions(document)[position]
			name := field.name + "/zero"
			if omit {
				name = field.name + "/omitted"
			}
			t.Run(name, func(t *testing.T) {
				if omit {
					delete(field.object, field.key)
				} else {
					field.object[field.key] = "0001-01-01T00:00:00Z"
				}
				data := documentBytes(t, document)
				before := bytes.Clone(data)
				var out, diagnostics bytes.Buffer
				if code := Run(bytes.NewReader(data), &out, &diagnostics); code != 4 || out.Len() != 0 || diagnostics.String() != "The rental ledger is not valid.\n" {
					t.Fatalf("required contract time changed native zero policy: exit=%d stderr=%q", code, diagnostics.String())
				}
				if !bytes.Equal(data, before) {
					t.Fatal("failed preview modified input")
				}
			})
		}
	}
}

type countingInput struct {
	reader io.Reader
	read   int
}

func (r *countingInput) Read(data []byte) (int, error) {
	n, err := r.reader.Read(data)
	r.read += n
	return n, err
}

func TestPreviewBoundedReadAndExactSizeBoundary(t *testing.T) {
	data := documentBytes(t, syntheticProofDocument(t))
	for _, extra := range []int{0, 1, MaximumInputBytes} {
		t.Run("bytes-"+strconv.Itoa(MaximumInputBytes+extra), func(t *testing.T) {
			padded := append(bytes.Clone(data), bytes.Repeat([]byte(" "), MaximumInputBytes-len(data)+extra)...)
			reader := &countingInput{reader: bytes.NewReader(padded)}
			var out, diagnostics bytes.Buffer
			code := Run(reader, &out, &diagnostics)
			if extra == 0 {
				if code != 0 || diagnostics.Len() != 0 || reader.read != MaximumInputBytes {
					t.Fatalf("exact-size valid document rejected: exit=%d bytes=%d", code, reader.read)
				}
				assertDecision(t, decodeDecisions(t, out.Bytes()), rental.Paid, 0, 1)
			} else if code != 3 || out.Len() != 0 || diagnostics.String() != "The preview JSON is not valid.\n" || reader.read != MaximumInputBytes+1 {
				t.Fatalf("size/read bound changed: exit=%d bytes=%d stderr=%q", code, reader.read, diagnostics.String())
			}
		})
	}
}

type failingInput struct {
	data []byte
}

func (r failingInput) Read(out []byte) (int, error) {
	return copy(out, r.data), errors.New("SYNTHETIC-private-cause")
}

func TestPreviewIOFailuresRemainStaticAndDecisionFree(t *testing.T) {
	data := documentBytes(t, syntheticProofDocument(t))
	var out, diagnostics bytes.Buffer
	if code := Run(failingInput{data}, &out, &diagnostics); code != 3 || out.Len() != 0 || diagnostics.String() != "The preview JSON is not valid.\n" {
		t.Fatalf("partial read failure became success or disclosed cause: %d %q", code, diagnostics.String())
	}
	out.Reset()
	diagnostics.Reset()
	if code := Run(nil, &out, &diagnostics); code != 2 || out.Len() != 0 || diagnostics.String() != "The preview input or output is not valid.\n" {
		t.Fatalf("nil input did not fail closed: %d", code)
	}
	diagnostics.Reset()
	if code := Run(bytes.NewReader(data), nil, &diagnostics); code != 2 || diagnostics.String() != "The preview input or output is not valid.\n" {
		t.Fatalf("nil output did not fail closed: %d", code)
	}
	if code := Run(strings.NewReader(`{"AsOf":null}`), &out, nil); code != 3 || out.Len() != 0 {
		t.Fatal("nil diagnostics writer changed schema rejection")
	}
}

func TestPreviewNilCollectionsKeepNativeEngineSemantics(t *testing.T) {
	for _, collection := range []string{"Tenants", "Periods", "Receipts", "Evidence"} {
		t.Run(collection, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document[collection] = nil
			if collection == "Tenants" {
				var out, diagnostics bytes.Buffer
				if code := Run(bytes.NewReader(documentBytes(t, document)), &out, &diagnostics); code != 4 || out.Len() != 0 || diagnostics.String() != "The rental ledger is not valid.\n" {
					t.Fatal("nil required tenants became a ledger")
				}
				return
			}
			result := previewDocument(t, document)
			switch collection {
			case "Periods":
				if len(result.Periods) != 0 || len(result.Candidates) != 0 {
					t.Fatal("nil periods invented obligations")
				}
			case "Receipts":
				assertDecision(t, result, rental.Due, 1, 0)
			case "Evidence":
				assertDecision(t, result, rental.Paid, 0, 1)
			}
		})
	}
}

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
