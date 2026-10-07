package bank

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

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
