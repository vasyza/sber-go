package sber

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

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
		var transport *TransportError
		if !errors.As(err, &transport) || transport.Code != "invalid_headers" {
			t.Error("conflicting owner duplicates accepted")
		}
	}
	if hits.Load() != before {
		t.Error("invalid header request reached localhost wire")
	}
}
