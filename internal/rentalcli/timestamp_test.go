package rentalcli

import (
	"strings"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

type timestampPosition struct {
	name   string
	object map[string]any
	key    string
}

func timestampPositions(document map[string]any) []timestampPosition {
	return []timestampPosition{
		{"AsOf", document, "AsOf"},
		{"Tenant.LedgerStart", firstObject(document, "Tenants"), "LedgerStart"},
		{"Period.Start", firstObject(document, "Periods"), "Start"},
		{"Period.End", firstObject(document, "Periods"), "End"},
		{"Period.DueAt", firstObject(document, "Periods"), "DueAt"},
		{"Receipt.ReceivedAt", firstObject(document, "Receipts"), "ReceivedAt"},
		{"Evidence.CoverageStart", firstObject(document, "Evidence"), "CoverageStart"},
		{"Evidence.CoverageThrough", firstObject(document, "Evidence"), "CoverageThrough"},
		{"Evidence.ObservedAt", firstObject(document, "Evidence"), "ObservedAt"},
	}
}

func TestPreviewRejectsUnsupportedPrecisionInEveryTimestamp(t *testing.T) {
	for position := range 9 {
		for _, fraction := range []string{"0000000001", "0000000000", "12345678901234567890"} {
			document := syntheticProofDocument(t)
			field := timestampPositions(document)[position]
			t.Run(field.name+"/"+fraction, func(t *testing.T) {
				field.object[field.key] = strings.TrimSuffix(field.object[field.key].(string), "Z") + "." + fraction + "Z"
				assertSchemaRejected(t, documentBytes(t, document))
			})
		}
	}
}

func TestPreviewRejectsMalformedCalendarClockOffsetAndFraction(t *testing.T) {
	for _, value := range []string{
		"026-03-01T00:00:00Z", "02026-03-01T00:00:00Z", "2026-3-01T00:00:00Z", "2026-03-1T00:00:00Z",
		"2026-03-01t00:00:00Z", "2026-03-01T00:00:00z", "2026-03-01 00:00:00Z",
		"2026-03-01T00:0:00Z", "2026-03-01T00:00:0Z", "2026-03-01T24:00:00Z",
		"2026-03-01T00:60:00Z", "2026-03-01T00:00:60Z", "2026-03-01T00:00:00.Z",
		"2026-03-01T00:00:00.１２Z", "2026-03-01T00:00:00.1e2Z", "2026-03-01T00:00:00.1.2Z",
		"2026-03-01T00:00:00+0:00", "2026-03-01T00:00:00+00:0", "2026-03-01T00:00:00+0000",
		"2026-03-01T00:00:00-00:60", "2026-03-01T00:00:00-24:00", "2026-03-01T00:00:00+99:99",
		"2026-03-01T00:00:00", "2026-02-29T00:00:00Z", "2026-02-30T00:00:00Z",
		"2026-00-01T00:00:00Z", "2026-13-01T00:00:00Z", "2026-03-00T00:00:00Z", "2026-04-31T00:00:00Z",
		" 2026-03-01T00:00:00Z", "2026-03-01T00:00:00Z ", "2026-03-01T00:00:00Z\n",
	} {
		t.Run(value, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document["AsOf"] = value
			assertSchemaRejected(t, documentBytes(t, document))
		})
	}
}

func TestPreviewAcceptsSupportedPrecisionAndOffsetsWithoutCanonicalRoundTrip(t *testing.T) {
	values := []string{
		"2024-02-29T00:00:00Z", "2026-03-01T00:00:00+00:00", "2026-03-01T00:00:00-00:00",
		"2026-03-01T00:00:00+03:00", "2026-03-01T00:00:00-05:30",
		"2026-03-01T00:00:00+23:59", "2026-03-01T00:00:00-23:59",
	}
	for digits := 0; digits <= 9; digits++ {
		fraction := ""
		if digits > 0 {
			fraction = "." + strings.Repeat("1", digits)
		}
		values = append(values, "2026-03-01T00:00:00"+fraction+"Z")
		if digits > 0 {
			values = append(values, "2026-03-01T00:00:00."+strings.Repeat("0", digits)+"Z")
		}
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document["AsOf"] = value
			result := previewDocument(t, document)
			// The leap-day control precedes the synthetic receipt and DueAt.
			if strings.HasPrefix(value, "2024-") {
				assertDecision(t, result, rental.NotDue, 0, 0)
			} else {
				assertDecision(t, result, rental.Paid, 0, 1)
			}
			want, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				t.Fatal(err)
			}
			got, err := time.Parse(time.RFC3339Nano, result.AsOf)
			if err != nil || !got.Equal(want) {
				t.Fatalf("supported instant changed: input=%s output=%s", value, result.AsOf)
			}
		})
	}
}

func TestPreviewRejectsExplicitNullTimestampInEveryPosition(t *testing.T) {
	for position := range 9 {
		document := syntheticProofDocument(t)
		field := timestampPositions(document)[position]
		t.Run(field.name, func(t *testing.T) {
			field.object[field.key] = nil
			assertSchemaRejected(t, documentBytes(t, document))
		})
	}
}

func TestPreviewRejectsNormalizedTimestampSpellings(t *testing.T) {
	for _, value := range []string{
		"2026-03-01T0:00:00Z",
		"2026-03-01T00:00:00,1Z",
		"2026-03-01T00:00:00+00:60",
		"2026-03-01T00:00:00+24:00",
	} {
		t.Run(value, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document["AsOf"] = value
			assertSchemaRejected(t, documentBytes(t, document))
		})
	}
}

func TestPreviewRejectsSubnanosecondReceipt(t *testing.T) {
	document := syntheticProofDocument(t)
	firstObject(document, "Receipts")["ReceivedAt"] = "2026-03-01T00:00:00.0000000001Z"
	assertSchemaRejected(t, documentBytes(t, document))
}
