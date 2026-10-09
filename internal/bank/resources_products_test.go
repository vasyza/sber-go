package bank

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
)

func TestResourceAnalyticsAmountsExactDefaultFilter(t *testing.T) {
	response := resourceMap(`{"success":true,"body":{"amounts":[{"from":"2026-08-01T00:00:00+03:00","to":"2026-08-31T23:59:59+03:00","incomeType":"outcome","nationalAmount":{"amount":"50000.00","currency":"RUB"},"categoryAmounts":[{"id":1,"name":"Fixture category","externalId":"CAFE","visibleAmount":{"amount":"12000.00","currency":"RUB"},"countOperations":8}]}]}}`)
	expected := map[string]any{"filter": map[string]any{"from": "2026-08-01T00:00:00+03:00", "to": "2026-08-31T23:59:59+03:00", "incomeType": "outcome", "betweenOwnFilter": "on", "openBankingFilter": "off", "productFilters": []any{map[string]any{"type": "CARD", "filter": "custom"}, map[string]any{"type": "CT_ACCOUNT", "filter": "custom"}, map[string]any{"type": "MANUAL", "filter": "custom"}}}, "display": map[string]any{"showCategoryAmounts": true, "showProductAmounts": true}}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/pfpv_alf_mb/v1.00/alf/amounts", Payload: expected}, Response: response}}}
	got, err := NewAnalyticsAPI(r).Amounts(context.Background(), "2026-08-01", "2026-08-31")
	if err != nil || got.Periods[0].NationalAmount.Amount.String() != "50000.00" || got.Periods[0].Categories[0].CountOperations != 8 {
		t.Fatalf("analytics: %#v %v", got, err)
	}
	r.done()
}
func TestResourceAnalyticsFlagsAndAwareMoscowBounds(t *testing.T) {
	expected := map[string]any{"filter": map[string]any{"from": "2026-08-01T03:00:00+03:00", "to": "2026-08-02T04:00:00+03:00", "incomeType": "income", "betweenOwnFilter": "off", "openBankingFilter": "on", "productFilters": []any{map[string]any{"type": "CARD", "filter": "custom"}, map[string]any{"type": "CT_ACCOUNT", "filter": "custom"}, map[string]any{"type": "MANUAL", "filter": "custom"}}}, "display": map[string]any{"showCategoryAmounts": false, "showProductAmounts": false}}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/pfpv_alf_mb/v1.00/alf/amounts", Payload: expected}, Response: map[string]any{"success": true, "body": map[string]any{"amounts": []any{}}}}}}
	_, err := NewAnalyticsAPI(r).Amounts(context.Background(), "2026-08-01T00:00:00Z", "2026-08-02T01:00:00Z", AnalyticsOptions{IncomeType: "income", OpenBanking: true})
	if err != nil {
		t.Fatal(err)
	}
	r.done()
}
func TestResourceAnalyticsRejectsInvalidBoundsAndIncomeType(t *testing.T) {
	r := &resourceScript{t: t}
	a := NewAnalyticsAPI(r)
	for _, b := range [][2]string{{"", "2026-08-01"}, {"2026-08-01", ""}, {"2026-08-31", "2026-08-01"}, {"bad", "2026-08-01"}, {"0000-01-01", "2026-08-01"}} {
		if _, err := a.Amounts(context.Background(), b[0], b[1]); err == nil {
			t.Fatal("bad bounds accepted")
		}
	}
	if _, err := a.Amounts(context.Background(), "2026-08-01", "2026-08-31", AnalyticsOptions{IncomeType: "both"}); err == nil {
		t.Fatal("bad income type")
	}
	if len(r.calls) != 0 {
		t.Fatal("bad analytics requested")
	}
}

func resourceCardInfoResponse() map[string]any {
	return resourceMap(`{"success":true,"body":{"cardDetails":{"cards":[{"id":12345,"name":"Fixture card","number":"4111 1111 1111 1111","state":"ACTIVE","cardHolder":"FIXTURE HOLDER","paySystemType":"VISA","expireDate":"12/28","limits":{"purchaseLimit":{"amount":"300000.00","currency":{"code":"RUB"}},"availableLimit":{"amount":"84250.17","currency":{"code":"RUB"}},"availableTotalLimit":{"amount":"84250.17","currency":{"code":"RUB"}}},"creditType":{"creditLimit":{"amount":"120000.00","currency":{"code":"RUB"}},"creditDebt":{"amount":"15000.00","currency":{"code":"RUB"}},"creditMinPaymentDate":"2026-09-01"}}]}}}`)
}
func TestResourceCardsInfoCanonicalNumericIDsAndTypedFields(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/ufs-carddetail/rest/card/v1/cardInfo", Payload: map[string]any{"cardIds": []int64{12345, 12345, 9007199254740991}}}, Response: resourceCardInfoResponse()}}}
	got, err := NewCardsAPI(r).Info(context.Background(), 12345, "٠٠١٢٣٤٥", int64(9007199254740991))
	if err != nil || len(got) != 1 || got[0].Last4 != "1111" || got[0].Limits.Available.Amount.String() != "84250.17" || got[0].Credit.Debt.Amount.String() != "15000.00" {
		t.Fatalf("card info: %#v %v", got, err)
	}
	r.done()
}
func TestResourceCardsLimitsReturnsFirstOrUnknown(t *testing.T) {
	for _, empty := range []bool{false, true} {
		response := resourceCardInfoResponse()
		if empty {
			response = resourceMap(`{"success":true,"body":{"cardDetails":{"cards":[]}}}`)
		}
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/ufs-carddetail/rest/card/v1/cardInfo", Payload: map[string]any{"cardIds": []int64{12345}}}, Response: response}}}
		got, err := NewCardsAPI(r).Limits(context.Background(), 12345)
		if err != nil || empty != (got == nil) {
			t.Fatalf("limits: %#v %v", got, err)
		}
		if !empty && (got.AvailableTotal.Amount.String() != "84250.17" || got.Purchase.Amount.String() != "300000.00") {
			t.Fatal("limits fields lost")
		}
		r.done()
	}
}
func TestResourceCardsInfoRejectsMalformedIDBeforeCanonicalizing(t *testing.T) {
	r := &resourceScript{t: t}
	api := NewCardsAPI(r)
	invalid := []any{nil, true, false, 1.0, -1, 0, "", " 1", "1 ", "+1", "1.0", "１\n", "abc", strings.Repeat("0", 17) + "1", int64(9007199254740992), ^uint64(0), "²"}
	if _, err := api.Info(context.Background()); err == nil {
		t.Fatal("empty ID list")
	}
	for _, id := range invalid {
		if _, err := api.Info(context.Background(), id); err == nil {
			t.Fatalf("accepted ID %#v", id)
		}
	}
	if !reflect.DeepEqual(r.calls, []resourceCall(nil)) {
		t.Fatal("invalid ID sent")
	}
}

func TestResourceOperationsDetailsExactIDAndTypedResult(t *testing.T) {
	response := resourceMap(`{"success":true,"body":{"uohId":"op-uuid-1","header":{"title":"Оплата","operationAmount":{"amount":"-1234.56","currencyCode":"RUB"}},"state":{"category":"COMPLETED"},"statementAvailable":true,"fields":[{"name":"Сумма","type":"AMOUNT","value":{"amount":"-1234.56","currencyCode":"RUB"}}]}}`)
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/uoh-bh/v1/operation/details", Payload: map[string]any{"uohId": "op-uuid-1"}}, Response: response}}}
	got, err := NewOperationsAPI(r).Details(context.Background(), "op-uuid-1")
	if err != nil || got.UOHID != "op-uuid-1" || got.Amount.Amount.String() != "-1234.56" || !got.StatementAvailable {
		t.Fatalf("detail: %#v %v", got, err)
	}
	if _, ok := got.Fields[0].Value.(*Money); !ok {
		t.Fatal("lost money type")
	}
	r.done()
}
func TestResourceOperationsDetailsRejectsLiteralMalformedIDs(t *testing.T) {
	r := &resourceScript{t: t}
	api := NewOperationsAPI(r)
	for _, id := range []string{"", "has space", "trim ", "\nvalid", "a/b", "é", "a" + string(make([]byte, 128))} {
		if _, err := api.Details(context.Background(), id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	if !reflect.DeepEqual(r.calls, []resourceCall(nil)) {
		t.Fatal("validation sent request")
	}
}

func resourcePortfolioResponse() map[string]any {
	return resourceMap(`{"success":true,"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":1001,"name":"Fixture current","number":"unique","balance":{"amount":"10.50","currencyCode":"RUB"}}]},"accounts":{"data":[{"id":2002,"name":"Fixture savings"}]},"cardsInWallet":{"data":[{"id":4004,"name":"Fixture card","number":"0004","isCTA":true,"cardAccount":"unique","availableTotalLimit":{"amount":"2.50","currencyCode":"RUB"}}]}}}}}}`)
}
func resourceProductsCall(force bool) resourceCall {
	return resourceCall{Kind: "read", Path: "/main-screen/rest/v2/m1/web/section/meta", Payload: map[string]any{"withData": true, "forceUpdate": force}}
}
func TestResourceAggregatePortfolioSharesBoundAPIs(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}, {Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}, {Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}}}
	APIs := NewResources(r)
	p, err := APIs.Portfolio(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Cards()[0].bankBinding().operations != APIs.Operations || p.Cards()[0].bankBinding().cards != APIs.Cards || p.Cards()[0].bankBinding().transfers != APIs.Transfers {
		t.Fatal("snapshot actions not bound to shared APIs")
	}
	cards, err := APIs.Cards.List(context.Background(), true)
	if err != nil || cards[0].bankBinding().transfers != APIs.Transfers {
		t.Fatal("cards shortcut binding")
	}
	accounts, err := APIs.Accounts.List(context.Background(), false)
	if err != nil || accounts[0].bankBinding().transfers != APIs.Transfers {
		t.Fatal("account shortcut binding")
	}
	r.done()
}
func TestResourceCardsListReturnsOneBoundPortfolioSnapshot(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}}}
	got, err := NewCardsAPI(r).List(context.Background(), true)
	if err != nil || len(got) != 1 || got[0].Account() == nil || got[0].Account().ID() != "1001" || got[0].Account().Cards()[0] != got[0] {
		t.Fatal("cards list inconsistent snapshot")
	}
	r.done()
}
func TestResourceAccountsListReturnsOneBoundPortfolioSnapshot(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}}}
	got, err := NewAccountsAPI(r).List(context.Background(), false)
	if err != nil || len(got) != 2 || len(got[0].Cards()) != 1 || got[0].Cards()[0].Account() != got[0] || got[1].Kind() != "account" {
		t.Fatal("account list not from one snapshot")
	}
	r.done()
}

// resourceScript is an offline requester, not a bank/client-core substitute.
// All responses/identifiers in this file are explicitly synthetic.
type resourceCall struct {
	Kind, Path string
	Payload    map[string]any
	Query      map[string]string
	PageID     string
	Workflow   bool
}
type resourceStep struct {
	Call     resourceCall
	Response map[string]any
	Err      error
	Before   func(context.Context)
}
type resourceScript struct {
	t           *testing.T
	mu          sync.Mutex
	sequence    sync.Mutex
	steps       []resourceStep
	calls       []resourceCall
	warm        []bool
	bundle      sdkSession.SessionBundle
	credentials sdkSession.SberCredentials
	exportErr   error
	warmErr     error
	sequences   int
}

func resourceMap(text string) map[string]any {
	p, err := DecodeJSON(strings.NewReader(text))
	if err != nil {
		panic(err)
	}
	return p
}
func (r *resourceScript) send(ctx context.Context, c resourceCall) (map[string]any, error) {
	r.mu.Lock()
	r.calls = append(r.calls, c)
	if len(r.steps) == 0 {
		r.mu.Unlock()
		r.t.Errorf("unexpected synthetic call: %s %s", c.Kind, c.Path)
		return nil, errors.New("script exhausted")
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	r.mu.Unlock()
	if !reflect.DeepEqual(c, step.Call) {
		r.t.Errorf("exact request mismatch:\n got %#v\nwant %#v", c, step.Call)
	}
	if step.Before != nil {
		step.Before(ctx)
	}
	return step.Response, step.Err
}
func (r *resourceScript) PostRead(ctx context.Context, p string, b map[string]any) (map[string]any, error) {
	return r.send(ctx, resourceCall{Kind: "read", Path: p, Payload: b})
}
func (r *resourceScript) Mutate(ctx context.Context, p string, b map[string]any, q map[string]string, id string, w bool) (map[string]any, error) {
	r.sequence.Lock()
	defer r.sequence.Unlock()
	return r.send(ctx, resourceCall{Kind: "mutation", Path: p, Payload: b, Query: q, PageID: id, Workflow: w})
}
func (r *resourceScript) MutationSequence(ctx context.Context, f func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	r.sequence.Lock()
	defer r.sequence.Unlock()
	r.mu.Lock()
	r.sequences++
	r.mu.Unlock()
	active := true
	defer func() { active = false }()
	return f(func(ctx context.Context, p string, b map[string]any, q map[string]string, id string, w bool) (map[string]any, error) {
		if !active {
			return nil, errors.New("sequence ended")
		}
		return r.send(ctx, resourceCall{Kind: "sequence", Path: p, Payload: b, Query: q, PageID: id, Workflow: w})
	})
}
func (r *resourceScript) ExportSession() (sdkSession.SessionBundle, error) {
	return r.bundle, r.exportErr
}
func (r *resourceScript) ExportCredentials() (sdkSession.SberCredentials, error) {
	return r.credentials, r.exportErr
}
func (r *resourceScript) WarmUp(ctx context.Context, force bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warm = append(r.warm, force)
	return r.warmErr
}
func (r *resourceScript) done() {
	r.t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.steps) != 0 {
		r.t.Fatalf("unconsumed synthetic steps: %d", len(r.steps))
	}
}

func TestResourceProductsGetExactBooleanPayload(t *testing.T) {
	response := resourceMap(`{"success":true,"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":1001,"name":"Fixture current","number":"0001","balance":{"amount":"10.50","currencyCode":"RUB"}}]},"accounts":{"data":[{"id":2002,"name":"Fixture saving"}]},"cardsInWallet":{"data":[{"id":4004,"name":"Fixture card","number":"0004","availableLimit":{"amount":"2.50","currencyCode":"RUB"}}]}}}}}}`)
	for _, force := range []bool{false, true} {
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/main-screen/rest/v2/m1/web/section/meta", Payload: map[string]any{"withData": true, "forceUpdate": force}}, Response: response}}}
		got, err := NewProductsAPI(r).Get(context.Background(), force)
		if err != nil || len(got.Accounts) != 2 || len(got.Cards) != 1 || got.Accounts[0].Balance.Amount.String() != "10.50" || got.Accounts[1].Kind != "account" {
			t.Fatalf("typed products: %#v %v", got, err)
		}
		r.done()
	}
}

func resourceRenameCall(kind string, id int64, name string) resourceCall {
	return resourceCall{Kind: kind, Path: "/ufs-productdetail/rest/v1/changeProductName", Payload: map[string]any{"id": id, "name": name, "type": "card"}, Query: map[string]string{}, PageID: "/app/cards/details/12345"}
}
func TestResourceRenameDefaultDisabledBeforeAnyMutation(t *testing.T) {
	r := &resourceScript{t: t}
	err := NewCardsAPI(r).Rename(context.Background(), 12345, "Valid")
	if !errors.Is(err, ErrResourceMutationsDisabled) || len(r.calls) != 0 {
		t.Fatalf("default writes enabled: %v", err)
	}
}
func TestResourceRenameExactPayloadAndValidation(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Новая карта, 1.-"), Response: map[string]any{"success": true}}}}
	api := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
	for _, name := range []string{"", "   ", strings.Repeat("a", 57), "slash/name", "underscore_name", "bad\nname", "é", "\x7f"} {
		if err := api.Rename(context.Background(), 12345, name); err == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	if err := api.Rename(context.Background(), "0012345", "Новая карта, 1.-"); err != nil {
		t.Fatal(err)
	}
	r.done()
}
func TestResourceRenamePreservesDefiniteRejection(t *testing.T) {
	rejected := &sdkErrs.APIRejected{Code: "5", Title: "Fixture", Text: "Fixture text", UUID: "fixture", System: "fixture"}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Valid"), Err: rejected}}}
	err := NewCardsAPI(r, ResourceOptions{AllowMutations: true}).Rename(context.Background(), 12345, "Valid")
	if err != rejected {
		t.Fatalf("lost definite rejection %v", err)
	}
	r.done()
}
func TestResourceRenameUncertaintyAndCancellationNeverReplay(t *testing.T) {
	for _, cause := range []error{&sdkErrs.APIError{}, &sdkErrs.AuthenticationExpired{}, &sdkErrs.TransportError{}, context.Canceled, context.DeadlineExceeded} {
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Valid"), Err: cause}}}
		api := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
		var uncertain *sdkErrs.MutationUncertain
		if err := api.Rename(context.Background(), 12345, "Valid"); !errors.As(err, &uncertain) {
			t.Fatalf("not uncertain: %v", err)
		}
		if err := api.Rename(context.Background(), 12345, "Another"); !errors.As(err, &uncertain) || len(r.calls) != 1 {
			t.Fatal("repeated uncertain rename")
		}
		r.done()
	}
}

func resourceSession(t *testing.T) (sdkSession.SessionBundle, sdkSession.SberCredentials) {
	t.Helper()
	c, err := sdkSession.NewSberCredentials("fixture-session", "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.ToBundle(sdkSession.CredentialsBundleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return b, c
}
func TestResourceSessionExportMemoryAndExplicitSyntheticPath(t *testing.T) {
	b, c := resourceSession(t)
	r := &resourceScript{t: t, bundle: b, credentials: c}
	a := NewSessionAPI(r)
	got, err := a.Export()
	if err != nil || !reflect.DeepEqual(got, b) {
		t.Fatal("export lost session")
	}
	got.Cookies[0].Value = "changed"
	if r.bundle.Cookies[0].Value == "changed" {
		t.Fatal("export aliases requester")
	}
	path := filepath.Join(testPrivateDir(t), "synthetic-session.json")
	_, err = a.Export(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := sdkSession.LoadSessionBundle(path)
	if err != nil || !reflect.DeepEqual(restored, b) {
		t.Fatal("explicit save not exercised")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("not private")
	}
	failure := errors.New("fixture export failure")
	r.exportErr = failure
	if _, err = a.Export(); err != failure {
		t.Fatal("export error lost")
	}
	if len(r.calls) != 0 {
		t.Fatal("export sent mutation")
	}
}
func TestResourceSessionCredentialsExplicitRotatedValues(t *testing.T) {
	b, c := resourceSession(t)
	r := &resourceScript{t: t, bundle: b, credentials: c}
	a := NewSessionAPI(r)
	got, err := a.Credentials()
	if err != nil || got != c {
		t.Fatal("credentials lost")
	}
	r.credentials, _ = sdkSession.NewSberCredentials("fixture-rotated-session", "fixture-rotated-token")
	got, err = a.Credentials()
	if err != nil || got != r.credentials {
		t.Fatal("stale credentials")
	}
	failure := errors.New("fixture credentials failure")
	r.exportErr = failure
	if _, err = a.Credentials(); err != failure {
		t.Fatal("credentials error lost")
	}
	if len(r.calls) != 0 {
		t.Fatal("credentials mutated")
	}
}
func TestResourceSessionWarmUpExplicitForceAndDefault(t *testing.T) {
	r := &resourceScript{t: t}
	a := NewSessionAPI(r)
	if err := a.WarmUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.WarmUp(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.warm, []bool{true, false}) || len(r.calls) != 0 {
		t.Fatal("wrong session dispatch")
	}
	failure := errors.New("fixture warm failure")
	r.warmErr = failure
	if err := a.WarmUp(context.Background(), true); err != failure {
		t.Fatal("warm error lost")
	}
}
