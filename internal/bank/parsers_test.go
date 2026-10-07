package bank

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type domainGolden struct {
	ID   string `json:"id"`
	Data struct {
		Input         json.RawMessage `json:"input"`
		Expected      json.RawMessage `json:"expected"`
		ExpectedError string          `json:"expected_error"`
	} `json:"data"`
}

func readDomainGolden(t *testing.T, file, id string) domainGolden {
	t.Helper()
	b, err := os.ReadFile("../../testdata/compat/" + file + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SyntheticOnly bool           `json:"synthetic_only"`
		Records       []domainGolden `json:"records"`
	}
	if err = json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.SyntheticOnly {
		t.Fatal("non-synthetic fixture")
	}
	for _, r := range fixture.Records {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("missing %s", id)
	return domainGolden{}
}
func mustDecodeDomain(t *testing.T, text string) map[string]any {
	t.Helper()
	p, err := DecodeJSON(strings.NewReader(text))
	if err != nil {
		t.Fatalf("invalid synthetic fixture %q: %v", text, err)
	}
	return p
}
func assertDomainJSON(t *testing.T, got any, want json.RawMessage) {
	t.Helper()
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var a, z any
	if err = json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(want, &z); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, z) {
		t.Fatalf("JSON\ngot %s\nwant %s", b, want)
	}
}
func TestOperationsGolden(t *testing.T) {
	for _, id := range []string{"operation_amount_is_transaction_and_resource_billing_amount_is_balance_after", "plain_billing_amount_is_not_guessed_to_be_a_balance", "a5_one_malformed_optional_amount_does_not_discard_the_page", "a5_malformed_core_amount_still_raises"} {
		t.Run(id, func(t *testing.T) {
			r := readDomainGolden(t, "history", id)
			p, err := DecodeJSON(strings.NewReader(string(r.Data.Input)))
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseOperations(p)
			if r.Data.ExpectedError != "" {
				if err == nil {
					t.Fatal("bad core amount accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertDomainJSON(t, got, r.Data.Expected)
		})
	}
}
func TestOperationsScopeAndRequiredHistory(t *testing.T) {
	p := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":9007199254740993,"date":"13.07.2026T12:34:56","form":"ExtCardPayment","classificationCode":5411,"state":{"category":"executed","name":"FINANCIAL"},"correspondent":" Магазин 4111 1111 1111 1111 ","operationAmount":{"amount":-12.5,"currencyCode":"RUB"},"fromResource":{"id":"card:20"},"isFinancial":true}]}}`)
	got, err := ParseOperations(p, "card:21", "card:20", "account:x", "card:21")
	if err != nil || len(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	op := got[0]
	if op.ID != "9007199254740993" || op.ClassificationCode != "5411" || op.Date != "2026-07-13T12:34:56+03:00" || op.Merchant != "Магазин •••• 1111" || !reflect.DeepEqual(op.ScopeCardIDs, []string{"20", "21"}) || op.IsFinancial == nil || !*op.IsFinancial || op.FromResource.ID != "20" {
		t.Fatalf("full operation: %+v", op)
	}
	for _, v := range []string{`{}`, `{"body":null}`, `{"body":{}}`, `{"body":{"operations":null}}`, `{"body":{"operations":false}}`, `{"body":{"operations":{}}}`, `{"body":{"operations":[null]}}`, `{"body":{"operations":[]},"success":false}`, `{"body":{"operations":[]},"sourceErrorResponse":{"code":"fixture-error"}}`, `{"body":{"operations":[],"sourceErrorResponse":{"code":"fixture-error"}}}`, `{"body":{"operations":[{"operationAmount":{"amount":null}}]}}`} {
		raw := mustDecodeDomain(t, v)
		if _, err := ParseOperations(raw); err == nil {
			t.Fatalf("missing/malformed history accepted: %s", v)
		}
	}
	empty := mustDecodeDomain(t, `{"body":{"operations":[]}}`)
	ops, err := ParseOperations(empty)
	if err != nil || ops == nil || len(ops) != 0 {
		t.Fatalf("valid empty history: %v %v", ops, err)
	}
	// N+1 sentinels and duplicate IDs are retained by this pure parser.
	duplicate := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"same"},{"uohId":"same"}]}}`)
	if ops, err := ParseOperations(duplicate); err != nil || len(ops) != 2 {
		t.Fatalf("sentinel unexpectedly discarded: %v %v", ops, err)
	}
}

// Source: har.py:_signed_money/_ledger, exercised by tests/test_parser_selftest.py.
func TestCardLedgerSignsScopeAndDeduplication(t *testing.T) {
	d, _ := ParseDecimal("-5.00")
	zero, _ := ParseDecimal("0.00")
	operations := []Operation{
		{ID: "transfer", Amount: &Money{Amount: d, Currency: "RUB"}, FromResource: &Resource{Type: "card", ID: "20"}, ToResource: &Resource{Type: "card", ID: "21"}, ScopeCardIDs: []string{"21", "20", "22", "22"}},
		{ID: "unknown", ScopeCardIDs: []string{"B", "A"}},
		{ID: "zero", Amount: &Money{Amount: zero, Currency: "RUB"}, ScopeCardIDs: []string{"20"}},
	}
	got := BuildCardLedger(operations)
	want := `[{"operation_id":"transfer","card_id":"20","direction":"out","amount":{"amount":"-5.00","currency":"RUB"}},{"operation_id":"transfer","card_id":"21","direction":"in","amount":{"amount":"5.00","currency":"RUB"}},{"operation_id":"transfer","card_id":"22","direction":"out","amount":{"amount":"-5.00","currency":"RUB"}},{"operation_id":"unknown","card_id":"A","direction":"unknown","amount":null},{"operation_id":"unknown","card_id":"B","direction":"unknown","amount":null},{"operation_id":"zero","card_id":"20","direction":"neutral","amount":{"amount":"0.00","currency":"RUB"}}]`
	assertDomainJSON(t, got, json.RawMessage(want))
	twice := append(append([]Operation{}, operations...), operations...)
	assertDomainJSON(t, BuildCardLedger(twice), json.RawMessage(want))
	if operations[0].Amount.Amount.String() != "-5.00" {
		t.Fatal("ledger changed source amount")
	}
	got[0].Amount.Currency = "USD"
	if operations[0].Amount.Currency != "RUB" {
		t.Fatal("ledger aliases mutable Money")
	}
	positive, _ := ParseDecimal("5.00")
	if positive.Sign() != 1 || d.Sign() != -1 || zero.Sign() != 0 || d.Abs().String() != "5.00" || positive.Neg().String() != "-5.00" {
		t.Fatal("exact signed arithmetic")
	}
	if got := BuildCardLedger(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty ledger: %v", got)
	}
	// Shared scoped views must retain opposite signs; never collapse by operation ID.
	views := []Operation{{ID: "same", Amount: &Money{Amount: d, Currency: "RUB"}, ScopeCardIDs: []string{"A"}}, {ID: "same", Amount: &Money{Amount: positive, Currency: "RUB"}, ScopeCardIDs: []string{"B"}}}
	ledger := BuildCardLedger(views)
	if len(ledger) != 2 || ledger[0].Direction != "out" || ledger[1].Direction != "in" {
		t.Fatalf("shared scopes lost: %+v", ledger)
	}
}

func TestOperationDetailsGolden(t *testing.T) {
	r := readDomainGolden(t, "resources", "op_details")
	p, err := DecodeJSON(strings.NewReader(string(r.Data.Input)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseOperationDetails(p)
	if err != nil {
		t.Fatal(err)
	}
	assertDomainJSON(t, got, r.Data.Expected)
	for _, p := range []map[string]any{{}, {"body": nil}, {"body": true}, {"sourceErrorResponse": map[string]any{}, "body": map[string]any{}}} {
		if _, err := ParseOperationDetails(p); err == nil {
			t.Fatalf("malformed detail accepted: %v", p)
		}
	}
	// Optional fields are soft; mapping garbage isn't stringified.
	raw := mustDecodeDomain(t, `{"body":{"fields":[null,{"value":{"amount":"oops"}},{"value":"Карта 4111 1111 1111 1111"},{"value":true}]}}`)
	d, err := ParseOperationDetails(raw)
	if err != nil || len(d.Fields) != 4 || d.Fields[1].Value != nil || d.Fields[2].Value != "Карта •••• 1111" || d.Fields[3].Value != "True" {
		t.Fatalf("detail field fallback: %+v %v", d, err)
	}
}
func TestCardInfoGolden(t *testing.T) {
	r := readDomainGolden(t, "resources", "card_info")
	p, err := DecodeJSON(strings.NewReader(string(r.Data.Input)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseCardInfo(p)
	if err != nil {
		t.Fatal(err)
	}
	assertDomainJSON(t, got, r.Data.Expected)
	for _, raw := range []string{`{"body":{"cardDetails":{"cards":[]}}}`, `{"body":{"cardDetails":{}}}`, `{"body":{"cardDetails":{"cards":true}}}`} {
		p := mustDecodeDomain(t, raw)
		cards, err := ParseCardInfo(p)
		if err != nil || cards == nil || len(cards) != 0 {
			t.Fatalf("optional card list %v %v", cards, err)
		}
	}
	p = mustDecodeDomain(t, `{"body":{"cardDetails":{"cards":[{"name":"4111-1111-1111-1111","number":"4111 1111 1111 1111","limits":{"availableLimit":{"amount":true}},"creditType":{}}]}}}`)
	got, err = ParseCardInfo(p)
	if err != nil || len(got) != 1 || got[0].Name != "•••• 1111" || got[0].Last4 != "1111" || got[0].Limits.Available != nil || got[0].Credit != nil {
		t.Fatalf("card fallback: %+v %v", got, err)
	}
	p = mustDecodeDomain(t, `{"body":{"cardDetails":{"cards":[false]}}}`)
	if _, err := ParseCardInfo(p); err == nil {
		t.Fatal("malformed card element accepted")
	}
	if _, err := ParseCardInfo(nil); err == nil {
		t.Fatal("absent body accepted")
	}
}

func TestPFMAmountsGolden(t *testing.T) {
	r := readDomainGolden(t, "resources", "pfm")
	p, err := DecodeJSON(strings.NewReader(string(r.Data.Input)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParsePFMAmounts(p)
	if err != nil {
		t.Fatal(err)
	}
	assertDomainJSON(t, got, r.Data.Expected)
	p = mustDecodeDomain(t, `{"body":{"amounts":[{"categoryAmounts":[{"name":"Карта 4111 1111 1111 1111","countOperations":"oops","visibleAmount":{"amount":"NaN"}},{"countOperations":"3"},{"countOperations":true},{"countOperations":4.9}]}]}}`)
	got, err = ParsePFMAmounts(p)
	if err != nil || len(got.Periods) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	cs := got.Periods[0].Categories
	if len(cs) != 4 || cs[0].Name != "Карта •••• 1111" || cs[0].CountOperations != 0 || cs[0].VisibleAmount != nil || cs[1].CountOperations != 3 || cs[2].CountOperations != 1 || cs[3].CountOperations != 4 {
		t.Fatalf("PFM coercion: %+v", cs)
	}
	for _, raw := range []string{`{"body":{}}`, `{"body":{"amounts":null}}`, `{"body":{"amounts":true}}`} {
		p := mustDecodeDomain(t, raw)
		pfm, err := ParsePFMAmounts(p)
		if err != nil || pfm.Periods == nil || len(pfm.Periods) != 0 {
			t.Fatalf("optional periods: %+v %v", pfm, err)
		}
	}
	for _, raw := range []string{`{}`, `{"body":{"amounts":[null]}}`, `{"body":{"amounts":[{"categoryAmounts":[false]}]}}`} {
		p := mustDecodeDomain(t, raw)
		if _, err := ParsePFMAmounts(p); err == nil {
			t.Fatalf("bad PFM accepted %s", raw)
		}
	}
}

// Source: _http.py:_request_moment/_request_datetime/_iso_moscow; regressions A11.
func TestMoscowTimeFilterAndOperationSort(t *testing.T) {
	f, err := NewTimeFilter("2026-08-01", "2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	from, to := f.RequestBounds()
	if from != "01.08.2026T00:00:00" || to != "31.08.2026T23:59:59" {
		t.Fatalf("bounds %s %s", from, to)
	}
	for _, s := range []string{"2026-07-31T21:00:00Z", "2026-08-31T20:59:59Z", "2026-08-01T00:00:00"} {
		if !f.Contains(s) {
			t.Fatalf("inclusive boundary excluded: %s", s)
		}
	}
	for _, s := range []string{"2026-07-31T20:59:59Z", "2026-08-31T21:00:00Z", "garbage"} {
		if f.Contains(s) {
			t.Fatalf("outside/unknown accepted: %s", s)
		}
	}
	isoFrom, isoTo := f.ISOBounds()
	if isoFrom != "2026-08-01T00:00:00+03:00" || isoTo != "2026-08-31T23:59:59+03:00" {
		t.Fatalf("ISO %s %s", isoFrom, isoTo)
	}
	f, err = NewTimeFilter("2026-08-01T01:02:03Z", "2026-08-01T05:00:00+03:00")
	if err != nil {
		t.Fatal(err)
	}
	from, to = f.RequestBounds()
	if from != "01.08.2026T04:02:03" || to != "01.08.2026T05:00:00" {
		t.Fatalf("aware conversion %s %s", from, to)
	}
	if _, err := NewTimeFilter("2026-08-31", "2026-08-01"); err == nil {
		t.Fatal("inverted bounds accepted")
	}
	if _, err := NewTimeFilter("invalid", ""); err == nil {
		t.Fatal("invalid bound accepted")
	}
	if _, err := NewTimeFilter("2026-02-30", ""); err == nil {
		t.Fatal("invalid date accepted")
	}
	f, err = NewTimeFilter("", "")
	if err != nil || !f.Contains("2026-08-01T00:00:00Z") {
		t.Fatal("unbounded filter")
	}
	dates := []string{"2026-01-31T00:00:00+03:00", "31.01.2026T00:00:00.000", "2026-02-01T00:00:00+03:00"}
	if !OperationSortKey(dates[2]).After(OperationSortKey(dates[0])) || !OperationSortKey(dates[0]).After(OperationSortKey(dates[1])) {
		t.Fatal("invalid dates must sort oldest")
	}
	if !OperationSortKey("2026-01-31T00:00:00").Equal(OperationSortKey(dates[0])) {
		t.Fatal("naive date isn't Moscow")
	}
	if !OperationSortKey("2026-01-31").Equal(time.Date(2026, 1, 31, 0, 0, 0, 0, domainMoscow)) {
		t.Fatal("date-only sort")
	}
	// Europe/Moscow has historical offsets; a constant +03 is not a timezone.
	p := mustDecodeDomain(t, `{"body":{"operations":[{"date":"01.07.2010T12:00:00"},{"date":"bad"},{"date":"13.07.2026T12:00:00.000"}]}}`)
	ops, err := ParseOperations(p)
	if err != nil || ops[0].Date != "2010-07-01T12:00:00+04:00" || ops[1].Date != "bad" || ops[2].Date != "13.07.2026T12:00:00.000" {
		t.Fatalf("source date fallback: %+v %v", ops, err)
	}
}

func TestPANUnicodeAndBoundaries(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Карта 4111 1111 1111 1111", "Карта •••• 1111"}, {"4111\u00a01111\u00a01111\u00a01111", "•••• 1111"},
		{"Карта ４１１１ １１１１ １１１１ １１１１", "Карта •••• １１１１"}, {"4111111111111112", "4111111111111112"},
		{"9411111111111111199", "9411111111111111199"}, {"4111111111111111 4111111111111111", "•••• 1111 •••• 1111"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := RedactPAN(tc.in); got != tc.want {
				t.Fatalf("redaction %q -> %q want %q", tc.in, got, tc.want)
			}
		})
	}
	if got := domainLast4("４１１１ １１１１ １１１１ １１１１"); got != "１１１１" {
		t.Fatalf("unicode last4: %q", got)
	}
}

func TestPFMCountRejectsNonNumericRatioAndTruncatesExactly(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int
	}{{json.Number("4.999999999999999999"), 4}, {json.Number("-4.999999999999999999"), -4}, {json.Number("1e20"), 0}, {[]any{1, 2}, 0}, {nil, 0}, {false, 0}, {json.Number("1/2"), 0}, {json.Number("1_0"), 0}} {
		t.Run(fmt.Sprint(tc.in), func(t *testing.T) {
			if got := domainInt(tc.in); got != tc.want {
				t.Fatalf("count %v -> %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestDomainParserBoundaryCases(t *testing.T) {
	if _, err := DecodeJSON(strings.NewReader("{")); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if domainID(false) != "False" || domainID(nil) != "" {
		t.Fatal("source ID coercion")
	}
	if domainLuhn("") || domainLuhn("123x") {
		t.Fatal("invalid Luhn accepted")
	}
	for _, v := range []any{nil, "", 0, []any{}, []string{}, map[string]any{}, uint64(0)} {
		if domainTruthy(v) {
			t.Fatalf("empty value truthy: %v", v)
		}
	}
	if !domainTruthy(struct{}{}) {
		t.Fatal("source object truthiness")
	}
	if domainLast4(nil) != "" {
		t.Fatal("absent last4")
	}
	if r := domainResource(map[string]any{"id": "untyped"}); r == nil || r.Type != "unknown" || r.ID != "untyped" {
		t.Fatal("unknown resource fallback")
	}
	if domainOperationDate("31.02.2026T00:00:00") != "31.02.2026T00:00:00" {
		t.Fatal("invalid date changed")
	}
	f, _ := NewTimeFilter("", "")
	a, b := f.RequestBounds()
	c, d := f.ISOBounds()
	if a+b+c+d != "" {
		t.Fatal("unbounded request fields populated")
	}
	for _, api := range []func(map[string]any) error{
		func(p map[string]any) error { _, err := ParseCardInfo(p); return err }, func(p map[string]any) error { _, err := ParsePFMAmounts(p); return err },
	} {
		if err := api(map[string]any{"sourceErrorResponse": map[string]any{}, "body": map[string]any{}}); err == nil {
			t.Fatal("source error accepted")
		}
	}
	for _, sec := range []string{`{"ctaccounts":{"data":[{"balance":{"amount":true}}]}}`, `{"cardsInWallet":{"data":[null]}}`} {
		p := mustDecodeDomain(t, `{"body":{"sections":{"technicalSection":{"sectionProductData":`+sec+`}}}}`)
		if _, err := ParseProducts(p); err == nil {
			t.Fatalf("bad product element %s", sec)
		}
	}
	// A unique current parent chooses total-limit even when missing; it must
	// never silently fall back to its own/available balance.
	p := mustDecodeDomain(t, `{"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":"A","number":"n","balance":{"amount":"100"}}]},"cardsInWallet":{"data":[{"id":"C","isCTA":true,"cardAccount":"n","availableLimit":{"amount":"50"}}]}}}}}}`)
	products, err := ParseProducts(p)
	if err != nil || len(products.Cards) != 1 || products.Cards[0].Balance != nil || products.Cards[0].BalanceSource != nil || products.Cards[0].AccountBalance.Amount.String() != "100" {
		t.Fatalf("balance fallback guessed: %+v %v", products, err)
	}
}

func FuzzDomainResponseDecoding(f *testing.F) {
	for _, s := range []string{
		`{}`, `{"body":{"operations":[]}}`, `{"body":{"operations":[{"operationAmount":{"amount":"1.00"}}]}}`, `{"body":{"sections":{"technicalSection":{"sectionProductData":{}}}}}`, `{"body":{"amounts":[]}}`,
		`{"body":{"operations":[{"uohId":"\ud800"}]}}`, `{"body":{"operations":[{"uohId":"\ud83d\ude00"}]}}`,
		`{"body":{"operations":[{"uohId":"known"}],"\u006fperations":[]}}`,
		`{"body":{"operations":[{"operationAmount":{"amount":true,"amount":"1.00"}}]}}`,
		`{"body":{"operations":[{"operationAmount":true}]}}`,
		`{"sourceErrorResponse":null}`, `{"error":{"code":"synthetic-error"}}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := DecodeJSON(strings.NewReader(s))
		if err != nil {
			return
		}
		_, _ = ParseProducts(p)
		_, _ = ParseOperations(p)
		_, _ = ParseOperationDetails(p)
		_, _ = ParseCardInfo(p)
		_, _ = ParsePFMAmounts(p)
	})
}

func TestOperationDateSourceStrptimeWidths(t *testing.T) {
	p := mustDecodeDomain(t, `{"body":{"operations":[{"date":"1.7.2010T1:2:3"},{"date":"31.2.2026T1:2:3"},{"date":"01.07.1900T12:00:00"}]}}`)
	ops, err := ParseOperations(p)
	if err != nil {
		t.Fatal(err)
	}
	if ops[0].Date != "2010-07-01T01:02:03+04:00" || ops[1].Date != "31.2.2026T1:2:3" || ops[2].Date != "1900-07-01T12:00:00+02:30:17" {
		t.Fatalf("strptime/timezone parity: %+v", ops)
	}
	if !OperationSortKey(ops[2].Date).Equal(time.Date(1900, 7, 1, 12, 0, 0, 0, domainMoscow)) {
		t.Fatal("historical offset seconds lost")
	}
}

func TestProductsGolden(t *testing.T) {
	r := readDomainGolden(t, "resources", "all_account_kinds_ambiguous_parent")
	p, err := DecodeJSON(strings.NewReader(string(r.Data.Input)))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseProducts(p)
	if err != nil {
		t.Fatal(err)
	}
	assertDomainJSON(t, got, r.Data.Expected)
}

// Tracer from har.py:self_test plus explicit invalid core-money contract.
func TestProductsBalanceSourceAndPAN(t *testing.T) {
	p, err := DecodeJSON(strings.NewReader(`{"body":{"sections":{"technicalSection":{"sectionProductData":{
 "ctaccounts":{"data":[{"id":10,"number":"•• 1111","balance":{"amount":"100.25","currency":{"code":"RUB"}}}]},
 "sharingCtAccounts":{"data":[]},"accounts":{"data":[{"id":11,"number":"•• 1111"}]},
 "cardsInWallet":{"data":[
  {"id":20,"name":"Карта 4111 1111 1111 1111","number":"1234 5678 9012 3456","availableLimit":{"amount":"90.25","currency":{"code":"RUB"}}},
  {"id":21,"number":"9999","isCTA":true,"cardAccount":"•• 1111","availableLimit":{"amount":"1.00"},"availableTotalLimit":{"amount":100.25,"currency":{"code":"RUB"}}}
 ]}}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseProducts(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Accounts) != 2 || got.Accounts[1].Kind != "account" || len(got.Cards) != 2 {
		t.Fatalf("products: %+v", got)
	}
	c := got.Cards[1]
	if c.AccountID == nil || *c.AccountID != "10" || c.BalanceSource == nil || *c.BalanceSource != "availableTotalLimit" || c.Balance.Amount.String() != "100.25" || c.AccountBalance.Amount.String() != "100.25" {
		t.Fatalf("parent: %+v", c)
	}
	if got.Cards[0].Last4 != "3456" || got.Cards[0].Name != "Карта •••• 1111" {
		t.Fatalf("PAN: %+v", got.Cards[0])
	}
	for _, sec := range []string{`{"ctaccounts":{"data":5}}`, `{"cardsInWallet":{"data":true}}`, `{"ctaccounts":{"data":[null]}}`, `{"cardsInWallet":{"data":[{"availableLimit":{"amount":true}}]}}`} {
		raw := mustDecodeDomain(t, `{"body":{"sections":{"technicalSection":{"sectionProductData":`+sec+`}}}}`)
		if _, err := ParseProducts(raw); err == nil {
			t.Fatalf("accepted %s", sec)
		}
	}
	for _, v := range []string{`[]`, `null`, `{} {}`, `{"amount":1}garbage`} {
		if _, err := DecodeJSON(strings.NewReader(v)); err == nil {
			t.Fatalf("bad envelope accepted: %s", v)
		}
	}
}
