package rental_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

// All test inputs, tenant IDs and prices in this package are SYNTHETIC.
// They are not owner records, contractual dates or actual tenant mappings.
func instant(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return t
}

func syntheticInput() rental.Input {
	return rental.Input{
		AsOf:    instant("2026-03-10T00:00:00Z"),
		Tenants: []rental.Tenant{{ID: "synthetic-parking", Currency: "RUB", LedgerStart: instant("2026-01-01T00:00:00Z")}},
		Periods: []rental.Period{{
			ID: "synthetic-jan", TenantID: "synthetic-parking",
			Start: instant("2026-01-01T00:00:00Z"), End: instant("2026-02-01T00:00:00Z"),
			DueAt: instant("2026-01-05T00:00:00Z"), Price: rental.Money{Minor: 250000, Currency: "RUB"},
		}},
	}
}

func TestConflictingGlobalReceiptIdentityFailsClosed(t *testing.T) {
	mutations := map[string]func(*rental.Receipt){
		"amount":       func(r *rental.Receipt) { r.Amount.Minor++ },
		"currency":     func(r *rental.Receipt) { r.Amount.Currency = "USD" },
		"tenant":       func(r *rental.Receipt) { r.TenantID = "synthetic-motorcycle" },
		"confirmation": func(r *rental.Receipt) { r.Confirmed = false },
		"timestamp":    func(r *rental.Receipt) { r.ReceivedAt = r.ReceivedAt.Add(time.Second) },
		"method":       func(r *rental.Receipt) { r.Method = rental.Transfer },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			in := syntheticInput()
			r := rental.Receipt{ID: "synthetic-global", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-02T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 80001, Currency: "RUB"}}
			conflict := r
			mutate(&conflict)
			in.Receipts = []rental.Receipt{r, conflict}
			got, err := rental.Evaluate(in)
			if err == nil || !reflect.DeepEqual(got, rental.Evaluation{}) {
				t.Fatalf("conflicting stable receipt ID must return error and no partial decisions: err=%v output=%+v", err, got)
			}
		})
	}
}

func TestIdenticalGlobalReceiptIDCountsOnceAcrossTimestampRepresentations(t *testing.T) {
	in := syntheticInput()
	r := rental.Receipt{ID: "synthetic-shared-id", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-02T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 80001, Currency: "RUB"}}
	same := r
	same.ReceivedAt = instant("2026-01-02T03:00:00+03:00") // Same instant, not a new receipt.
	in.Receipts = []rental.Receipt{r, same, r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Periods()[0].Applied.Minor != 80001 || got.Totals()[0].ReceivedMinor != 80001 || len(got.Allocations()) != 1 {
		t.Fatalf("stable receipt identity was counted more than once: periods=%+v totals=%+v allocations=%+v", got.Periods(), got.Totals(), got.Allocations())
	}
}

func TestExcessCreditIsExactWithoutInventingFuturePeriods(t *testing.T) {
	in := syntheticInput()
	in.Receipts = []rental.Receipt{{ID: "synthetic-excess", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-02T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 250001, Currency: "RUB"}}}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	credits := got.Credits()
	if len(credits) != 1 || credits[0].ReceiptID != "synthetic-excess" || credits[0].TenantID != "synthetic-parking" || credits[0].Amount.Minor != 1 || credits[0].Amount.Currency != "RUB" {
		t.Fatalf("excess minor unit must remain explicit credit: %+v", credits)
	}
	totals := got.Totals()
	if len(totals) != 1 || totals[0].ReceivedMinor != 250001 || totals[0].AppliedMinor != 250000 || totals[0].CreditMinor != 1 || totals[0].ObligationMinor != 250000 || totals[0].RemainingMinor != 0 {
		t.Fatalf("conservation totals must include excess exactly once: %+v", totals)
	}
	if len(got.Periods()) != 1 {
		t.Fatal("credit invented a period absent from the owner's input")
	}
}

func TestSeveralReceiptsApplyChronologicallyWithExactPartialRemainder(t *testing.T) {
	in := syntheticInput()
	in.Receipts = []rental.Receipt{
		{ID: "synthetic-later", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-02-02T00:00:00Z"), Method: rental.Transfer, Amount: rental.Money{Minor: 49998, Currency: "RUB"}},
		{ID: "synthetic-earlier", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-20T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 80001, Currency: "RUB"}},
	}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	p := got.Periods()[0]
	if p.Applied.Minor != 129999 || p.Remaining.Minor != 120001 || p.State != rental.Unknown {
		t.Fatalf("partial exact receipts without evidence must retain UNKNOWN balance: %+v", p)
	}
	a := got.Allocations()
	if len(a) != 2 || a[0].ReceiptID != "synthetic-earlier" || a[1].ReceiptID != "synthetic-later" {
		t.Fatalf("receipt chronology must be deterministic, not caller slice order: %+v", a)
	}
}

func TestSyntheticCash5000PrepaysTwoExplicit2500PeriodsOldestFirst(t *testing.T) {
	in := syntheticInput()
	in.AsOf = instant("2026-01-10T00:00:00Z")
	feb := in.Periods[0]
	feb.ID = "synthetic-feb"
	feb.Start = instant("2026-02-01T00:00:00Z")
	feb.End = instant("2026-03-01T00:00:00Z")
	feb.DueAt = instant("2026-02-05T00:00:00Z")
	in.Periods = []rental.Period{feb, in.Periods[0]} // Deliberately unordered.
	in.Receipts = []rental.Receipt{{
		ID: "synthetic-cash-5000", TenantID: "synthetic-parking", Confirmed: true,
		ReceivedAt: instant("2026-01-06T00:00:00Z"), Method: rental.Cash,
		Amount: rental.Money{Minor: 500000, Currency: "RUB"},
	}}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	allocations := got.Allocations()
	if len(allocations) != 2 || allocations[0].PeriodID != "synthetic-jan" || allocations[1].PeriodID != "synthetic-feb" {
		t.Fatalf("oldest explicit obligation must receive cash first: %+v", allocations)
	}
	for _, p := range got.Periods() {
		if p.State != rental.Paid || p.Applied.Minor != 250000 || p.Remaining.Minor != 0 {
			t.Fatalf("SYNTHETIC 5000 cash must cover both 2500 periods exactly: %+v", p)
		}
	}
	if got.Periods()[1].Period.Start != feb.Start || got.Periods()[1].Period.DueAt != feb.DueAt {
		t.Fatal("prepayment changed explicit future anchor")
	}
}

func TestConfirmedReceiptPaysExplicitPeriodWithoutCompleteHistory(t *testing.T) {
	in := syntheticInput()
	in.Receipts = []rental.Receipt{{
		ID: "synthetic-cash-1", TenantID: "synthetic-parking", Confirmed: true,
		ReceivedAt: instant("2026-02-20T00:00:00Z"), Method: rental.Cash,
		Amount: rental.Money{Minor: 250000, Currency: "RUB"},
	}}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	periods := got.Periods()
	if len(periods) != 1 || periods[0].State != rental.Paid || periods[0].Remaining.Minor != 0 || periods[0].Applied.Minor != 250000 {
		t.Fatalf("confirmed exact receipt must prove PAID: %+v", periods)
	}
	allocations := got.Allocations()
	if len(allocations) != 1 || allocations[0].ReceiptID != "synthetic-cash-1" || allocations[0].PeriodID != "synthetic-jan" || allocations[0].Amount.Minor != 250000 {
		t.Fatalf("missing exact allocation: %+v", allocations)
	}
	if periods[0].Period.Start != in.Periods[0].Start || periods[0].Period.DueAt != in.Periods[0].DueAt {
		t.Fatal("late receipt shifted a contractual anchor or due date")
	}
}
