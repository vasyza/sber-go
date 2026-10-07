package bank

import (
	"context"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientDiscoveredAppOriginIsCachedRatherThanTreatedAsUnknown(t *testing.T) {
	b, _ := (sdkSession.SberCredentials{UFSSession: "session-fixture", UFSToken: "token-fixture"}).ToBundle(sdkSession.CredentialsBundleOptions{})
	tr := clientFake(t, b)
	tr.get = func(_ context.Context, u string, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if u == sdkSession.AppOrigin+"/app/main" {
			return clientResponse(200, `{"ufsHost":"https://web2.online.sberbank.ru"}`), nil
		}
		return clientResponse(200, `{"ufs.block.root.url":"https://online.sberbank.ru"}`), nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for i := 0; i < 3; i++ {
		if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e != nil {
			t.Fatal(e)
		}
	}
	gets, posts := 0, 0
	for _, call := range tr.snapshotCalls() {
		if call.method == "GET" {
			gets++
		} else {
			posts++
		}
	}
	if gets != 2 || posts != 3 {
		t.Fatalf("discovered valid app origin wasn't cached: GET=%d POST=%d", gets, posts)
	}
}
