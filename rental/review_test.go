package rental_test

import (
	"errors"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
)

// One parking and two motorcycle ledgers; identifiers are intentionally similar.
// This is a SYNTHETIC model shape, not an owner's actual tenants or mappings.
func syntheticThreeInput() rental.Input {
	in := syntheticInput()
	for _, id := range []string{"synthetic-moto-Anna", "synthetic-moto-Anna-M"} {
		tenant := in.Tenants[0]
		tenant.ID = id
		period := in.Periods[0]
		period.ID = id + "-jan"
		period.TenantID = id
		in.Tenants = append(in.Tenants, tenant)
		in.Periods = append(in.Periods, period)
	}
	in.Evidence = completeEvidence(in)
	return in
}

func reviewHasReason(review rental.ReviewItem, want string) bool {
	for _, reason := range review.Reasons {
		if string(reason) == want {
			return true
		}
	}
	return false
}

func TestReceiptsAfterExplicitAsOfStayOwnerVisibleWithoutAffectingSnapshotDebt(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*rental.Receipt)
	}{
		{"confirmed-exact", func(r *rental.Receipt) {}},
		{"unconfirmed-exact", func(r *rental.Receipt) { r.Confirmed = false }},
		{"unmapped", func(r *rental.Receipt) { r.TenantID = "" }},
		{"ambiguous", func(r *rental.Receipt) { r.TenantID = ""; r.PossibleTenantIDs = []string{"synthetic-moto-Anna"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticThreeInput()
			r := syntheticReceipt()
			r.Amount.Minor = 250000
			r.ReceivedAt = in.AsOf.Add(time.Second)
			tc.mutate(&r)
			in.Receipts = []rental.Receipt{r}
			got, err := rental.Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Allocations()) != 0 || len(got.Credits()) != 0 || len(got.Candidates()) != 3 {
				t.Fatalf("receipt after explicit AsOf must not change historical snapshot: %+v", got)
			}
			if len(got.Review()) != 1 || !reviewHasReason(got.Review()[0], "RECEIPT_AFTER_AS_OF") {
				t.Fatalf("out-of-snapshot receipt must remain owner-visible rather than disappearing: %+v", got.Review())
			}
			for _, total := range got.Totals() {
				if total.ReceivedMinor != 0 {
					t.Fatal("future-to-AsOf money entered confirmed totals")
				}
			}
		})
	}
}

func TestReceiptIdentityConflictsWhenPossibleTenantScopeChanges(t *testing.T) {
	for _, changed := range [][]string{{"synthetic-moto-Anna-M"}, {"synthetic-moto-Anna", "synthetic-moto-Anna-M"}, nil} {
		in := syntheticThreeInput()
		r := syntheticReceipt()
		r.TenantID = ""
		r.PossibleTenantIDs = []string{"synthetic-moto-Anna"}
		other := r
		other.PossibleTenantIDs = changed
		in.Receipts = []rental.Receipt{r, other}
		assertFailClosed(t, in)
		_, err := rental.Evaluate(in)
		var detail *rental.InputError
		if !errors.As(err, &detail) || detail.Code != rental.ConflictingReceipt {
			t.Fatalf("conflicting global receipt identity requires typed diagnostic: %v", err)
		}
	}
}

func TestContradictoryOrUnknownPossibleTenantHintsFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*rental.Receipt)
	}{
		{"exact-and-possible", func(r *rental.Receipt) { r.TenantID = "synthetic-parking" }},
		{"unknown-hint", func(r *rental.Receipt) { r.PossibleTenantIDs = []string{"synthetic-moto"} }},
		{"duplicate-hint", func(r *rental.Receipt) { r.PossibleTenantIDs = []string{"synthetic-moto-Anna", "synthetic-moto-Anna"} }},
		{"empty-hint", func(r *rental.Receipt) { r.PossibleTenantIDs = []string{""} }},
		{"padded-hint", func(r *rental.Receipt) { r.PossibleTenantIDs = []string{" synthetic-moto-Anna"} }},
		{"currency-mismatch", func(r *rental.Receipt) { r.Amount.Currency = "USD" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := syntheticThreeInput()
			r := syntheticReceipt()
			r.TenantID = ""
			r.PossibleTenantIDs = []string{"synthetic-moto-Anna"}
			tc.mutate(&r)
			in.Receipts = []rental.Receipt{r}
			assertFailClosed(t, in)
		})
	}
}

func TestAmbiguousFundsUseOnlyExplicitPossibleTenantScopeWithoutGuessing(t *testing.T) {
	for _, possible := range [][]string{{"synthetic-moto-Anna"}, {"synthetic-moto-Anna-M", "synthetic-moto-Anna"}} {
		t.Run(possible[0], func(t *testing.T) {
			in := syntheticThreeInput()
			r := syntheticReceipt()
			r.TenantID = ""
			r.PossibleTenantIDs = possible
			r.Amount.Minor = 250000
			same := r
			same.PossibleTenantIDs = append([]string(nil), possible...)
			for i, j := 0, len(same.PossibleTenantIDs)-1; i < j; i, j = i+1, j-1 {
				same.PossibleTenantIDs[i], same.PossibleTenantIDs[j] = same.PossibleTenantIDs[j], same.PossibleTenantIDs[i]
			}
			in.Receipts = []rental.Receipt{r, same}
			got, err := rental.Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Candidates()) != 3-len(possible) || len(got.Allocations()) != 0 || len(got.Review()) != 1 {
				t.Fatalf("a possible-tenant hint is never exact mapping or positive funds, even when unique: output=%+v", got)
			}
			review := got.Review()[0]
			if len(review.AffectedTenantIDs) != len(possible) || !reviewHasReason(review, "RECEIPT_AMBIGUOUS") {
				t.Fatalf("review must retain only explicit possible-tenant scope: %+v", review)
			}
			for _, p := range got.Periods() {
				possibleMatch := false
				for _, id := range possible {
					possibleMatch = possibleMatch || p.Period.TenantID == id
				}
				if possibleMatch && p.State != rental.Unknown || !possibleMatch && p.State != rental.Due {
					t.Fatalf("ambiguity must be scoped exactly, never to similar-name prefixes: %+v", p)
				}
			}
		})
	}
}

func TestUnconfirmedExactReceiptBlocksOnlyExactlyMappedTenant(t *testing.T) {
	in := syntheticThreeInput()
	r := syntheticReceipt()
	r.TenantID = "synthetic-moto-Anna"
	r.Confirmed = false
	in.Receipts = []rental.Receipt{r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Candidates()) != 2 || len(got.Allocations()) != 0 {
		t.Fatalf("unconfirmed funds block only exact mapping without becoming positive funds: candidates=%+v allocations=%+v", got.Candidates(), got.Allocations())
	}
	for _, p := range got.Periods() {
		if p.Period.TenantID == r.TenantID {
			if p.State != rental.Unknown || !hasReason(p, "UNRESOLVED_FUNDS") {
				t.Fatalf("mapped unconfirmed funds could affect this ledger: %+v", p)
			}
		} else if p.State != rental.Due {
			t.Fatalf("similar tenant IDs must not be merged or blocked by prefix: %+v", p)
		}
	}
	review := got.Review()
	if len(review) != 1 || len(review[0].AffectedTenantIDs) != 1 || review[0].AffectedTenantIDs[0] != r.TenantID || !reviewHasReason(review[0], "RECEIPT_UNCONFIRMED") {
		t.Fatalf("unconfirmed receipt needs owner review scoped by explicit mapping: %+v", review)
	}
	for _, total := range got.Totals() {
		if total.ReceivedMinor != 0 {
			t.Fatal("unconfirmed money was counted as confirmed")
		}
	}
}

func TestUnmappedFundsReturnOwnerReviewAndBlockEveryPositiveLedger(t *testing.T) {
	in := syntheticThreeInput()
	r := syntheticReceipt()
	r.TenantID = ""
	in.Receipts = []rental.Receipt{r, r} // The review itself must also be deduplicated.
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Allocations()) != 0 || len(got.Candidates()) != 0 {
		t.Fatalf("blank exact mapping must never allocate or permit a false debt reminder: allocations=%+v candidates=%+v", got.Allocations(), got.Candidates())
	}
	for _, p := range got.Periods() {
		if p.State != rental.Unknown || !hasReason(p, "UNRESOLVED_FUNDS") {
			t.Fatalf("even a single unresolved minor unit blocks potentially affected debt: %+v", p)
		}
	}
	review := got.Review()
	if len(review) != 1 || review[0].ReceiptID != r.ID || review[0].Amount != r.Amount || len(review[0].AffectedTenantIDs) != 3 || !reviewHasReason(review[0], "RECEIPT_UNMAPPED") {
		t.Fatalf("unmapped receipt cannot be silently dropped; owner needs compact metadata: %+v", review)
	}
	known := syntheticReceipt()
	known.ID = "synthetic-known-paid"
	known.Amount.Minor = in.Periods[0].Price.Minor
	in.Receipts = append(in.Receipts, known)
	paid, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paid.Periods() {
		if p.Period.TenantID == "synthetic-parking" && p.State != rental.Paid {
			t.Fatal("additional unresolved history cannot undo confirmed PAID")
		}
	}
	if len(paid.Candidates()) != 0 || len(paid.Review()) != 1 {
		t.Fatal("paid exception must neither hide owner review nor expose other unknown debts")
	}
}
