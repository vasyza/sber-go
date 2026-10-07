package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// The first accepted socket closes before the server completes TLS or sees
// application data. A subsequent socket can serve one ordinary verified POST.
type handshakeDropListener struct {
	net.Listener
	drops         int64
	accepted      atomic.Int64
	firstAccepted chan struct{}
}

func (l *handshakeDropListener) Accept() (net.Conn, error) {
	connection, err := l.Listener.Accept()
	if err == nil {
		count := l.accepted.Add(1)
		if count == 1 && l.firstAccepted != nil {
			close(l.firstAccepted)
		}
		if count <= l.drops {
			_ = connection.Close()
		}
	}
	return connection, err
}

func TestVerifiedTLSRecoversBeforeSendingOnePOST(t *testing.T) {
	for _, authentication := range []bool{false, true} {
		t.Run(map[bool]string{false: "business", true: "authentication"}[authentication], func(t *testing.T) {
			var requests atomic.Int64
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost {
					t.Error("request method changed")
				}
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "synthetic")
			}))
			listener := &handshakeDropListener{Listener: origin.Listener, drops: 1}
			origin.Listener = listener
			origin.Config.ErrorLog = log.New(io.Discard, "", 0)
			origin.StartTLS()
			defer origin.Close()
			transport, err := newHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true}, authentication)
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			result, err := transport.Post(context.Background(), origin.URL, map[string]any{"synthetic": "proof"}, RequestOptions{})
			if err != nil || result == nil || result.StatusCode != 200 || requests.Load() != 1 || listener.accepted.Load() != 2 {
				t.Fatal("a pre-HTTP connection loss did not recover with exactly one POST")
			}
		})
	}
}

func TestVerifiedTLSRecoveryIsBoundedBeforeHTTP(t *testing.T) {
	var requests atomic.Int64
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1) }))
	listener := &handshakeDropListener{Listener: origin.Listener, drops: 100}
	origin.Listener = listener
	origin.Config.ErrorLog = log.New(io.Discard, "", 0)
	origin.StartTLS()
	defer origin.Close()
	transport, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	if _, err := transport.Post(context.Background(), origin.URL, map[string]any{"synthetic": "proof"}, RequestOptions{}); err == nil || requests.Load() != 0 || listener.accepted.Load() != 3 {
		t.Fatal("TLS establishment did not stop after three attempts without HTTP")
	}
}

func TestVerifiedTLSRecoveryDoesNotRetryCertificateFailure(t *testing.T) {
	var requests atomic.Int64
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1) }))
	listener := &handshakeDropListener{Listener: origin.Listener}
	origin.Listener = listener
	origin.Config.ErrorLog = log.New(io.Discard, "", 0)
	origin.StartTLS()
	defer origin.Close()
	transport, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	_, err = transport.Post(context.Background(), origin.URL, nil, RequestOptions{})
	if safeTransportCode(err) != "tls_untrusted" || requests.Load() != 0 || listener.accepted.Load() != 1 {
		t.Fatal("certificate failure was retried or accepted")
	}
}

func TestVerifiedTLSRecoveryUsesOneRequestBudget(t *testing.T) {
	var requests atomic.Int64
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1) }))
	listener := &handshakeDropListener{Listener: origin.Listener, drops: 100}
	origin.Listener = listener
	origin.Config.ErrorLog = log.New(io.Discard, "", 0)
	origin.StartTLS()
	defer origin.Close()
	transport, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err = transport.Post(ctx, origin.URL, nil, RequestOptions{})
	if safeTransportCode(err) != "timeout" || requests.Load() != 0 || listener.accepted.Load() != 1 {
		t.Fatal("TLS recovery restarted the caller deadline")
	}
}

func TestVerifiedTLSRecoveryDoesNotReplayTransmittedPOST(t *testing.T) {
	for _, authentication := range []bool{false, true} {
		t.Run(map[bool]string{false: "business", true: "authentication"}[authentication], func(t *testing.T) {
			var requests atomic.Int64
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				connection, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = connection.Close()
				}
			}))
			listener := &handshakeDropListener{Listener: origin.Listener, drops: 1}
			origin.Listener = listener
			origin.Config.ErrorLog = log.New(io.Discard, "", 0)
			origin.StartTLS()
			defer origin.Close()
			transport, err := newHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true}, authentication)
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			if _, err := transport.Post(context.Background(), origin.URL, map[string]any{"synthetic": "proof"}, RequestOptions{}); err == nil || requests.Load() != 1 || listener.accepted.Load() != 2 {
				t.Fatal("a transmitted POST was replayed after response loss")
			}
		})
	}
}

func TestVerifiedTLSRecoveryStopsWhenTransportCloses(t *testing.T) {
	var requests atomic.Int64
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { requests.Add(1) }))
	listener := &handshakeDropListener{Listener: origin.Listener, drops: 100, firstAccepted: make(chan struct{})}
	origin.Listener = listener
	origin.Config.ErrorLog = log.New(io.Discard, "", 0)
	origin.StartTLS()
	defer origin.Close()
	transport, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	done := make(chan error, 1)
	go func() { _, err := transport.Post(context.Background(), origin.URL, nil, RequestOptions{}); done <- err }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	select {
	case <-listener.firstAccepted:
	case <-deadline.C:
		t.Fatal("TLS connection did not start")
	}
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || listener.accepted.Load() != 1 || requests.Load() != 0 {
			t.Fatal("closed transport continued TLS recovery")
		}
	case <-deadline.C:
		t.Fatal("closed transport retained a TLS recovery request")
	}
}

func TestVerifiedTLSAdvertisesOnlySupportedHTTPProtocol(t *testing.T) {
	var requests atomic.Int64
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.TLS == nil || r.TLS.NegotiatedProtocol != "http/1.1" || r.Proto != "HTTP/1.1" {
			t.Error("advertised protocol differs from the HTTP transport")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	origin.Config.ErrorLog = log.New(io.Discard, "", 0)
	origin.TLS = &tls.Config{NextProtos: []string{"http/1.1"}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		if len(hello.SupportedProtos) != 1 || hello.SupportedProtos[0] != "http/1.1" {
			return nil, errors.New("synthetic unsupported protocol offer")
		}
		return nil, nil
	}}
	origin.StartTLS()
	defer origin.Close()
	transport, err := NewHTTPTransport(unreadyBundle(), TransportOptions{CABundle: serverCA(t, origin), AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	response, err := transport.Post(context.Background(), origin.URL, nil, RequestOptions{})
	if err != nil || response == nil || response.StatusCode != http.StatusNoContent || requests.Load() != 1 {
		t.Fatal("TLS did not advertise the supported HTTP/1.1 protocol")
	}
}
