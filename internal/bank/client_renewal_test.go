package bank

import (
	"bytes"
	"context"
	"errors"
	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestClientDiscoveryCoalescesAndCommitsBothHosts(t *testing.T) {
	b, e := (sdkSession.SberCredentials{UFSSession: "session-fixture", UFSToken: "token-fixture"}).ToBundle(sdkSession.CredentialsBundleOptions{})
	if e != nil {
		t.Fatal(e)
	}
	tr := clientFake(t, b)
	entered := make(chan struct{})
	release := make(chan struct{})
	tr.get = func(ctx context.Context, target string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if o.AcceptEncoding != "gzip, deflate, br, zstd" {
			t.Error("missing encodings")
		}
		switch target {
		case sdkSession.AppOrigin + "/app/main":
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
		return nil, &sdkErrs.APIError{}
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
	if e != nil || before.APIBase != sdkSession.AppOrigin || before.WebBase != sdkSession.AppOrigin {
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
			b, _ := (sdkSession.SberCredentials{UFSSession: "session-fixture", UFSToken: "token-fixture"}).ToBundle(sdkSession.CredentialsBundleOptions{})
			tr := clientFake(t, b)
			tr.get = func(_ context.Context, u string, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				if u == sdkSession.AppOrigin+"/app/main" {
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
			var api *sdkErrs.APIError
			var expired *sdkErrs.AuthenticationExpired
			if tc.kind == "api" && !errors.As(e, &api) || tc.kind == "expired" && !errors.As(e, &expired) {
				t.Fatalf("wrong discovery failure: %v", e)
			}
			after, e := c.ExportSession()
			if e != nil || after.APIBase != sdkSession.AppOrigin || after.WebBase != sdkSession.AppOrigin {
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

func TestClientPersistsOnlySuccessfulRotationAndPreservesProfileScope(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "minimum", true: "full"}[full], func(t *testing.T) {
			b := clientFixture(t, "old")
			b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
			if full {
				b.Cookies = append(b.Cookies, sdkSession.CookieRecord{Name: "remembered", Value: "synthetic-remembered", Domain: sdkSession.AuthCookieDomain, Path: "/CSAFront", Secure: true, HTTPOnly: true, HostOnly: true, SameSite: sdkTransport.PtrString("Strict")})
			}
			path := clientPrivatePath(t)
			if e := b.Save(path); e != nil {
				t.Fatal(e)
			}
			tr := clientFake(t, b)
			mode := "success"
			tr.post = func(_ context.Context, _ string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				u := mustClientURL(t, sdkSession.AppOrigin+"/")
				if e := tr.jar.ApplySetCookie(u, []string{"UFS-SESSION=session-" + mode + "; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=Lax", "UFS-TOKEN=token-" + mode + "; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly", "protection=synthetic-protection; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=None"}); e != nil {
					t.Fatal(e)
				}
				switch mode {
				case "expired":
					return clientResponse(401, `{}`), nil
				case "rejected":
					return clientResponse(200, `{"success":false}`), nil
				case "network":
					return nil, errors.New("raw sensitive URL must not escape")
				}
				return clientResponse(200, `{"success":true}`), nil
			}
			c, e := NewSberClientFromSessionFile(path, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil); e != nil {
				t.Fatal(e)
			}
			saved, e := sdkSession.LoadSessionBundle(path)
			if e != nil {
				t.Fatal(e)
			}
			creds, e := sdkSession.CredentialsFromBundle(saved)
			if e != nil || creds.UFSSession != "session-success" {
				t.Fatal("rotation was not saved")
			}
			expected := 2
			if full {
				expected = 4
			}
			if len(saved.Cookies) != expected || *saved.Deviceprint != *b.Deviceprint || *saved.AntifraudDeviceprint != *b.AntifraudDeviceprint {
				t.Fatal("wrong persistence scope")
			}
			if full && saved.Cookies[2].Path != "/CSAFront" {
				t.Fatal("remembered identity lost")
			}
			info, e := os.Stat(path)
			if e != nil || info.Mode().Perm() != 0600 {
				t.Fatal("profile not private")
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			for _, next := range []string{"expired", "rejected", "network"} {
				mode = next
				if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil); e == nil {
					t.Fatal("failed response accepted")
				}
				after, e := os.ReadFile(path)
				if e != nil || !bytes.Equal(before, after) {
					t.Fatal("failed response persisted")
				}
			}
			// The live rotating jar is still available explicitly, including extra cookies
			// that credential-only persistence deliberately excludes.
			exported, e := c.ExportSession()
			if e != nil || len(exported.Cookies) < 3 {
				t.Fatal("live cookies lost")
			}
		})
	}
}
func TestClientReadPersistenceFailureIsReturnedWithoutReplay(t *testing.T) {
	b := clientFixture(t, "persist")
	tr := clientFake(t, b)
	path := clientPrivatePath(t)
	if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, SessionPath: path})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/pfpv_alf_mb/v1.00/alf/amounts", nil)
	var insecure *sdkErrs.InsecureSessionFile
	if !errors.As(e, &insecure) {
		t.Fatal("persistence failure hidden")
	}
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatal("business read replayed for persistence")
	}
}

func TestClientRefreshFactoryReuseCannotRetireCommittedTransport(t *testing.T) {
	b := clientFixture(t, "retained")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	fresh := clientFixture(t, "replacement")
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return old, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var transport *sdkErrs.TransportError
	if !errors.As(e, &transport) || transport.Code != "reused_transport" {
		t.Fatal("factory reuse committed")
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid replacement profile published")
	}
	exported, e := c.ExportSession()
	if e != nil || exported.APIBase != b.APIBase {
		t.Fatal("committed state changed on failed refresh")
	}
	calls, closes, _ := old.counts()
	if calls != 1 || closes != 0 {
		t.Fatal("old generation torn down after failed refresh")
	}
}

func TestClientFailedRefreshLeavesOldStateAndCleanupWaitsForClose(t *testing.T) {
	for _, mode := range []string{"provider", "renewal", "invalid-bundle", "factory", "factory-with-owned", "nil-jar", "save"} {
		t.Run(mode, func(t *testing.T) {
			b := clientFixture(t, "failure-old")
			path := clientPrivatePath(t)
			if e := b.Save(path); e != nil {
				t.Fatal(e)
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			old := clientFake(t, b)
			old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(401, `{}`), nil
			}
			fresh := clientFixture(t, "failure-new")
			candidate := clientFake(t, fresh)
			var closeAttempts atomic.Int32
			candidate.close = func() error {
				if closeAttempts.Add(1) == 1 {
					return errors.New("synthetic close failure")
				}
				return nil
			}
			c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) {
				if mode == "provider" {
					return "", errors.New("synthetic provider failure")
				}
				return "13579", nil
			}, ClientOptions{Transport: old,
				Renewal: func(ctx context.Context, _ sdkSession.SessionBundle, p sdkAuth.PINProvider, _ sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
					if _, e := p(ctx); e != nil {
						return sdkSession.SessionBundle{}, e
					}
					switch mode {
					case "renewal":
						return sdkSession.SessionBundle{}, &sdkErrs.PinAuthError{Code: "fixture_failure"}
					case "invalid-bundle":
						fresh.APIBase = "https://external.invalid"
					case "save":
						if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
							t.Fatal(e)
						}
					}
					return fresh, nil
				},
				TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					switch mode {
					case "factory":
						return nil, errors.New("synthetic factory failure")
					case "factory-with-owned":
						return candidate, errors.New("synthetic factory failure")
					case "nil-jar":
						candidate.jar = nil
					}
					return candidate, nil
				},
			})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e == nil {
				t.Fatal("failed renewal accepted")
			}
			after, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("failed refresh wrote profile")
			}
			exported, e := c.ExportSession()
			if e != nil || exported.APIBase != b.APIBase {
				t.Fatal("failed refresh changed state")
			}
			_, oldCloses, _ := old.counts()
			if oldCloses != 0 {
				t.Fatal("old transport closed before commit")
			}
			expected := 0
			if mode == "factory-with-owned" || mode == "nil-jar" || mode == "save" {
				expected = 1
			}
			if int(closeAttempts.Load()) != expected {
				t.Fatal("unexpected candidate cleanup")
			}
			if e = c.Close(); e != nil {
				t.Fatal(e)
			}
			if int(closeAttempts.Load()) != expected*2 {
				t.Fatal("failed cleanup not retried exclusively by Close")
			}
		})
	}
}

// Done is observed at the gate's select, after arrival epoch capture. This makes
// contention tests deterministic without sleeps or scheduler assumptions.
type clientQueuedContext struct {
	context.Context
	once   sync.Once
	queued chan struct{}
}

func (c *clientQueuedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.queued) })
	return c.Context.Done()
}
func clientQueue() (*clientQueuedContext, <-chan struct{}) {
	ch := make(chan struct{})
	return &clientQueuedContext{Context: context.Background(), queued: ch}, ch
}

func TestClientPINReadRenewsOncePersistsBeforeRetryAndOwnsBothTransports(t *testing.T) {
	b := clientFixture(t, "old")
	b.Cookies = append(b.Cookies, sdkSession.CookieRecord{Name: "remembered", Value: "fixture", Domain: sdkSession.AuthCookieDomain, Path: "/CSAFront", Secure: true, HostOnly: true})
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	fresh := clientFixture(t, "new")
	fresh.Cookies = append(fresh.Cookies, b.Cookies[2])
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var builds, pins, renewals atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	proxy := sdkTransport.ProxyOptions{URL: "socks5://127.0.0.1:1080", Username: "synthetic-user", Password: "synthetic-password"}
	o := ClientOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: "synthetic-CA", Proxy: proxy}, BrowserBootstrapTimeout: 7 * time.Second,
		TransportFactory: func(got sdkSession.SessionBundle, to sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			if to.CABundle != "synthetic-CA" || to.Proxy != proxy {
				t.Error("CA or proxy not forwarded")
			}
			if builds.Add(1) == 1 {
				return old, nil
			}
			return next, nil
		},
		Renewal: func(ctx context.Context, got sdkSession.SessionBundle, provider sdkAuth.PINProvider, ao sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
			if ao.TransportOptions.CABundle != "synthetic-CA" || ao.TransportOptions.Proxy != proxy || ao.BrowserBootstrapTimeout != 7*time.Second {
				t.Error("auth options not forwarded")
			}
			if len(got.Cookies) != 3 || got.Cookies[2].Path != "/CSAFront" {
				t.Error("remembered profile not renewed")
			}
			pin, e := provider(ctx)
			if e != nil || pin != "13579" {
				t.Error("provider not used")
			}
			renewals.Add(1)
			close(started)
			<-release
			return fresh, nil
		}}
	next.post = func(_ context.Context, u string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		saved, e := sdkSession.LoadSessionBundle(path)
		if e != nil || saved.APIBase != fresh.APIBase {
			t.Error("retry before profile publication")
		}
		if u != fresh.APIBase+"/uoh-bh/v1/operations/list" {
			t.Error("stale business host")
		}
		return clientResponse(200, `{"success":true}`), nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { pins.Add(1); return "13579", nil }, o)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	results := make(chan error, 13)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); results <- e }()
	select {
	case <-started:
	case e := <-results:
		t.Fatalf("not renewed: %v", e)
	}
	for i := 0; i < 12; i++ {
		ctx, queued := clientQueue()
		go func() { _, e := c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); results <- e }()
		<-queued
	}
	close(release)
	for i := 0; i < 13; i++ {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	if pins.Load() != 1 || renewals.Load() != 1 || builds.Load() != 2 {
		t.Fatal("duplicate credential/login attempt")
	}
	oldCalls, oldCloses, _ := old.counts()
	nextCalls, nextCloses, _ := next.counts()
	if oldCalls != 1 || oldCloses != 1 || nextCalls != 13 || nextCloses != 0 {
		t.Fatalf("wrong transport lifecycle %d %d %d %d", oldCalls, oldCloses, nextCalls, nextCloses)
	}
	exported, e := c.ExportSession()
	if e != nil || exported.APIBase != fresh.APIBase {
		t.Fatal("replacement not committed")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	_, oldCloses, _ = old.counts()
	_, nextCloses, _ = next.counts()
	if oldCloses != 1 || nextCloses != 1 {
		t.Fatal("owned cleanup repeated or leaked")
	}
}

func TestClientPINProfileUnreadyRenewsBeforeTransportConstruction(t *testing.T) {
	b := clientFixture(t, "unready")
	b.Cookies = nil
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	fresh := clientFixture(t, "initial")
	renewals, builds := 0, 0
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{
		Renewal: func(ctx context.Context, _ sdkSession.SessionBundle, p sdkAuth.PINProvider, _ sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
			renewals++
			if _, e := p(ctx); e != nil {
				t.Error(e)
			}
			return fresh, nil
		},
		TransportFactory: func(got sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			builds++
			saved, e := sdkSession.LoadSessionBundle(path)
			if e != nil || saved.APIBase != fresh.APIBase {
				t.Error("initial transport before persistence")
			}
			return clientFake(t, got), nil
		},
	})
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
	if renewals != 1 || builds != 1 {
		t.Fatal("wrong initial renewal")
	}
	b.Deviceprint = nil
	if e = b.Save(path); e != nil {
		t.Fatal(e)
	}
	if _, e = NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { t.Fatal("provider called for missing identity"); return "", nil }, ClientOptions{}); e == nil {
		t.Fatal("profile without identity accepted")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	var insecure *sdkErrs.InsecureSessionFile
	if _, e = NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "", nil }, ClientOptions{}); !errors.As(e, &insecure) {
		t.Fatal("nonprivate PIN profile accepted")
	}
}

func TestClientAnyHTMLContentTypeAliasRejectsLoginResponse(t *testing.T) {
	b := clientFixture(t, "header-alias")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		r := clientResponse(200, `{"success":true}`)
		r.Headers = http.Header{"Content-Type": []string{"application/json"}, "content-type": []string{"TEXT/HTML"}}
		return r, nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for i := 0; i < 32; i++ {
		_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil)
		var expired *sdkErrs.AuthenticationExpired
		if !errors.As(e, &expired) {
			t.Fatal("ambiguous header let login response through")
		}
	}
}

func TestClientWarmUpIsExplicitDebouncedAndForceable(t *testing.T) {
	b := clientFixture(t, "warm")
	tr := clientFake(t, b)
	now := time.Unix(1000, 0)
	tr.post = func(_ context.Context, u string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if u != b.WebBase+"/api/warmUpSession" || len(p) != 0 {
			t.Error("wrong warmup request")
		}
		return clientResponse(204, "not JSON; allowed"), nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, Monotonic: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := c.WarmUp(context.Background(), false); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatalf("not debounced: %d", calls)
	}
	now = now.Add(59 * time.Second)
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	calls, _, _ = tr.counts()
	if calls != 1 {
		t.Fatal("premature warmup")
	}
	now = now.Add(time.Second)
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	calls, _, _ = tr.counts()
	if calls != 4 {
		t.Fatalf("force ignored: %d", calls)
	}
}
func TestClientWarmUpGateAndPersistenceMustSucceedBeforeDebounce(t *testing.T) {
	for _, tc := range []struct {
		name, kind, ctype string
		status            int
	}{
		{"empty-200", "ok", "application/json", 200}, {"204", "ok", "", 204}, {"html-200", "expired", "text/html", 200}, {"html-204", "expired", "text/html", 204}, {"redirect", "expired", "", 302}, {"503-html", "api", "text/html", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := clientFixture(t, "warm-gate")
			tr := clientFake(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				r := clientResponse(tc.status, "not JSON")
				r.Headers = http.Header{"Content-Type": []string{tc.ctype}}
				return r, nil
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			e = c.WarmUp(context.Background(), false)
			var api *sdkErrs.APIError
			var expired *sdkErrs.AuthenticationExpired
			if tc.kind == "ok" && e != nil || tc.kind == "api" && !errors.As(e, &api) || tc.kind == "expired" && !errors.As(e, &expired) {
				t.Fatalf("wrong warmup gate: %v", e)
			}
			_ = c.WarmUp(context.Background(), false)
			calls, _, _ := tr.counts()
			want := 1
			if tc.kind != "ok" {
				want = 2
			}
			if calls != want {
				t.Fatal("failed warmup debounced")
			}
		})
	}
	b := clientFixture(t, "warm-persist")
	tr := clientFake(t, b)
	path := clientPrivatePath(t)
	if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, SessionPath: path})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.WarmUp(context.Background(), false); e == nil {
		t.Fatal("save error hidden")
	}
	if e = c.WarmUp(context.Background(), false); e == nil {
		t.Fatal("failed save marked warm")
	}
	calls, _, _ := tr.counts()
	if calls != 2 {
		t.Fatal("save failure skipped warmup")
	}
	if e := os.Chmod(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if _, e = sdkSession.LoadSessionBundle(path); e != nil {
		t.Fatal("warmup did not save")
	}
}
