package sber

import (
	"context"
	"testing"
)

func TestClientDiscoveredAppOriginIsCachedRatherThanTreatedAsUnknown(t *testing.T) {
	b, _ := (SberCredentials{"session-fixture", "token-fixture"}).ToBundle(CredentialsBundleOptions{})
	tr := clientFake(t, b)
	tr.get = func(_ context.Context, u string, _ RequestOptions) (*Response, error) {
		if u == AppOrigin+"/app/main" {
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
