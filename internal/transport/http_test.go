package transport

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"errors"
	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestHTTPNativeCompressionAndDecodedSizeBound(t *testing.T) {
	const content = "synthetic native compression content"
	for _, enc := range []string{"gzip", "deflate", "br", "zstd"} {
		t.Run(enc, func(t *testing.T) {
			var raw bytes.Buffer
			var writer io.WriteCloser
			switch enc {
			case "gzip":
				writer = gzip.NewWriter(&raw)
			case "deflate":
				writer = zlib.NewWriter(&raw)
			case "br":
				writer = brotli.NewWriter(&raw)
			case "zstd":
				w, err := zstd.NewWriter(&raw)
				if err != nil {
					t.Fatal(err)
				}
				writer = w
			}
			if _, err := io.WriteString(writer, content); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", enc)
				w.Write(raw.Bytes())
			}))
			defer s.Close()
			tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true})
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			response, err := tr.Get(context.Background(), s.URL, RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd"})
			if err != nil || response.Text() != content {
				t.Fatalf("native %s decoder not usable: %v", enc, err)
			}
			small, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true, MaxResponseBytes: 4})
			if err != nil {
				t.Fatal(err)
			}
			defer small.Close()
			if _, err := small.Get(context.Background(), s.URL, RequestOptions{}); safeTransportCode(err) != "response_too_large" {
				t.Fatal("decoded size bound failed")
			}
		})
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		io.WriteString(w, strings.Repeat("invalid", 3))
	}))
	defer s.Close()
	tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if _, err := tr.Get(context.Background(), s.URL, RequestOptions{}); safeTransportCode(err) != "invalid_encoding" {
		t.Fatal("invalid compressed response accepted")
	}
}

func TestHTTPPostBodySemanticsAndNoReplay(t *testing.T) {
	type capture struct {
		method, body, contentType string
		length                    int64
	}
	received := make(chan capture, 8)
	var uncertain atomic.Int64
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		received <- capture{r.Method, string(raw), r.Header.Get("Content-Type"), r.ContentLength}
		if r.URL.Path == "/uncertain" {
			uncertain.Add(1)
			c, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				c.Close()
			}
			return
		}
		io.WriteString(w, "synthetic")
	}))
	defer s.Close()
	tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	_, err = tr.Post(context.Background(), s.URL, map[string]any{"a": "synthetic", "n": 3}, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c := <-received
	if c.method != "POST" || c.body != `{"a":"synthetic","n":3}` {
		t.Fatal("JSON wire semantics changed")
	}
	_, err = tr.PostForm(context.Background(), s.URL, map[string]string{"a": "a+b c", "x": "ж"}, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c = <-received
	if c.body != "a=a%2Bb+c&x=%D0%B6" || c.contentType != "application/x-www-form-urlencoded" {
		t.Fatal("form wire semantics changed")
	}
	_, err = tr.Post(context.Background(), s.URL, nil, RequestOptions{Headers: HeaderOverrides{"Content-Type": nil}})
	if err != nil {
		t.Fatal(err)
	}
	c = <-received
	if c.body != "" || c.length != 0 || c.contentType != "" {
		t.Fatal("nil JSON became null/object/chunked body")
	}
	_, err = tr.Post(context.Background(), s.URL, map[string]any{}, RequestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c = <-received
	if c.body != "{}" {
		t.Fatal("empty object became no body")
	}
	_, err = tr.Post(context.Background(), s.URL+"/uncertain", nil, RequestOptions{Headers: HeaderOverrides{"Idempotency-Key": stringPtr("synthetic-key")}})
	if err == nil {
		t.Fatal("ambiguous network failure not surfaced")
	}
	<-received
	if uncertain.Load() != 1 {
		t.Fatal("empty POST replayed")
	}
	if _, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true, Retry: 1}); err == nil {
		t.Fatal("automatic retry permitted")
	}
}
func TestHTTPContextCancellationAndClose(t *testing.T) {
	started := make(chan struct{}, 8)
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() }))
	defer s.Close()
	tr, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, s), AllowUnready: true, MaxClients: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := tr.Get(ctx, s.URL+"/?synthetic-secret", RequestOptions{}); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("cancellation/error redaction lost")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop request")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = tr.Get(ctx, s.URL, RequestOptions{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline identity lost")
	}
	<-started
	go func() { _, err := tr.Get(context.Background(), s.URL, RequestOptions{}); done <- err }()
	<-started
	// One request is queued behind the capacity gate when Close occurs.
	queued := make(chan error, 1)
	go func() { _, err := tr.Get(context.Background(), s.URL, RequestOptions{}); queued <- err }()
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	for _, ch := range []chan error{done, queued} {
		select {
		case err := <-ch:
			if err == nil {
				t.Fatal("close let pending request succeed")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("close left pending request")
		}
	}
	if _, err := tr.Post(context.Background(), s.URL, nil, RequestOptions{}); err != sdkErrs.ErrClosed {
		t.Fatal("closed POST resurrected")
	}
}

func TestHTTPConfiguredTimeoutAllowsSlowerVerifiedHandshake(t *testing.T) {
	stop := make(chan struct{})
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	origin.Config.ErrorLog = log.New(io.Discard, "", 0)
	origin.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
		timer := time.NewTimer(5300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-stop:
		}
		return nil, nil
	}}
	origin.StartTLS()
	defer origin.Close()
	defer close(stop)
	transport, err := NewHTTPTransport(unreadyBundle(), TransportOptions{
		AllowUnready: true, CABundle: serverCA(t, origin), Timeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Get(context.Background(), origin.URL, RequestOptions{})
	if err != nil || response == nil || response.StatusCode != http.StatusNoContent {
		t.Fatal("a hidden phase timeout overrode the configured request budget")
	}
}
