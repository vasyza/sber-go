package bank

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientBindingAccountsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	api := clientBindingGetter[*AccountsAPI](t, c, "Accounts")
	if api != clientBindingGetter[*AccountsAPI](t, c, "Accounts") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	accounts, err := api.List(context.Background(), true)
	if err != nil || len(accounts) != 2 || accounts[0].Cards()[0].Account() != accounts[0] {
		t.Fatalf("bound accounts: %v", err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+ProductsPath || !reflect.DeepEqual(calls[0].body, resourceProductsCall(true).Payload) {
		t.Fatal("accounts shortcut changed source product read")
	}
}

func TestClientBindingAnalyticsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourceMap(`{"success":true,"body":{"amounts":[{"nationalAmount":{"amount":"50000.00","currency":"RUB"}}]}}`))
	api := clientBindingGetter[*AnalyticsAPI](t, c, "Analytics")
	if api != clientBindingGetter[*AnalyticsAPI](t, c, "Analytics") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	amounts, err := api.Amounts(context.Background(), "2026-08-01", "2026-08-31")
	if err != nil || len(amounts.Periods) != 1 || amounts.Periods[0].NationalAmount.Amount.String() != "50000.00" {
		t.Fatalf("native analytics: %v", err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+PFMAmountsPath {
		t.Fatal("analytics requester changed")
	}
	filter := calls[0].body["filter"].(map[string]any)
	if filter["from"] != "2026-08-01T00:00:00+03:00" || filter["to"] != "2026-08-31T23:59:59+03:00" || filter["betweenOwnFilter"] != "on" {
		t.Fatal("analytics source filter changed")
	}
}

func TestClientBindingCardsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourceCardInfoResponse())
	api := clientBindingGetter[*CardsAPI](t, c, "Cards")
	if api != clientBindingGetter[*CardsAPI](t, c, "Cards") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	cards, err := api.Info(context.Background(), "12345")
	if err != nil || len(cards) != 1 || cards[0].Limits.Available.Amount.String() != "84250.17" {
		t.Fatalf("native card info: %v", err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+CardInfoPath || !reflect.DeepEqual(calls[0].body, map[string]any{"cardIds": []int64{12345}}) {
		t.Fatal("card info source request changed")
	}
}

func TestClientBindingOperationsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourceOperationsResponse("native-op"))
	api := clientBindingGetter[*OperationsAPI](t, c, "Operations")
	if api != clientBindingGetter[*OperationsAPI](t, c, "Operations") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	page, err := api.Page(context.Background(), OperationsPageOptions{Limit: 2, Resource: "card:4004", From: "2026-07-01", To: "2026-07-31"})
	if err != nil || len(page.Operations) != 1 || page.Operations[0].ID != "native-op" || page.Operations[0].ScopeCardIDs[0] != "4004" {
		t.Fatalf("typed operations: %v", err)
	}
	calls := tr.snapshotCalls()
	want := resourceHistoryCall(0, 3, "card:4004", "01.07.2026T00:00:00", "31.07.2026T23:59:59")
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+OperationsPath || !reflect.DeepEqual(calls[0].body, want.Payload) {
		t.Fatal("source operation request changed")
	}
}

type clientBindingPortfolioAPI interface {
	Portfolio(context.Context, bool) (*BankPortfolio, error)
}

func clientBindingPortfolio(t *testing.T, c *SberClient, force bool) *BankPortfolio {
	t.Helper()
	api, ok := any(c).(clientBindingPortfolioAPI)
	if !ok {
		t.Fatal("missing concrete SberClient.Portfolio(ctx, bool) (*BankPortfolio, error)")
	}
	p, err := api.Portfolio(context.Background(), force)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func clientBindingCheckSnapshot(t *testing.T, c *SberClient, p *BankPortfolio) {
	t.Helper()
	accounts, cards := p.Accounts(), p.Cards()
	if len(accounts) != 2 || len(cards) != 1 || cards[0].Account() != accounts[0] || accounts[0].Cards()[0] != cards[0] || len(accounts[1].Cards()) != 0 {
		t.Fatal("product identity relationships lost")
	}
	binding := cards[0].bankBinding()
	if binding != accounts[0].bankBinding() || !clientBindingOwnsRequester(c, binding.requester) || binding.operations != c.Operations() || binding.cards != c.Cards() || binding.transfers != c.Transfers() || p.Transfers() != c.Transfers() {
		t.Fatal("snapshot did not retain client's shared native APIs/issuer")
	}
}

func TestClientBindingPortfolioOneReadAndSharedSnapshotLifetime(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	first := clientBindingPortfolio(t, c, false)
	second := clientBindingPortfolio(t, c, true)
	clientBindingCheckSnapshot(t, c, first)
	clientBindingCheckSnapshot(t, c, second)
	if first == second || first.Cards()[0] == second.Cards()[0] || first.Accounts()[0] == second.Accounts()[0] {
		t.Fatal("reads reused a financial snapshot")
	}
	cards, err := c.Cards().List(context.Background(), false)
	if err != nil || cards[0].bankBinding() != first.Cards()[0].bankBinding() || cards[0].Account().Cards()[0] != cards[0] {
		t.Fatal("cards shortcut created a new binding/issuer")
	}
	accounts, err := c.Accounts().List(context.Background(), true)
	if err != nil || accounts[0].bankBinding() != first.Accounts()[0].bankBinding() || accounts[0].Cards()[0].Account() != accounts[0] {
		t.Fatal("accounts shortcut created a new binding/issuer")
	}
	calls := tr.snapshotCalls()
	if len(calls) != 4 {
		t.Fatal("not exactly one products read per snapshot")
	}
	for i, force := range []bool{false, true, false, true} {
		if calls[i].target != c.core().bundle.APIBase+ProductsPath || !reflect.DeepEqual(calls[i].body, resourceProductsCall(force).Payload) {
			t.Fatal("source forceUpdate/withData request changed")
		}
	}

	// All mutable wrappers/slices/raw copies are detached, while the relation
	// identity within one snapshot remains stable.
	raw := first.Raw()
	raw.Accounts[0].Balance.Amount = resourceAmount(t, "999.99")
	*raw.Cards[0].AccountID = "changed"
	raw.Cards[0].Balance.Amount = resourceAmount(t, "0.01")
	cardCopy := first.Cards()[0].Snapshot()
	*cardCopy.AccountID = "different"
	cardCopy.Balance.Amount = resourceAmount(t, "0.02")
	first.Accounts()[0].Balance().Amount = resourceAmount(t, "0.03")
	*first.Cards()[0].AccountID() = "caller-copy"
	cardSlice := first.Cards()
	cardSlice[0] = nil
	for _, p := range []*BankPortfolio{first, second} {
		clientBindingCheckSnapshot(t, c, p)
		if p.Raw().Accounts[0].Balance.Amount.String() != "10.50" || p.Cards()[0].Balance().Amount.String() != "2.50" || *p.Cards()[0].AccountID() != "1001" {
			t.Fatal("raw financial snapshot aliased caller mutation")
		}
	}
}

func TestClientBindingSessionFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, nil)
	api := clientBindingGetter[*SessionAPI](t, c, "Session")
	if api != clientBindingGetter[*SessionAPI](t, c, "Session") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	exported, err := api.Export()
	if err != nil || exported.APIBase != c.core().bundle.APIBase {
		t.Fatalf("session export: %v", err)
	}
	exported.Cookies[0].Value = "caller-edit"
	creds, err := api.Credentials()
	if err != nil || creds.UFSSession != "session-binding-access" {
		t.Fatalf("session credentials snapshot: %v", err)
	}
	if n, _, _ := tr.counts(); n != 0 {
		t.Fatal("in-memory export made a request")
	}
	if err := api.WarmUp(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.WebBase+clientWarmUpPath {
		t.Fatal("warmup did not use owned transport")
	}
}

// Runtime lookup lets each absent convenience method fail as a behavior test,
// not a compiler error; the returned APIs are the actual native types.
func clientBindingGetter[T any](t *testing.T, c *SberClient, name string) T {
	t.Helper()
	method := reflect.ValueOf(c).MethodByName(name)
	if !method.IsValid() {
		t.Fatalf("missing concrete SberClient.%s convenience API", name)
	}
	if method.Type().NumIn() != 0 || method.Type().NumOut() != 1 {
		t.Fatalf("wrong %s getter signature", name)
	}
	api, ok := method.Call(nil)[0].Interface().(T)
	if !ok || reflect.ValueOf(api).IsNil() {
		t.Fatalf("%s is not a nonnil native API", name)
	}
	return api
}

func clientBindingOwnsRequester(c *SberClient, requester BusinessRequester) bool {
	owner, ok := requester.(*clientResourceRequester)
	return ok && owner.client != nil && *owner.client == c && requester == c.Products().requester
}

func clientBindingResponse(t *testing.T, payload map[string]any) *sdkTransport.Response {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return clientResponse(200, string(raw))
}

func clientBindingSyntheticClient(t *testing.T, payload map[string]any) (*SberClient, *clientFakeTransport) {
	t.Helper()
	bundle := clientFixture(t, "binding-access")
	tr := clientFake(t, bundle)
	if payload != nil {
		tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
			return clientBindingResponse(t, payload), nil
		}
	}
	c, err := NewSberClient(bundle, ClientOptions{Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c, tr
}

func TestClientBindingProductsUsesOwnedRequester(t *testing.T) {
	bundle := clientFixture(t, "binding-products")
	tr := clientFake(t, bundle)
	tr.post = func(_ context.Context, u string, body map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if u != bundle.APIBase+ProductsPath || !reflect.DeepEqual(body, map[string]any{"withData": true, "forceUpdate": true}) {
			t.Fatal("source products request changed")
		}
		return clientBindingResponse(t, resourcePortfolioResponse()), nil
	}
	c, err := NewSberClient(bundle, ClientOptions{Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	api := clientBindingGetter[*ProductsAPI](t, c, "Products")
	if api != clientBindingGetter[*ProductsAPI](t, c, "Products") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("products getter did not retain one actual client requester/API")
	}
	if requests, _, _ := tr.counts(); requests != 0 {
		t.Fatal("API construction made a request")
	}
	products, err := api.Get(context.Background(), true)
	if err != nil || len(products.Accounts) != 2 || len(products.Cards) != 1 || products.Accounts[0].Balance.Amount.String() != "10.50" {
		t.Fatalf("typed products unavailable: %v", err)
	}
	if requests, _, _ := tr.counts(); requests != 1 {
		t.Fatal("products read created extra traffic")
	}
}
