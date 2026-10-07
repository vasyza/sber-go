package bank

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// All transport fixtures are generated, in-process, and incapable of I/O.
type clientCall struct {
	method, target string
	body           map[string]any
	options        sdkTransport.RequestOptions
}
type clientFakeTransport struct {
	mu                 sync.Mutex
	jar                *sdkSession.CookieJar
	calls              []clientCall
	closed, closeCalls int
	get                func(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error)
	post               func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error)
	close              func() error
}

func clientFixture(t *testing.T, suffix string) sdkSession.SessionBundle {
	t.Helper()
	b, e := (sdkSession.SberCredentials{UFSSession: "session-" + suffix, UFSToken: "token-" + suffix}).ToBundle(sdkSession.CredentialsBundleOptions{APIBase: "https://web-node-" + suffix + ".online.sberbank.ru", WebBase: "https://web-" + suffix + ".online.sberbank.ru", Deviceprint: sdkTransport.PtrString("version=1.7.3&fixture=true")})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func clientFake(t *testing.T, b sdkSession.SessionBundle) *clientFakeTransport {
	t.Helper()
	j, e := sdkSession.NewCookieJar(b.Cookies)
	if e != nil {
		t.Fatal(e)
	}
	return &clientFakeTransport{jar: j}
}
func (f *clientFakeTransport) record(c clientCall) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}
func (f *clientFakeTransport) Get(c context.Context, u string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	f.record(clientCall{method: "GET", target: u, options: o})
	if f.get != nil {
		return f.get(c, u, o)
	}
	return nil, fmt.Errorf("unexpected synthetic GET")
}
func (f *clientFakeTransport) Post(c context.Context, u string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	f.record(clientCall{method: "POST", target: u, body: p, options: o})
	if f.post != nil {
		return f.post(c, u, p, o)
	}
	return clientResponse(200, `{"success":true}`), nil
}
func (f *clientFakeTransport) PostForm(context.Context, string, map[string]string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	panic("credential POST forbidden in fixture")
}
func (f *clientFakeTransport) CookieJar() *sdkSession.CookieJar { return f.jar }
func (f *clientFakeTransport) Close() error {
	f.mu.Lock()
	f.closeCalls++
	f.mu.Unlock()
	if f.close != nil {
		if e := f.close(); e != nil {
			return e
		}
	}
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
	return nil
}
func (f *clientFakeTransport) counts() (int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls), f.closeCalls, f.closed
}
func (f *clientFakeTransport) snapshotCalls() []clientCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]clientCall(nil), f.calls...)
}
func clientResponse(status int, raw string) *sdkTransport.Response {
	return &sdkTransport.Response{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json"}}, Content: []byte(raw)}
}

func mustClientURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, e := url.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
func TestClientOwnsRotatingTransportAndRedacts(t *testing.T) {
	b := clientFixture(t, "old")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	// Input identities are copied and a complete Set-Cookie jar remains authoritative.
	b.Cookies[0].Value = "caller-corruption"
	*b.Deviceprint = "caller-corruption"
	u := mustClientURL(t, sdkSession.AppOrigin+"/")
	if e := tr.jar.ApplySetCookie(u, []string{"UFS-SESSION=session-new; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=Lax", "UFS-TOKEN=token-new; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly"}); e != nil {
		t.Fatal(e)
	}
	exported, e := c.ExportSession()
	if e != nil {
		t.Fatal(e)
	}
	creds, e := c.ExportCredentials()
	if e != nil || creds.UFSSession != "session-new" || creds.UFSToken != "token-new" {
		t.Fatalf("bad rotated credentials: %v", e)
	}
	if *exported.Deviceprint != "version=1.7.3&fixture=true" {
		t.Fatal("aliased identity")
	}
	exported.Cookies[0].Value = "export-corruption"
	*exported.Deviceprint = "export-corruption"
	next, e := c.ExportSession()
	if e != nil || *next.Deviceprint != "version=1.7.3&fixture=true" {
		t.Fatal("aliased export")
	}
	for _, s := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%+v", ClientOptions{Transport: tr, SessionPath: "synthetic-sensitive-path"})} {
		if strings.Contains(s, "session-new") || strings.Contains(s, "synthetic-sensitive-path") {
			t.Fatal("unredacted client")
		}
	}
	raw, e := json.Marshal(c)
	if e != nil || strings.Contains(string(raw), "token-new") {
		t.Fatal("unredacted JSON")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	calls, closes, _ := tr.counts()
	if calls != 0 || closes != 1 {
		t.Fatalf("unexpected lifecycle calls %d %d", calls, closes)
	}
}
