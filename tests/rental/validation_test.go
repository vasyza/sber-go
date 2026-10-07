package rental_test

import (
	"github.com/vasyza/sber-go/rental"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExactMappedFundsNeverSpillToSimilarTenantIDs(t *testing.T) {
	in := syntheticThreeInput()
	r := syntheticReceipt()
	r.TenantID = "synthetic-moto-Anna-M"
	r.Amount.Minor = 250001
	in.Receipts = []rental.Receipt{r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got.Periods() {
		if p.Period.TenantID == r.TenantID {
			if p.State != rental.Paid {
				t.Fatal("exact mapped motorcycle receipt did not pay its ledger")
			}
		} else if p.Applied.Minor != 0 || p.State != rental.Due {
			t.Fatal("similar synthetic IDs were merged or excess spilled to another tenant")
		}
	}
	if len(got.Credits()) != 1 || got.Credits()[0].TenantID != r.TenantID || got.Credits()[0].Amount.Minor != 1 || len(got.Candidates()) != 2 {
		t.Fatal("excess credit lost exact tenant ownership")
	}
}

func TestDueAndReceiptBoundariesUseExplicitAsOfInclusively(t *testing.T) {
	in := syntheticInput()
	in.AsOf = in.Periods[0].DueAt
	in.Evidence = completeEvidence(in)
	r := syntheticReceipt()
	r.ReceivedAt = in.AsOf
	r.Amount.Minor = in.Periods[0].Price.Minor - 1
	in.Receipts = []rental.Receipt{r}
	atDue, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if atDue.Periods()[0].State != rental.Due || len(atDue.Candidates()) != 1 || atDue.Candidates()[0].Remaining.Minor != 1 {
		t.Fatal("explicit due/receipt timestamp boundary must be inclusive")
	}
	in.AsOf = in.AsOf.Add(-time.Nanosecond)
	in.Evidence = completeEvidence(in)
	beforeDue, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if beforeDue.Periods()[0].State != rental.NotDue || len(beforeDue.Candidates()) != 0 || len(beforeDue.Allocations()) != 0 || len(beforeDue.Review()) != 1 {
		t.Fatal("one nanosecond before explicit due must not remind or allocate after-AsOf receipts")
	}
	in.Evidence = nil
	unknownNotDue, err := rental.Evaluate(in)
	if err != nil || unknownNotDue.Periods()[0].State != rental.Unknown || len(unknownNotDue.Candidates()) != 0 {
		t.Fatal("not-due cannot manufacture complete history or a reminder")
	}
}

func TestContractAnchorsPreserveOriginalTimeZoneRepresentation(t *testing.T) {
	in := syntheticInput()
	zone := time.FixedZone("synthetic-contract-zone", 3*60*60)
	in.Periods[0].Start = in.Periods[0].Start.In(zone)
	in.Periods[0].End = in.Periods[0].End.In(zone)
	in.Periods[0].DueAt = in.Periods[0].DueAt.In(zone)
	r := syntheticReceipt()
	r.ReceivedAt = in.AsOf
	r.Amount.Minor = in.Periods[0].Price.Minor
	in.Receipts = []rental.Receipt{r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Periods()[0].Period, in.Periods[0]) {
		t.Fatal("late receipts must not mutate any contractual anchor or its representation")
	}
}

func TestExplicitEmptyPeriodSetKeepsFundsAsCreditWithoutInference(t *testing.T) {
	in := syntheticInput()
	in.Periods = nil // Explicitly no obligations; never infer a contract from receipt time/amount.
	r := syntheticReceipt()
	r.Amount.Minor = 80001
	in.Receipts = []rental.Receipt{r}
	got, err := rental.Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Periods()) != 0 || len(got.Candidates()) != 0 || len(got.Allocations()) != 0 || len(got.Credits()) != 1 || got.Credits()[0].Amount.Minor != r.Amount.Minor || got.Totals()[0].ObligationMinor != 0 {
		t.Fatal("missing explicitly supplied periods must remain credit, not become inferred rent dates")
	}
}

func TestPackageIsPureStandardLibraryAndHasNoDeliveryOrWallClock(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate owned package source")
	}
	dir := filepath.Join(filepath.Dir(current), "..", "..", "rental")
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	approved := map[string]bool{"math": true, "sort": true, "slices": true, "strings": true, "time": true, "unicode": true, "unicode/utf8": true}
	forbiddenClock := map[string]bool{"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "AfterFunc": true, "NewTimer": true, "NewTicker": true, "Tick": true}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") || strings.HasSuffix(file.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file.Name()), nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil || !approved[path] {
				t.Fatalf("pure native package acquired disallowed import: %s", imp.Path.Value)
			}
			if imp.Name != nil {
				t.Fatal("purity audit rejects import aliases/dot imports that hide wall-clock or delivery access")
			}
		}
		for _, decl := range f.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				t.Fatal("pure engine must not hold mutable package globals")
			}
			if fun, ok := decl.(*ast.FuncDecl); ok && fun.Name.Name == "init" {
				t.Fatal("pure engine must not initialize external runtime state")
			}
		}
		ast.Inspect(f, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.GoStmt, *ast.SendStmt, *ast.SelectStmt:
				t.Fatal("pure engine must not contain concurrency delivery/scheduling operations")
			case *ast.BasicLit:
				if n.Kind == token.FLOAT {
					t.Fatal("rental money must not use binary float literals")
				}
			case *ast.Ident:
				if n.Name == "float32" || n.Name == "float64" || n.Name == "complex64" || n.Name == "complex128" {
					t.Fatal("pure engine must not use inexact money types")
				}
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && id.Name == "time" && forbiddenClock[n.Sel.Name] {
					t.Fatal("evaluation may only use explicit AsOf, not wall-clock/scheduling")
				}
			}
			return true
		})
	}
}

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
