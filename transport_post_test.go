package sber

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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
	if _, err := tr.Post(context.Background(), s.URL, nil, RequestOptions{}); err != ErrClosed {
		t.Fatal("closed POST resurrected")
	}
}
