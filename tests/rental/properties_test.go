package rental_test

import (
	"encoding/json"
	"fmt"
	"github.com/vasyza/sber-go/rental"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestConcurrentEvaluationsAndAccessorMutationAreIndependent(t *testing.T) {
	const workers = 32
	const iterations = 128
	in := syntheticMixedInput()
	before := cloneInput(in)
	baseline, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	want := encodedView(t, baseline)
	problems := make(chan string, workers)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				got, err := rental.Evaluate(in)
				if err != nil {
					problems <- "concurrent evaluation rejected valid synthetic input"
					return
				}
				encoded, err := json.Marshal(view(got))
				if err != nil || string(encoded) != want {
					problems <- "concurrent evaluation changed deterministic output"
					return
				}
				got.Periods()[0].Applied.Minor = -1
				baseline.Review()[0].AffectedTenantIDs[0] = "mutated"
				baseline.Review()[0].Reasons[0] = "mutated"
				encoded, err = json.Marshal(view(baseline))
				if err != nil || string(encoded) != want {
					problems <- "concurrent accessor mutation changed shared evaluation"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(problems)
	for problem := range problems {
		t.Error(problem)
	}
	if !reflect.DeepEqual(in, before) {
		t.Error("concurrent evaluations mutated shared input")
	}
	t.Logf("SYNTHETIC workers=%d iterations_per_worker=%d", workers, iterations)
}

func sample(data []byte, i int) byte {
	if len(data) == 0 {
		return 0
	}
	return data[i%len(data)]
}

// Obligations/dates are configured independently of the receipts. All generated
// ledgers are SYNTHETIC and bounded; no supplied input is owner/bank data.
func generatedInput(data []byte) rental.Input {
	in := rental.Input{AsOf: instant("2026-02-10T00:00:00Z")}
	start := instant("2026-01-01T00:00:00Z")
	for i, currency := range []string{"RUB", "RUB", "USD"} {
		id := fmt.Sprintf("synthetic-ledger-%d", i)
		in.Tenants = append(in.Tenants, rental.Tenant{ID: id, Currency: currency, LedgerStart: start})
		for month := 0; month < 2; month++ {
			periodStart := start.AddDate(0, month, 0)
			in.Periods = append(in.Periods, rental.Period{ID: fmt.Sprintf("%s-period-%d", id, month), TenantID: id,
				Start: periodStart, End: start.AddDate(0, month+1, 0), DueAt: periodStart.AddDate(0, 0, 4),
				Price: rental.Money{Minor: 1 + int64(sample(data, 2+i+month)), Currency: currency}})
		}
	}
	in.Evidence = completeEvidence(in)
	for i := range in.Evidence {
		flag := sample(data, 5+i)
		in.Evidence[i].Complete = flag&1 != 0
		in.Evidence[i].OwnerReconciled = flag&2 != 0
		in.Evidence[i].Truncated = flag&4 != 0
	}
	for i := 0; i < int(sample(data, 0)%17); i++ {
		tenant := in.Tenants[int(sample(data, 7+i)%3)]
		flag := sample(data, 8+i)
		r := rental.Receipt{ID: fmt.Sprintf("synthetic-generated-%d", i), TenantID: tenant.ID, Method: rental.Transfer,
			Confirmed: flag&4 != 0, ReceivedAt: start.AddDate(0, 0, i),
			Amount: rental.Money{Minor: 1 + int64(sample(data, 9+i))*16, Currency: tenant.Currency}}
		if flag&8 != 0 {
			r.ReceivedAt = in.AsOf.Add(time.Second)
		}
		switch flag % 4 {
		case 1:
			r.TenantID = ""
		case 2:
			r.TenantID = ""
			r.PossibleTenantIDs = []string{tenant.ID}
		case 3:
			r.TenantID = ""
			if tenant.Currency == "RUB" {
				r.PossibleTenantIDs = []string{in.Tenants[1].ID, in.Tenants[0].ID}
			} else {
				r.PossibleTenantIDs = []string{tenant.ID}
			}
		}
		if flag&16 != 0 {
			r.Method = rental.Cash
		}
		in.Receipts = append(in.Receipts, r)
		if flag&32 != 0 {
			in.Receipts = append(in.Receipts, r)
		}
	}
	return in
}

func checkConservationDedupAndCopies(t testing.TB, data []byte) {
	t.Helper()
	in := generatedInput(data)
	before := cloneInput(in)
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatalf("generated valid SYNTHETIC input rejected: %v", err)
	}
	if !reflect.DeepEqual(in, before) {
		t.Fatal("input immutability property failed")
	}
	seen := make(map[string]bool)
	eligible := make(map[string]int64)
	receiptAmounts := make(map[string]int64)
	for _, r := range in.Receipts {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		if r.Confirmed && r.TenantID != "" && !r.ReceivedAt.After(in.AsOf) {
			eligible[r.TenantID] += r.Amount.Minor
			receiptAmounts[r.ID] = r.Amount.Minor
		}
	}
	perReceipt := make(map[string]int64)
	perPeriod := make(map[string]int64)
	periodByID := make(map[string]rental.Period)
	for _, p := range in.Periods {
		periodByID[p.ID] = p
	}
	for _, a := range got.Allocations() {
		p, ok := periodByID[a.PeriodID]
		if !ok || a.TenantID != p.TenantID || a.Amount.Currency != p.Price.Currency || a.Amount.Minor <= 0 {
			t.Fatal("allocation escaped explicit tenant/period/currency or nonpositive money")
		}
		perReceipt[a.ReceiptID] += a.Amount.Minor
		perPeriod[a.PeriodID] += a.Amount.Minor
	}
	for _, c := range got.Credits() {
		if c.Amount.Minor <= 0 {
			t.Fatal("invalid excess credit")
		}
		perReceipt[c.ReceiptID] += c.Amount.Minor
	}
	if !reflect.DeepEqual(perReceipt, receiptAmounts) {
		t.Fatal("confirmed receipt funds were lost, invented or double-counted")
	}
	periods := got.Periods()
	for i, p := range periods {
		if p.Applied.Minor < 0 || p.Remaining.Minor < 0 || p.Applied.Minor+p.Remaining.Minor != p.Period.Price.Minor || perPeriod[p.Period.ID] != p.Applied.Minor {
			t.Fatal("period conservation property failed")
		}
		if p.Remaining.Minor == 0 && p.State != rental.Paid {
			t.Fatal("confirmed full coverage did not prove PAID")
		}
		if p.Remaining.Minor > 0 {
			for _, younger := range periods[i+1:] {
				if younger.Period.TenantID == p.Period.TenantID && younger.Applied.Minor != 0 {
					t.Fatal("FIFO skipped an older explicit obligation")
				}
			}
		}
	}
	for _, total := range got.Totals() {
		if total.ReceivedMinor != eligible[total.TenantID] || total.ReceivedMinor != total.AppliedMinor+total.CreditMinor || total.ObligationMinor != total.AppliedMinor+total.RemainingMinor {
			t.Fatal("tenant conservation property failed")
		}
	}
	originalOutput := encodedView(t, got)
	duplicated := cloneInput(in)
	duplicated.Receipts = append(duplicated.Receipts, cloneInput(in).Receipts...)
	slices.Reverse(duplicated.Receipts)
	slices.Reverse(duplicated.Tenants)
	slices.Reverse(duplicated.Periods)
	slices.Reverse(duplicated.Evidence)
	again, err := rental.Evaluate(duplicated)
	if err != nil || encodedView(t, again) != originalOutput {
		t.Fatal("global semantic dedup/permutation property failed")
	}
	for _, p := range got.Periods() {
		if len(p.Reasons) > 0 {
			p.Reasons[0] = "mutated"
		}
	}
	for _, r := range got.Review() {
		if len(r.Reasons) > 0 {
			r.Reasons[0] = "mutated"
		}
		if len(r.AffectedTenantIDs) > 0 {
			r.AffectedTenantIDs[0] = "mutated"
		}
	}
	if encodedView(t, got) != originalOutput {
		t.Fatal("deep output-copy property failed")
	}
}

func TestBoundedSyntheticConservationDedupAndInputImmutability(t *testing.T) {
	for seed := 0; seed < 1000; seed++ {
		data := make([]byte, 32)
		for i := range data {
			data[i] = byte((seed*37 + i*71 + seed*i*13) % 256)
		}
		checkConservationDedupAndCopies(t, data)
	}
	t.Log("SYNTHETIC deterministic property cases=1000")
}

func FuzzConservationDedupAndInputImmutability(f *testing.F) {
	for _, seed := range [][]byte{nil, {0}, {255}, {16, 127, 5, 3, 249, 31, 47, 159}, []byte("SYNTHETIC allocation property only")} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			data = data[:128]
		}
		checkConservationDedupAndCopies(t, data)
	})
}

func FuzzNegativeReminderGates(f *testing.F) {
	for mode := uint8(0); mode < 16; mode++ {
		f.Add(mode, uint32(1))
	}
	f.Fuzz(func(t *testing.T, mode uint8, observed uint32) {
		in := syntheticInput()
		in.Evidence = completeEvidence(in)
		r := syntheticReceipt()
		r.Amount.Minor = 1 + int64(observed%100000) // Never fully satisfies the explicit price.
		control := false
		notDue := false
		switch mode % 16 {
		case 0:
			in.Evidence = nil
		case 1:
			in.Evidence[0] = rental.CollectionEvidence{TenantID: in.Tenants[0].ID}
		case 2:
			in.Evidence[0].Complete = false
		case 3:
			in.Evidence[0].OwnerReconciled = false
		case 4:
			in.Evidence[0].CoverageStart = in.Tenants[0].LedgerStart.Add(time.Second)
		case 5:
			in.Evidence[0].CoverageThrough = in.AsOf.Add(-time.Second)
		case 6:
			in.Evidence[0].ObservedAt = in.AsOf.Add(-time.Second)
		case 7:
			in.Evidence[0].HasGaps = true
		case 8:
			in.Evidence[0].Truncated = true
		case 9:
			in.Evidence[0].PageUncertain = true
		case 10:
			r.TenantID = ""
			in.Receipts = []rental.Receipt{r}
		case 11:
			r.Confirmed = false
			in.Receipts = []rental.Receipt{r}
		case 12:
			r.TenantID = ""
			r.PossibleTenantIDs = []string{in.Tenants[0].ID}
			in.Receipts = []rental.Receipt{r}
		case 13:
			in.AsOf = in.Periods[0].DueAt.Add(-time.Nanosecond)
			in.Evidence = completeEvidence(in)
			notDue = true
		case 14:
			in.Evidence[0].CoverageStart = time.Time{}
		case 15:
			control = true
		}
		before := cloneInput(in)
		got, err := rental.Evaluate(in)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(in, before) {
			t.Fatal("negative reminder gate mutated input")
		}
		if control {
			if len(got.Candidates()) != 1 || got.Periods()[0].State != rental.Due {
				t.Fatal("healthy evidence control must permit a due metadata candidate")
			}
		} else {
			if len(got.Candidates()) != 0 {
				t.Fatal("unsafe evidence/unresolved receipt/not-due gate exposed a reminder candidate")
			}
			want := rental.Unknown
			if notDue {
				want = rental.NotDue
			}
			if got.Periods()[0].State != want {
				t.Fatal("positive remainder must be UNKNOWN unless known and not due")
			}
		}
	})
}
