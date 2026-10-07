package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasyza/sber-go/internal/errs"
	"github.com/vasyza/sber-go/internal/testproxy"
)

func TestVerifiedNativeRequestsThroughExplicitProxies(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5"} {
		for _, authenticated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/auth=%t", scheme, authenticated), func(t *testing.T) {
				var requests atomic.Int32
				origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Header.Get("Proxy-Authorization") != "" {
						t.Error("proxy credentials reached origin")
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				defer origin.Close()
				username, password := "", ""
				if authenticated {
					username, password = "synthetic-proxy-user", "synthetic-proxy-secret"
				}
				destination := strings.TrimPrefix(origin.URL, "https://")
				var address string
				var stats *testproxy.Stats
				if scheme == "socks5" {
					var host string
					host, stats = testproxy.SOCKS5(t, username, password, destination)
					address = "socks5://" + host
				} else {
					server, counters := testproxy.HTTP(t, scheme == "https", username, password, destination)
					address, stats = server.URL, counters
				}
				options := TransportOptions{AllowUnready: true, CABundle: serverCA(t, origin), Timeout: 2 * time.Second, Proxy: ProxyOptions{URL: address, Username: username, Password: password}}
				tr, err := NewAuthenticationTransport(unreadyBundle(), options)
				if err != nil {
					t.Fatal(err)
				}
				defer tr.Close()
				if _, err := tr.Post(context.Background(), origin.URL, map[string]any{"synthetic": true}, RequestOptions{}); err != nil {
					t.Fatal(err)
				}
				if requests.Load() != 1 || stats.Connections.Load() != 1 || stats.Authorized.Load() != 1 {
					t.Fatal("request bypassed proxy or was replayed")
				}
				for _, output := range []string{fmt.Sprintf("%#v", options), fmt.Sprintf("%+v", tr)} {
					if strings.Contains(output, "synthetic-proxy") {
						t.Fatal("proxy secret leaked")
					}
				}
				raw, _ := json.Marshal(options)
				if strings.Contains(string(raw), "synthetic-proxy") {
					t.Fatal("transport JSON leaked proxy secret")
				}
			})
		}
	}
}

func TestProxyFailureHasNoDirectFallbackOrSecretLeak(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer origin.Close()
	for _, scheme := range []string{"http", "https", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := listener.Addr().String()
			listener.Close()
			tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: serverCA(t, origin), Timeout: time.Second, Proxy: ProxyOptions{URL: scheme + "://" + address, Username: "synthetic-proxy-user", Password: "synthetic-proxy-secret"}})
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			_, err = tr.Get(context.Background(), origin.URL, RequestOptions{})
			if err == nil || hits.Load() != 0 || strings.Contains(fmt.Sprintf("%+v", err), "synthetic-proxy") {
				t.Fatal("proxy failure was unsafe")
			}
			var failure *errs.TransportError
			if !errors.As(err, &failure) || failure.Code != "proxy_failed" {
				t.Fatal("proxy failure lost classification")
			}
		})
	}
}

func TestProxyAuthenticationTLSAndCancellation(t *testing.T) {
	var hits atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer origin.Close()
	server, _ := testproxy.HTTP(t, false, "synthetic-user", "synthetic-password", strings.TrimPrefix(origin.URL, "https://"))
	for _, tc := range []struct{ password, ca, code string }{
		{"wrong-password", serverCA(t, origin), "proxy_authentication"},
		{"synthetic-password", "", "tls_untrusted"},
	} {
		tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: tc.ca, Timeout: time.Second, Proxy: ProxyOptions{URL: server.URL, Username: "synthetic-user", Password: tc.password}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = tr.Get(context.Background(), origin.URL, RequestOptions{})
		tr.Close()
		var failure *errs.TransportError
		if !errors.As(err, &failure) || failure.Code != tc.code || hits.Load() != 0 {
			t.Fatal("proxy auth or TLS failure was not preserved")
		}
	}
	started := make(chan struct{})
	stall := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
	defer stall.Close()
	tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: serverCA(t, origin), Timeout: time.Second, Proxy: ProxyOptions{URL: stall.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := tr.Get(ctx, origin.URL, RequestOptions{}); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("proxy request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("proxy cancellation lost")
		}
	case <-time.After(time.Second):
		t.Fatal("proxy ignored cancellation")
	}
}
