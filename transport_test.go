package sber

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func serverCA(t *testing.T, s *httptest.Server) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func unreadyBundle() SessionBundle { return SessionBundle{APIBase: AppOrigin, WebBase: AppOrigin} }
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
	b.Browser = BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "SyntheticBrowser/1.0"}, {Name: "accept-language", Value: "ru-RU"}}}
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
	if _, err := tr.Get(context.Background(), origin.URL, RequestOptions{}); err != ErrClosed {
		t.Fatal("closed transport resurrected")
	}
}
