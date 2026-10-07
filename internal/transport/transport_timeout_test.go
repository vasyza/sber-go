package transport

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
