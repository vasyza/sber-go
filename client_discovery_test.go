package sber

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestClientDiscoveryCoalescesAndCommitsBothHosts(t *testing.T) {
	b, e := (SberCredentials{"session-fixture", "token-fixture"}).ToBundle(CredentialsBundleOptions{})
	if e != nil {
		t.Fatal(e)
	}
	tr := clientFake(t, b)
	entered := make(chan struct{})
	release := make(chan struct{})
	tr.get = func(ctx context.Context, target string, o RequestOptions) (*Response, error) {
		if o.AcceptEncoding != "gzip, deflate, br, zstd" {
			t.Error("missing encodings")
		}
		switch target {
		case AppOrigin + "/app/main":
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return clientResponse(200, `startup({"ufsHost":"https://web2.online.sberbank.ru"})`), nil
		case "https://web2.online.sberbank.ru/main":
			return clientResponse(200, `{"ufs.block.root.url":"https://web-node2.online.sberbank.ru","ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), nil
		}
		t.Error("unexpected discovery URL")
		return nil, &APIError{}
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	results := make(chan error, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := c.PostRead(context.Background(), "/main-screen/rest/v2/m1/web/section/meta", nil)
			results <- e
		}()
	}
	select {
	case <-entered:
	case e := <-results:
		t.Fatalf("read completed without required discovery: %v", e)
	}
	before, e := c.ExportSession()
	if e != nil || before.APIBase != AppOrigin || before.WebBase != AppOrigin {
		t.Fatal("partly adopted discovery")
	}
	close(release)
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	after, e := c.ExportSession()
	if e != nil || after.WebBase != "https://web2.online.sberbank.ru" || after.APIBase != "https://web-node2.online.sberbank.ru" {
		t.Fatal("discovery not committed")
	}
	calls := tr.snapshotCalls()
	gets, posts := 0, 0
	for _, call := range calls {
		if call.method == "GET" {
			gets++
		} else {
			posts++
			if call.target != after.APIBase+"/main-screen/rest/v2/m1/web/section/meta" {
				t.Fatal("wrong discovered target or warmup")
			}
		}
	}
	if gets != 2 || posts != 20 {
		t.Fatalf("discovery not coalesced: %d %d", gets, posts)
	}
}
func TestClientDiscoveryRejectsUntrustedDocumentsWithoutPartialCommit(t *testing.T) {
	for _, tc := range []struct {
		name, shell, main, kind string
		status                  int
	}{
		{"ambiguous-shell", `{"ufsHost":"https://web1.online.sberbank.ru","ufsHost":"https://web2.online.sberbank.ru"}`, ``, "api", 200},
		{"external-shell", `{"ufsHost":"https://example.invalid"}`, ``, "api", 200},
		{"unsafe-main", `{"ufsHost":"https://web2.online.sberbank.ru"}`, `{"ufs.block.root.url":"https://web-node2.online.sberbank.ru/?ticket=secret"}`, "api", 200},
		{"invalid-main", `{"ufsHost":"https://web2.online.sberbank.ru"}`, `{"ufs.block.root.url":1}`, "api", 200},
		{"redirect", ``, ``, "expired", 302}, {"401", ``, ``, "expired", 401}, {"403", ``, ``, "expired", 403}, {"server", ``, ``, "api", 503},
		{"invalid-text", string([]byte{255}), ``, "api", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := (SberCredentials{"session-fixture", "token-fixture"}).ToBundle(CredentialsBundleOptions{})
			tr := clientFake(t, b)
			tr.get = func(_ context.Context, u string, _ RequestOptions) (*Response, error) {
				if u == AppOrigin+"/app/main" {
					return clientResponse(tc.status, tc.shell), nil
				}
				return clientResponse(tc.status, tc.main), nil
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil)
			var api *APIError
			var expired *AuthenticationExpired
			if tc.kind == "api" && !errors.As(e, &api) || tc.kind == "expired" && !errors.As(e, &expired) {
				t.Fatalf("wrong discovery failure: %v", e)
			}
			after, e := c.ExportSession()
			if e != nil || after.APIBase != AppOrigin || after.WebBase != AppOrigin {
				t.Fatal("partial discovery commit")
			}
			for _, call := range tr.snapshotCalls() {
				if call.method != "GET" {
					t.Fatal("read sent before safe discovery")
				}
			}
		})
	}
}
