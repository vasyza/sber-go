package rentalcli

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/vasyza/sber-go/rental"
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
			if code := Run(bytes.NewReader(bad), &out, &diagnostics); code != 3 || out.Len() != 0 || diagnostics.String() != "invalid preview JSON\n" {
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
