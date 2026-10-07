package rental_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vasyza/sber-go/rental"
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
	dir := filepath.Dir(current)
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
