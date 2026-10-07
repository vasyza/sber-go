package rentalcli

import (
	"github.com/vasyza/sber-go/rental"
	"slices"
	"strings"
	"testing"
	"time"
)

func fuzzTimestampSpelling(instant time.Time, spelling uint8, offset int16) string {
	full := instant.UTC().Format("2006-01-02T15:04:05.000000000Z")
	switch spelling % 10 {
	case 0:
		return instant.UTC().Format(time.RFC3339Nano)
	case 1:
		return full // Supported trailing zeroes need not round-trip byte-for-byte.
	case 2:
		zone := time.FixedZone("SYNTHETIC-offset", int(offset%1440)*60)
		return instant.In(zone).Format(time.RFC3339Nano)
	case 3:
		return strings.TrimSuffix(full, "Z") + "0Z" // Even a redundant tenth digit is unsupported.
	case 4:
		return strings.TrimSuffix(full, "Z") + "1Z"
	case 5:
		return strings.Replace(full, ".", ",", 1)
	case 6:
		return full[:11] + "0" + full[13:]
	case 7:
		return strings.TrimSuffix(full, "Z") + "+00:60"
	case 8:
		return strings.Replace(full, "T", "t", 1)
	default:
		return full[:19] + ".Z"
	}
}

func FuzzPreviewTemporalCutoffs(f *testing.F) {
	for kind := uint8(0); kind < 6; kind++ {
		for _, nanos := range []uint32{0, 2} {
			for _, delta := range []int8{-1, 0, 1} {
				f.Add(kind, nanos, delta, kind%3, int16(-179))
			}
		}
		for spelling := uint8(3); spelling < 10; spelling++ {
			f.Add(kind, uint32(2), int8(1), spelling, int16(0))
		}
	}
	f.Fuzz(func(t *testing.T, kind uint8, nanos uint32, delta int8, spelling uint8, offset int16) {
		document := syntheticProofDocument(t)
		proof := firstObject(document, "Evidence")
		asOf := time.Date(2026, 3, 1, 0, 0, 0, int(nanos%1_000_000_000), time.UTC)
		document["AsOf"] = asOf.Format(time.RFC3339Nano)
		proof["CoverageThrough"], proof["ObservedAt"] = document["AsOf"], document["AsOf"]
		step := time.Duration(delta) * time.Nanosecond
		instant := asOf.Add(step)
		field := proof
		key := "ObservedAt"
		want := rental.Due
		candidates, allocations := 1, 0
		var reason rental.Reason
		if kind%6 != 1 {
			document["Receipts"] = nil
		}
		switch kind % 6 {
		case 0:
			field, key = firstObject(document, "Periods"), "DueAt"
			if delta > 0 {
				want, candidates = rental.NotDue, 0
			}
		case 1:
			field, key = firstObject(document, "Receipts"), "ReceivedAt"
			if delta <= 0 {
				want, candidates, allocations = rental.Paid, 0, 1
			}
		case 2:
			key = "CoverageThrough"
			if delta < 0 {
				want, candidates, reason = rental.Unknown, 0, rental.EvidenceCoverageGap
			} else {
				proof["ObservedAt"] = instant.Format(time.RFC3339Nano)
			}
		case 3:
			if delta < 0 {
				want, candidates, reason = rental.Unknown, 0, rental.EvidenceStale
			}
		case 4:
			// Observation is always >= AsOf here, but must also meet coverage.
			through := asOf.Add(256 * time.Nanosecond)
			proof["CoverageThrough"] = through.Format(time.RFC3339Nano)
			instant = through.Add(step)
			if delta < 0 {
				want, candidates, reason = rental.Unknown, 0, rental.EvidenceStale
			}
		case 5:
			key = "CoverageStart"
			instant = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(step)
			if delta > 0 {
				want, candidates, reason = rental.Unknown, 0, rental.EvidenceCoverageGap
			}
		}
		field[key] = fuzzTimestampSpelling(instant, spelling, offset)
		if spelling%10 >= 3 {
			assertSchemaRejected(t, documentBytes(t, document))
			return
		}
		result := previewDocument(t, document)
		assertDecision(t, result, want, candidates, allocations)
		if reason != "" && !slices.Contains(result.Periods[0].Reasons, reason) {
			t.Fatalf("temporal proof blocker lost: %s", reason)
		}
		gotAsOf, err := time.Parse(time.RFC3339Nano, result.AsOf)
		if err != nil || !gotAsOf.Equal(asOf) {
			t.Fatal("supported reference instant changed")
		}
		if kind%6 == 0 && !result.Periods[0].Period.DueAt.Equal(instant) {
			t.Fatal("supported independent DueAt changed")
		}
		if kind%6 == 1 && delta > 0 {
			if len(result.Review) != 1 || !slices.Contains(result.Review[0].Reasons, rental.ReceiptAfterAsOf) || !result.Review[0].ReceivedAt.Equal(instant) || result.Totals[0].ReceivedMinor != 0 {
				t.Fatal("future receipt availability or exact review instant changed")
			}
		}
	})
}

func FuzzPreviewProofAcceptance(f *testing.F) {
	for mask := uint8(0); mask < 32; mask++ {
		f.Add(mask, uint8(0), uint8(0), false)
		f.Add(mask, uint8(0), uint8(0), true)
	}
	for form := uint8(1); form < 15; form++ {
		f.Add(uint8(3), form, uint8(0), false)
		f.Add(uint8(3), form, uint8(0), true)
	}
	for temporal := uint8(1); temporal < 3; temporal++ {
		f.Add(uint8(3), uint8(0), temporal, false)
		f.Add(uint8(3), uint8(0), temporal, true)
	}
	f.Fuzz(func(t *testing.T, mask uint8, form uint8, temporal uint8, funded bool) {
		document := syntheticProofDocument(t)
		proof := firstObject(document, "Evidence")
		if !funded {
			document["Receipts"] = nil
		}
		proof["Complete"], proof["OwnerReconciled"] = mask&1 != 0, mask&2 != 0
		negative := []string{"HasGaps", "Truncated", "PageUncertain"}
		for i, field := range negative {
			proof[field] = mask&(4<<i) != 0
		}
		var temporalReason rental.Reason
		switch temporal % 3 {
		case 1:
			proof["CoverageThrough"] = "2026-02-28T23:59:59.999999999Z"
			temporalReason = rental.EvidenceCoverageGap
		case 2:
			proof["CoverageThrough"] = "2026-03-01T00:00:00.000000001Z"
			temporalReason = rental.EvidenceStale
		}
		invalid := false
		missing := false
		missingBoundary := false
		switch form % 15 {
		case 1, 2, 3:
			delete(proof, negative[form%15-1])
			invalid = true
		case 4:
			proof[negative[int(mask>>5)%3]] = nil
			invalid = true
		case 5:
			proof[negative[int(mask>>5)%3]] = "false"
			invalid = true
		case 6:
			delete(document, "Evidence")
			missing = true
		case 7:
			document["Evidence"] = nil
			missing = true
		case 8:
			document["Evidence"] = []any{}
			missing = true
		case 9:
			delete(proof, []string{"CoverageStart", "CoverageThrough", "ObservedAt"}[int(mask>>5)%3])
			missingBoundary = true
		case 10:
			proof["Complete"] = nil
			invalid = true
		case 11:
			document["Evidence"] = []any{nil}
			invalid = true
		case 12:
			proof["hasgaps"] = false
			invalid = true
		case 13:
			delete(proof, "Complete")
			mask &^= 1
		case 14:
			delete(proof, "OwnerReconciled")
			mask &^= 2
		}
		if invalid {
			assertSchemaRejected(t, documentBytes(t, document))
			return
		}
		result := previewDocument(t, document)
		if funded {
			assertDecision(t, result, rental.Paid, 0, 1)
			return
		}
		var reasons []rental.Reason
		if missing {
			reasons = []rental.Reason{rental.MissingEvidence}
		} else {
			if mask&1 == 0 {
				reasons = append(reasons, rental.EvidenceIncomplete)
			}
			if mask&2 == 0 {
				reasons = append(reasons, rental.OwnerReconciliationMissing)
			}
			if missingBoundary {
				reasons = append(reasons, rental.EvidenceBoundariesMissing)
			} else if temporalReason != "" {
				reasons = append(reasons, temporalReason)
			}
			for i, reason := range []rental.Reason{rental.EvidenceHasGaps, rental.EvidenceTruncated, rental.EvidencePageUncertain} {
				if mask&(4<<i) != 0 {
					reasons = append(reasons, reason)
				}
			}
		}
		if len(reasons) == 0 {
			assertDecision(t, result, rental.Due, 1, 0)
		} else {
			assertDecision(t, result, rental.Unknown, 0, 0)
		}
		if !slices.Equal(result.Periods[0].Reasons, reasons) {
			t.Fatalf("proof facts changed: got=%v want=%v", result.Periods[0].Reasons, reasons)
		}
	})
}
