package transport

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestAuthenticationConnectionReuseDoesNotReplayPOST(t *testing.T) {
	var connections, requests, uncertain atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/uncertain" {
			uncertain.Add(1)
			connection, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = connection.Close()
			}
			return
		}
		_, _ = io.WriteString(w, "synthetic")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	defer server.Close()
	transport, err := NewAuthenticationTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, server), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	ctx := context.Background()
	if _, err := transport.Get(ctx, server.URL, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.PostForm(ctx, server.URL, map[string]string{"synthetic": "proof"}, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Post(ctx, server.URL, map[string]any{"synthetic": "encrypted-pin"}, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if connections.Load() != 1 || requests.Load() != 3 {
		t.Fatal("authentication discarded its usable verified connection")
	}
	for _, body := range []map[string]any{nil, {"synthetic": "proof"}} {
		for _, header := range []string{"Idempotency-Key", "X-Idempotency-Key"} {
			if _, err := transport.Post(ctx, server.URL, body, RequestOptions{Headers: HeaderOverrides{header: stringPtr("")}}); err == nil {
				t.Fatal("pooled auth transport accepted replay-enabling metadata")
			}
		}
	}
	if requests.Load() != 3 {
		t.Fatal("replay-enabling metadata reached the server")
	}
	if _, err := transport.Post(ctx, server.URL+"/uncertain", map[string]any{"synthetic": "proof"}, RequestOptions{}); err == nil {
		t.Fatal("ambiguous authentication response was not reported")
	}
	if uncertain.Load() != 1 || requests.Load() != 4 {
		t.Fatal("authentication POST was repeated after connection loss")
	}
	// Seamless navigation is an empty POST. It has the same no-replay
	// requirement, including when it follows a successfully reused connection.
	if _, err := transport.Get(ctx, server.URL, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Post(ctx, server.URL+"/uncertain", nil, RequestOptions{}); err == nil {
		t.Fatal("ambiguous empty authentication response was not reported")
	}
	if uncertain.Load() != 2 || requests.Load() != 6 {
		t.Fatal("empty authentication POST was repeated after connection loss")
	}
}
