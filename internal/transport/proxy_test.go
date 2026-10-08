package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vasyza/sber-go/internal/errs"
	"github.com/vasyza/sber-go/internal/testproxy"
)

func TestProxyTLSClosureIsClassifiedBeforeSendingHTTP(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5"} {
		t.Run(scheme, func(t *testing.T) {
			var requests atomic.Int32
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
			listener := &handshakeDropListener{Listener: origin.Listener, drops: 100}
			origin.Listener = listener
			origin.Config.ErrorLog = log.New(io.Discard, "", 0)
			origin.StartTLS()
			defer origin.Close()
			destination := strings.TrimPrefix(origin.URL, "https://")
			var address string
			var stats *testproxy.Stats
			if scheme == "socks5" {
				host, counters := testproxy.SOCKS5(t, "", "", destination)
				address, stats = "socks5://"+host, counters
			} else {
				server, counters := testproxy.HTTP(t, scheme == "https", "", "", destination)
				address, stats = server.URL, counters
			}
			tr, err := NewAuthenticationTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: serverCA(t, origin), Timeout: time.Second, Proxy: ProxyOptions{URL: address}})
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			var callerTrace atomic.Bool
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{TLSHandshakeDone: func(tls.ConnectionState, error) { callerTrace.Store(true) }})
			_, err = tr.Post(ctx, origin.URL, map[string]any{"synthetic": "proof"}, RequestOptions{})
			var failure *errs.TransportError
			if !errors.As(err, &failure) || failure.Code != "proxy_tls_closed" || !callerTrace.Load() {
				t.Fatal("TLS closure lost its phase or the caller trace")
			}
			if requests.Load() != 0 || stats.Connections.Load() != 1 || listener.accepted.Load() != 1 {
				t.Fatal("TLS diagnostic sent or replayed HTTP")
			}
		})
	}
}

func TestProxyClosureAfterHTTPIsNotReportedAsTLSFailure(t *testing.T) {
	var requests atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		connection.Close()
	}))
	defer origin.Close()
	server, stats := testproxy.HTTP(t, false, "", "", strings.TrimPrefix(origin.URL, "https://"))
	tr, err := NewAuthenticationTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: serverCA(t, origin), Timeout: time.Second, Proxy: ProxyOptions{URL: server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	_, err = tr.Post(context.Background(), origin.URL, map[string]any{"synthetic": "proof"}, RequestOptions{})
	var failure *errs.TransportError
	if !errors.As(err, &failure) || failure.Code != "proxy_failed" || requests.Load() != 1 || stats.Connections.Load() != 1 {
		t.Fatal("HTTP closure was confused with TLS establishment or replayed")
	}
}

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
