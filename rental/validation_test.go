package rental_test

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

func syntheticReceipt() rental.Receipt {
	return rental.Receipt{ID: "synthetic-receipt", TenantID: "synthetic-parking", Confirmed: true, ReceivedAt: instant("2026-01-10T00:00:00Z"), Method: rental.Cash, Amount: rental.Money{Minor: 1, Currency: "RUB"}}
}

func assertFailClosed(t *testing.T, in rental.Input) {
	t.Helper()
	got, err := rental.Evaluate(in)
	if err == nil || !reflect.DeepEqual(got, rental.Evaluation{}) {
		t.Fatalf("invalid input must fail closed with an empty evaluation: err=%v output=%+v", err, got)
	}
	if !strings.HasPrefix(err.Error(), "rental:") {
		t.Fatalf("error must provide rental diagnostic metadata: %v", err)
	}
}

func TestOverflowFailsClosedAfterIdentityDedupButMaxInt64IsExact(t *testing.T) {
	t.Run("receipt-overflow", func(t *testing.T) {
		in := syntheticInput()
		r := syntheticReceipt()
		r.Amount.Minor = math.MaxInt64
		other := r
		other.ID = "synthetic-other"
		other.Amount.Minor = 1
		in.Receipts = []rental.Receipt{r, other}
		assertFailClosed(t, in)
	})
	t.Run("obligation-overflow", func(t *testing.T) {
		in := syntheticInput()
		in.Periods[0].Price.Minor = math.MaxInt64
		p := in.Periods[0]
		p.ID = "synthetic-next"
		p.Start = p.End
		p.End = instant("2026-03-01T00:00:00Z")
		p.Price.Minor = 1
		in.Periods = append(in.Periods, p)
		assertFailClosed(t, in)
	})
	t.Run("unresolved-overflow", func(t *testing.T) {
		in := syntheticInput()
		r := syntheticReceipt()
		r.TenantID = ""
		r.Confirmed = false
		r.Amount.Minor = math.MaxInt64
		other := r
		other.ID = "synthetic-other"
		other.Amount.Minor = 1
		in.Receipts = []rental.Receipt{r, other}
		assertFailClosed(t, in)
	})
	t.Run("max-int64-dedup-exact", func(t *testing.T) {
		in := syntheticInput()
		in.Periods[0].Price.Minor = math.MaxInt64
		r := syntheticReceipt()
		r.Amount.Minor = math.MaxInt64
		in.Receipts = []rental.Receipt{r, r}
		got, err := rental.Evaluate(in)
		if err != nil || got.Periods()[0].State != rental.Paid || got.Totals()[0].ReceivedMinor != math.MaxInt64 || got.Totals()[0].AppliedMinor != math.MaxInt64 {
			t.Fatalf("safe exact upper bound must survive dedup before summation: err=%v output=%+v", err, got)
		}
	})
}

func TestUnsafeIDsCurrencyReferencesAndContradictoryPeriodsFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*rental.Input)
	}{
		{"no-tenants", func(in *rental.Input) { in.Tenants = nil }},
		{"duplicate-tenant", func(in *rental.Input) { in.Tenants = append(in.Tenants, in.Tenants[0]) }},
		{"duplicate-period", func(in *rental.Input) { in.Periods = append(in.Periods, in.Periods[0]) }},
		{"overlapping-period", func(in *rental.Input) {
			p := in.Periods[0]
			p.ID = "synthetic-overlap"
			p.Start = instant("2026-01-15T00:00:00Z")
			in.Periods = append(in.Periods, p)
		}},
		{"zero-length-period", func(in *rental.Input) { in.Periods[0].End = in.Periods[0].Start }},
		{"reversed-period", func(in *rental.Input) { in.Periods[0].End = in.Periods[0].Start.Add(-time.Second) }},
		{"period-before-ledger", func(in *rental.Input) { in.Tenants[0].LedgerStart = instant("2026-01-02T00:00:00Z") }},
		{"due-before-ledger", func(in *rental.Input) { in.Periods[0].DueAt = instant("2025-12-31T00:00:00Z") }},
		{"unknown-period-tenant", func(in *rental.Input) { in.Periods[0].TenantID = "synthetic-prefix" }},
		{"unknown-receipt-tenant", func(in *rental.Input) { in.Receipts[0].TenantID = "synthetic-prefix" }},
		{"padded-id", func(in *rental.Input) { in.Tenants[0].ID = " synthetic-parking" }},
		{"control-id", func(in *rental.Input) { in.Receipts[0].ID = "synthetic\x00receipt" }},
		{"invalid-utf8-id", func(in *rental.Input) { in.Periods[0].ID = "synthetic\xff" }},
		{"lowercase-currency", func(in *rental.Input) { in.Tenants[0].Currency = "rub" }},
		{"malformed-currency", func(in *rental.Input) { in.Receipts[0].Amount.Currency = "R12" }},
		{"currency-conversion-period", func(in *rental.Input) { in.Periods[0].Price.Currency = "USD" }},
		{"currency-conversion-receipt", func(in *rental.Input) { in.Receipts[0].Amount.Currency = "USD" }},
		{"negative-price", func(in *rental.Input) { in.Periods[0].Price.Minor = -1 }},
		{"negative-receipt", func(in *rental.Input) { in.Receipts[0].Amount.Minor = -1 }},
		{"unsupported-method", func(in *rental.Input) { in.Receipts[0].Method = "guessed-method" }},
		{"unsafe-year", func(in *rental.Input) { in.AsOf = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticInput()
			in.Receipts = []rental.Receipt{syntheticReceipt()}
			tc.mutate(&in)
			assertFailClosed(t, in)
		})
	}
}

func TestMissingRequiredContractFieldsFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*rental.Input)
	}{
		{"as-of", func(in *rental.Input) { in.AsOf = time.Time{} }},
		{"tenant-id", func(in *rental.Input) { in.Tenants[0].ID = "" }},
		{"tenant-currency", func(in *rental.Input) { in.Tenants[0].Currency = "" }},
		{"ledger-start", func(in *rental.Input) { in.Tenants[0].LedgerStart = time.Time{} }},
		{"period-id", func(in *rental.Input) { in.Periods[0].ID = "" }},
		{"period-tenant", func(in *rental.Input) { in.Periods[0].TenantID = "" }},
		{"period-start", func(in *rental.Input) { in.Periods[0].Start = time.Time{} }},
		{"period-end", func(in *rental.Input) { in.Periods[0].End = time.Time{} }},
		{"period-due", func(in *rental.Input) { in.Periods[0].DueAt = time.Time{} }},
		{"period-price", func(in *rental.Input) { in.Periods[0].Price.Minor = 0 }},
		{"period-currency", func(in *rental.Input) { in.Periods[0].Price.Currency = "" }},
		{"receipt-id", func(in *rental.Input) { in.Receipts[0].ID = "" }},
		{"receipt-time", func(in *rental.Input) { in.Receipts[0].ReceivedAt = time.Time{} }},
		{"receipt-currency", func(in *rental.Input) { in.Receipts[0].Amount.Currency = "" }},
		{"receipt-amount", func(in *rental.Input) { in.Receipts[0].Amount.Minor = 0 }},
		{"receipt-method", func(in *rental.Input) { in.Receipts[0].Method = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticInput()
			in.Receipts = []rental.Receipt{syntheticReceipt()}
			tc.mutate(&in)
			assertFailClosed(t, in)
		})
	}
}
