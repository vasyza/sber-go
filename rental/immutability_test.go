package rental_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

type evaluationView struct {
	AsOf        time.Time
	Periods     []rental.PeriodResult
	Allocations []rental.Allocation
	Credits     []rental.Credit
	Totals      []rental.TenantTotal
	Candidates  []rental.ReminderCandidate
	Review      []rental.ReviewItem
}

func view(e rental.Evaluation) evaluationView {
	return evaluationView{e.AsOf(), e.Periods(), e.Allocations(), e.Credits(), e.Totals(), e.Candidates(), e.Review()}
}

func encodedView(t testing.TB, e rental.Evaluation) string {
	t.Helper()
	encoded, err := json.Marshal(view(e))
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func cloneInput(in rental.Input) rental.Input {
	in.Tenants = slices.Clone(in.Tenants)
	in.Periods = slices.Clone(in.Periods)
	in.Evidence = slices.Clone(in.Evidence)
	in.Receipts = slices.Clone(in.Receipts)
	for i := range in.Receipts {
		in.Receipts[i].PossibleTenantIDs = slices.Clone(in.Receipts[i].PossibleTenantIDs)
	}
	return in
}

func syntheticMixedInput() rental.Input {
	in := syntheticThreeInput()
	known := syntheticReceipt()
	known.ID = "synthetic-known"
	known.TenantID = "synthetic-moto-Anna"
	known.Amount.Minor = 250001
	unconfirmed := syntheticReceipt()
	unconfirmed.ID = "synthetic-unconfirmed"
	unconfirmed.TenantID = "synthetic-moto-Anna-M"
	unconfirmed.Confirmed = false
	in.Receipts = []rental.Receipt{known, unconfirmed}
	return in
}

func TestEvaluationAccessorsReturnDeepIndependentCopies(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(rental.Evaluation)
	}{
		{"period-struct", func(e rental.Evaluation) { e.Periods()[0].Period.ID = "mutated" }},
		{"period-reasons", func(e rental.Evaluation) {
			for _, p := range e.Periods() {
				if len(p.Reasons) > 0 {
					p.Reasons[0] = "mutated"
				}
			}
		}},
		{"allocation", func(e rental.Evaluation) { e.Allocations()[0].Amount.Minor = -1 }},
		{"credit", func(e rental.Evaluation) { e.Credits()[0].Amount.Minor = -1 }},
		{"totals", func(e rental.Evaluation) { e.Totals()[0].ReceivedMinor = -1 }},
		{"candidate", func(e rental.Evaluation) { e.Candidates()[0].TenantID = "mutated" }},
		{"review-struct", func(e rental.Evaluation) { e.Review()[0].ReceiptID = "mutated" }},
		{"review-reasons", func(e rental.Evaluation) { e.Review()[0].Reasons[0] = "mutated" }},
		{"review-affected", func(e rental.Evaluation) { e.Review()[0].AffectedTenantIDs[0] = "mutated" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rental.Evaluate(syntheticMixedInput())
			if err != nil {
				t.Fatal(err)
			}
			before := encodedView(t, got)
			tc.mutate(got)
			if encodedView(t, got) != before {
				t.Fatal("evaluation accessor leaked mutable internal storage")
			}
		})
	}
}

func TestEvaluateNeverMutatesOrRetainsCallerSlices(t *testing.T) {
	in := syntheticThreeInput()
	r := syntheticReceipt()
	r.TenantID = ""
	r.PossibleTenantIDs = []string{"synthetic-moto-Anna-M", "synthetic-moto-Anna"}
	in.Receipts = []rental.Receipt{r, r}
	slices.Reverse(in.Periods)
	slices.Reverse(in.Tenants)
	slices.Reverse(in.Evidence)
	before := cloneInput(in)
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, before) {
		t.Fatal("evaluation sorted or altered caller-owned input")
	}
	beforeOutput := encodedView(t, got)
	in.Receipts[0].PossibleTenantIDs[0] = "mutated"
	in.Receipts[0].Amount.Minor = -1
	in.Periods[0].Price.Minor = -1
	in.Tenants[0].ID = "mutated"
	in.Evidence[0].Complete = false
	if encodedView(t, got) != beforeOutput {
		t.Fatal("evaluation retained caller-owned slices")
	}
}
