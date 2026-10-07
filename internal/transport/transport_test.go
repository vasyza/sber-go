package transport

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBrowserBootstrapSnapshotContractAndRedaction(t *testing.T) {
	result := BrowserBootstrapResult{HTML: `<script>window.config = {processId:"synthetic-html-process-secret"};</script>`, URL: PublicBootstrapURL, Cookies: syntheticBundle().Cookies, Browser: syntheticBundle().Browser}
	normalized, err := NewBrowserBootstrapResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Cookies[0].Domain != "online.sberbank.ru" || normalized.HTML != result.HTML {
		t.Fatal("snapshot contract changed")
	}
	for _, s := range []string{fmt.Sprintf("%#v", normalized), fmt.Sprintf("%+v", normalized)} {
		if strings.Contains(s, "synthetic") {
			t.Fatal("snapshot formatting leaked")
		}
	}
	raw, _ := json.Marshal(normalized)
	if strings.Contains(string(raw), "synthetic") {
		t.Fatal("snapshot JSON leaked")
	}
	bad := result
	bad.Cookies = append(bad.Cookies, bad.Cookies[0])
	if _, err := NewBrowserBootstrapResult(bad); err == nil {
		t.Fatal("duplicate cookies accepted")
	}
	bad = result
	bad.HTML = ""
	if _, err := NewBrowserBootstrapResult(bad); err == nil {
		t.Fatal("empty HTML accepted")
	}
	calls := 0
	provider := BrowserBootstrapFunc(func(ctx context.Context, b sdkSession.SessionBundle, u string) (BrowserBootstrapResult, error) {
		calls++
		return normalized, nil
	})
	returned, err := provider.Bootstrap(context.Background(), unreadyBundle(), PublicBootstrapURL)
	if err != nil || calls != 1 || returned.HTML != normalized.HTML {
		t.Fatal("provider adapter failed")
	}
}
func TestRecognizableBrowserCheckOnly(t *testing.T) {
	shell := `<script src="/TSPD/synthetic.js"></script><noscript>Please enable JavaScript</noscript>`
	if !IsBrowserCheck(shell) {
		t.Fatal("observed JS shell not recognized")
	}
	for _, html := range []string{"", `<script>enable JavaScript</script>`, shell + `<script>window . config = {};</script>`} {
		if IsBrowserCheck(html) {
			t.Fatal("normal/malformed config treated as JS check")
		}
	}
}

func stringPtr(s string) *string { return &s }

func int64Ptr(v int64) *int64 { return &v }

func syntheticBundle() sdkSession.SessionBundle {
	return sdkSession.SessionBundle{APIBase: "https://web-node2.online.sberbank.ru", WebBase: "https://web2.online.sberbank.ru", SchemaVersion: 4,
		Cookies:     []sdkSession.CookieRecord{{Name: "UFS-SESSION", Value: "synthetic-session-secret", Domain: ".online.sberbank.ru", Path: "/", Secure: true, HTTPOnly: true, HostOnly: false, Expires: int64Ptr(4102444800), SameSite: stringPtr("Strict")}},
		Browser:     sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "User-Agent", Value: "synthetic-agent-secret"}}},
		Deviceprint: stringPtr("synthetic-device-secret"), AntifraudDeviceprint: stringPtr("synthetic-antifraud-secret"), CapturedAt: stringPtr("2026-01-01T00:00:00+00:00"),
	}
}

// Profile fixtures must be private independently of the developer's umask.
func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFoundationGETCanonicalOverrides(t *testing.T) {
	var hits atomic.Int64
	requests := make(chan http.Header, 4)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		requests <- r.Header.Clone()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer origin.Close()
	tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	for _, tt := range []struct {
		name    string
		headers HeaderOverrides
		accept  string
	}{
		{"lowercase", HeaderOverrides{"accept": stringPtr("application/audit")}, "application/audit"},
		{"nil-removal", HeaderOverrides{"accept": nil, "content-type": nil, "origin": nil}, ""},
		{"mixed-case", HeaderOverrides{"aCcEpT": stringPtr("application/mixed")}, "application/mixed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.Get(context.Background(), origin.URL, RequestOptions{Headers: tt.headers})
			if err != nil {
				t.Fatalf("valid canonical GET override rejected: %v", err)
			}
			h := <-requests
			if h.Get("Accept") != tt.accept {
				t.Error("GET override did not replace default")
			}
			if tt.name == "nil-removal" {
				for _, name := range []string{"Accept", "Content-Type", "Origin"} {
					if _, exists := h[name]; exists {
						t.Errorf("explicit nil %s still present", name)
					}
				}
			}
		})
	}
	before := hits.Load()
	for _, h := range []HeaderOverrides{
		{"accept": stringPtr("one"), "Accept": stringPtr("two")},
		{"accept": nil, "Accept": stringPtr("two")},
	} {
		_, err := tr.Get(context.Background(), origin.URL, RequestOptions{Headers: h})
		var transport *sdkErrs.TransportError
		if !errors.As(err, &transport) || transport.Code != "invalid_headers" {
			t.Error("conflicting owner duplicates accepted")
		}
	}
	if hits.Load() != before {
		t.Error("invalid header request reached localhost wire")
	}
}

func serverCA(t *testing.T, s *httptest.Server) string {
	t.Helper()
	p := filepath.Join(testPrivateDir(t), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func unreadyBundle() sdkSession.SessionBundle {
	return sdkSession.SessionBundle{APIBase: sdkSession.AppOrigin, WebBase: sdkSession.AppOrigin}
}
func TestHTTPGetVerifiedTLSHeadersNoProxyOrRedirect(t *testing.T) {
	var hits atomic.Int64
	var proxyHits atomic.Int64
	requests := make(chan *http.Request, 4)
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		requests <- r.Clone(context.Background())
		if r.URL.Path == "/redirect" {
			w.Header().Set("Location", "/followed")
			w.WriteHeader(302)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "synthetic=secret-cookie-value; Path=/; Secure; HttpOnly; SameSITE=lAx")
		fmt.Fprint(w, `{"synthetic":"secret-response-value"}`)
	}))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { proxyHits.Add(1); w.WriteHeader(502) }))
	defer proxy.Close()
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"} {
		t.Setenv(name, proxy.URL)
	}
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	b := unreadyBundle()
	b.Browser = sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "user-agent", Value: "SyntheticBrowser/1.0"}, {Name: "accept-language", Value: "ru-RU"}}}
	tr, err := NewHTTPTransport(b, TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	response, err := tr.Get(context.Background(), origin.URL+"/first", RequestOptions{AcceptEncoding: "identity", Headers: HeaderOverrides{"Content-Type": nil, "Origin": nil}})
	if err != nil {
		t.Fatal(err)
	}
	request := <-requests
	if response.StatusCode != 200 || response.Text() != `{"synthetic":"secret-response-value"}` {
		t.Fatal("usable response missing")
	}
	var data map[string]string
	if err := response.DecodeJSON(&data); err != nil || data["synthetic"] != "secret-response-value" {
		t.Fatal("JSON semantics differ")
	}
	if request.Header.Get("User-Agent") != "SyntheticBrowser/1.0" || request.Header.Get("Accept-Language") != "ru-RU" || request.Header.Get("Content-Type") != "" || request.Header.Get("Origin") != "" || request.Header.Get("X-Requested-With") != "" || request.Header.Get("Accept") != "*/*" {
		t.Fatal("header source semantics differ")
	}
	c := tr.CookieJar().Snapshot()
	if len(c) != 1 || !c[0].Secure || !c[0].HTTPOnly || !c[0].HostOnly || *c[0].SameSite != "Lax" {
		t.Fatal("response-cookie metadata lost")
	}
	response, err = tr.Get(context.Background(), origin.URL+"/redirect", RequestOptions{})
	if err != nil || response.StatusCode != 302 {
		t.Fatal("redirect followed")
	}
	<-requests
	if hits.Load() != 2 || proxyHits.Load() != 0 {
		t.Fatal("proxy used or redirect followed")
	}
	for _, v := range []any{tr, response, RequestOptions{Headers: HeaderOverrides{"Authorization": stringPtr("secret-header-value")}}} {
		j, _ := json.Marshal(v)
		for _, s := range []string{string(j), fmt.Sprintf("%#v", v), fmt.Sprintf("%+v", v)} {
			for _, secret := range []string{"secret-cookie-value", "secret-response-value", "secret-header-value"} {
				if strings.Contains(s, secret) {
					t.Fatal("transport representation leaked")
				}
			}
		}
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Get(context.Background(), origin.URL, RequestOptions{}); err != sdkErrs.ErrClosed {
		t.Fatal("closed transport resurrected")
	}
}
