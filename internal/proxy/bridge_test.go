package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vasyza/sber-go/internal/testproxy"
)

func TestAuthenticatedSOCKSBridgeAndCleanup(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credentials reached TLS origin")
		}
		w.Write([]byte("synthetic-response"))
	}))
	defer origin.Close()
	address, stats := testproxy.SOCKS5(t, "synthetic-user", "synthetic-password", strings.TrimPrefix(origin.URL, "https://"))
	bridge, err := StartBridge(context.Background(), Options{URL: "socks5://" + address, Username: "synthetic-user", Password: "synthetic-password"}, time.Second, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	local := bridge.Options()
	if local.Username == "synthetic-user" || local.Password == "synthetic-password" {
		t.Fatal("bridge reused remote credentials")
	}
	proxyURL, err := local.Endpoint()
	if err != nil {
		t.Fatal(err)
	}
	transport := origin.Client().Transport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxyURL)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	response, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if stats.Authorized.Load() != 1 || response.StatusCode != 200 {
		t.Fatal("bridge did not use authenticated SOCKS5")
	}
	bridge.Close()
	endpoint := strings.TrimPrefix(local.URL, "http://")
	if conn, err := net.DialTimeout("tcp", endpoint, 100*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("closed bridge still accepts connections")
	}
}

func TestBridgeRejectsUnauthenticatedLocalRequestsAndCancelsPendingDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bridge, err := StartBridge(ctx, Options{URL: "socks5://" + listener.Addr().String(), Username: "synthetic-user", Password: "synthetic-password"}, time.Second, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	local := bridge.Options()
	unauthenticated := Options{URL: local.URL}
	noAuthURL, _ := unauthenticated.Endpoint()
	transport := &http.Transport{Proxy: http.ProxyURL(noAuthURL)}
	defer transport.CloseIdleConnections()
	if _, err := (&http.Client{Transport: transport, Timeout: time.Second}).Get("https://localhost:443"); err == nil {
		t.Fatal("bridge accepted a local request without its token")
	}
	proxyURL, _ := local.Endpoint()
	authorized := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
	defer authorized.CloseIdleConnections()
	done := make(chan error, 1)
	go func() {
		_, err := (&http.Client{Transport: authorized, Timeout: 2 * time.Second}).Get("https://localhost:443")
		done <- err
	}()
	var conn net.Conn
	select {
	case conn = <-accepted:
	case <-time.After(time.Second):
		t.Fatal("SOCKS connection did not start")
	}
	defer conn.Close()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled bridge returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled bridge left a request open")
	}
}
