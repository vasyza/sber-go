package bank

import (
	"context"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientDefaultRenewalUsesAuthenticationConnectionPolicy(t *testing.T) {
	for _, tt := range []struct {
		name        string
		auth        bool
		connections int64
	}{
		{"business", false, 3},
		{"authentication", true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var connections, requests atomic.Int64
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "synthetic")
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
			}
			server.StartTLS()
			defer server.Close()
			ca := filepath.Join(filepath.Dir(clientPrivatePath(t)), "local-ca.pem")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			options, err := clientNormalizeOptions(ClientOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: ca, Timeout: 3 * time.Second}})
			if err != nil {
				t.Fatal(err)
			}
			var transport sdkTransport.Transport
			bundle := clientFixture(t, "connection-policy")
			if tt.auth {
				transport, err = options.AuthOptions.TransportFactory(bundle, options.AuthOptions.TransportOptions)
			} else {
				transport, err = options.TransportFactory(bundle, options.TransportOptions)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			ctx := context.Background()
			if _, err := transport.Get(ctx, server.URL, sdkTransport.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := transport.PostForm(ctx, server.URL, map[string]string{"synthetic": "proof"}, sdkTransport.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := transport.Post(ctx, server.URL, nil, sdkTransport.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 3 || connections.Load() != tt.connections {
				t.Fatalf("requests=%d connections=%d; want 3 requests over %d connections", requests.Load(), connections.Load(), tt.connections)
			}
		})
	}
}
