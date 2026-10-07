package rental_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/vasyza/sber-go/rental"
)

func TestReceiptDedupReviewIsDeterministicAcrossOrderingAndEquivalentInstants(t *testing.T) {
	in := syntheticThreeInput()
	unresolved := syntheticReceipt()
	unresolved.ID = "synthetic-unresolved"
	unresolved.TenantID = ""
	unresolved.PossibleTenantIDs = []string{"synthetic-moto-Anna-M", "synthetic-moto-Anna"}
	unresolved.Confirmed = false
	same := unresolved
	same.ReceivedAt = instant("2026-01-10T03:00:00+03:00")
	knownA := syntheticReceipt()
	knownA.ID = "synthetic-a"
	knownA.Amount.Minor = 80001
	knownB := knownA
	knownB.ID = "synthetic-b"
	knownB.Amount.Minor = 2
	in.Receipts = []rental.Receipt{unresolved, same, knownB, knownA}
	baseline, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	reversed := cloneInput(in)
	slices.Reverse(reversed.Tenants)
	slices.Reverse(reversed.Periods)
	slices.Reverse(reversed.Receipts)
	slices.Reverse(reversed.Evidence)
	for i := range reversed.Receipts {
		slices.Reverse(reversed.Receipts[i].PossibleTenantIDs)
	}
	before := cloneInput(reversed)
	got, err := rental.Evaluate(reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, reversed) {
		t.Fatal("canonical ordering mutated input")
	}
	if encodedView(t, got) != encodedView(t, baseline) {
		t.Fatal("same semantic receipts yielded different reports because caller order or timestamp representation changed")
	}
	if len(got.Review()) != 1 || got.Review()[0].ReceivedAt.Location().String() != "UTC" {
		t.Fatal("deduplicated receipt-review instants must be canonical UTC, independently of first duplicate")
	}
}
