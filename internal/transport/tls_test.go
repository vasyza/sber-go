package transport

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

func bundledRootForTest(t *testing.T) *x509.Certificate {
	t.Helper()
	data, err := os.ReadFile("certificates/russian_trusted_root_ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	block, rest := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		t.Fatal("bundled asset must contain exactly one public certificate")
	}
	fingerprint := sha256.Sum256(block.Bytes)
	if hex.EncodeToString(fingerprint[:]) != "d26d2d0231b7c39f92cc738512ba54103519e4405d68b5bd703e9788ca8ecf31" {
		t.Fatal("bundled root differs from the verified public certificate")
	}
	root, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !root.IsCA || !bytes.Equal(root.RawSubject, root.RawIssuer) || root.CheckSignatureFrom(root) != nil {
		t.Fatal("bundled certificate is not a valid self-signed CA")
	}
	return root
}

func TestDefaultTransportsTrustBundledBankRoot(t *testing.T) {
	root := bundledRootForTest(t)
	for _, tt := range []struct {
		name string
		new  func() (*HTTPTransport, error)
	}{
		{"business", func() (*HTTPTransport, error) {
			return NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true})
		}},
		{"authentication", func() (*HTTPTransport, error) {
			return NewAuthenticationTransport(unreadyBundle(), TransportOptions{AllowUnready: true})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := tt.new()
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			if _, err := root.Verify(x509.VerifyOptions{Roots: tr.wire.TLSClientConfig.RootCAs}); err != nil {
				t.Fatalf("bank root is not trusted without a CA file: %v", err)
			}
		})
	}
}

func TestExplicitCABundleReplacesDefaultTrust(t *testing.T) {
	server, ca, _ := generatedTLSServer(t, false, false)
	pool, err := applicationRoots(ca)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Certificate().Verify(x509.VerifyOptions{Roots: pool, DNSName: "127.0.0.1"}); err != nil {
		t.Fatalf("explicit synthetic CA was not trusted: %v", err)
	}
	if _, err := bundledRootForTest(t).Verify(x509.VerifyOptions{Roots: pool}); err == nil {
		t.Fatal("explicit CA bundle unexpectedly inherited bundled trust")
	}
}

func TestDefaultRootsWithoutSystemBundle(t *testing.T) {
	empty := filepath.Join(testPrivateDir(t), "empty.pem")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name  string
		paths []string
	}{
		{"none", nil},
		{"unavailable", []string{empty + ".missing", empty, filepath.Dir(empty)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := defaultApplicationRoots(tt.paths)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := bundledRootForTest(t).Verify(x509.VerifyOptions{Roots: pool}); err != nil {
				t.Fatalf("bundled root depends on a system CA file: %v", err)
			}
		})
	}
	if _, err := applicationRoots(empty); safeTransportCode(err) != "invalid_ca_bundle" {
		t.Fatal("invalid explicit CA bundle fell back to default trust")
	}
}

func TestDefaultRootsPreserveSystemTrust(t *testing.T) {
	server, ca, _ := generatedTLSServer(t, false, false)
	pool, err := defaultApplicationRoots([]string{ca + ".missing", ca})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Certificate().Verify(x509.VerifyOptions{Roots: pool, DNSName: "127.0.0.1"}); err != nil {
		t.Fatalf("canonical system trust was lost: %v", err)
	}
	if _, err := bundledRootForTest(t).Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("system pool replaced bundled bank trust: %v", err)
	}
}

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

func generatedTLSServer(t *testing.T, wrongHost, expired bool) (*httptest.Server, string, *atomic.Int64) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic test CA"}, NotBefore: now.Add(-2 * time.Hour), NotAfter: now.Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Synthetic localhost"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	if wrongHost {
		leaf.IPAddresses = nil
		leaf.DNSNames = []string{"wrong.synthetic.invalid"}
	}
	if expired {
		leaf.NotBefore = now.Add(-2 * time.Hour)
		leaf.NotAfter = now.Add(-time.Hour)
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	hits := &atomic.Int64{}
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); io.WriteString(w, "synthetic") }))
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafKey}}}
	s.StartTLS()
	t.Cleanup(s.Close)
	path := filepath.Join(testPrivateDir(t), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	return s, path, hits
}
func TestHTTPRealTLSNegativeControlsAndScopedCA(t *testing.T) {
	for _, tt := range []struct {
		name                      string
		wrongHost, expired, trust bool
		want                      string
	}{
		{"verified", false, false, true, ""}, {"untrusted", false, false, false, "tls_untrusted"}, {"wrong_hostname", true, false, true, "tls_hostname"}, {"expired", false, true, true, "tls_expired"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, ca, hits := generatedTLSServer(t, tt.wrongHost, tt.expired)
			o := TransportOptions{AllowUnready: true}
			if tt.trust {
				o.CABundle = ca
			}
			tr, err := NewHTTPTransport(unreadyBundle(), o)
			if err != nil {
				t.Fatal(err)
			}
			defer tr.Close()
			r, err := tr.Get(context.Background(), s.URL, RequestOptions{})
			if tt.want == "" {
				if err != nil || r.StatusCode != 200 || hits.Load() != 1 {
					t.Fatal("verified leaf not accepted")
				}
				return
			}
			var te *sdkErrs.TransportError
			if !errors.As(err, &te) || te.Code != tt.want || hits.Load() != 0 {
				t.Fatalf("negative TLS control: code=%v, expected %s; hits=%d", safeTransportCode(err), tt.want, hits.Load())
			}
		})
	}
	s, ca, _ := generatedTLSServer(t, false, false)
	t.Setenv("SSL_CERT_FILE", ca)
	t.Setenv("SSL_CERT_DIR", filepath.Dir(ca))
	defaultClient, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true})
	if err != nil {
		t.Fatal(err)
	}
	defer defaultClient.Close()
	if _, err := defaultClient.Get(context.Background(), s.URL, RequestOptions{}); safeTransportCode(err) != "tls_untrusted" {
		t.Fatal("ambient CA environment changed trust")
	}
	explicit, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: ca})
	if err != nil {
		t.Fatal(err)
	}
	defer explicit.Close()
	if _, err := explicit.Get(context.Background(), s.URL, RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(testPrivateDir(t), "missing.pem"), testPrivateDir(t)} {
		if _, err := NewHTTPTransport(unreadyBundle(), TransportOptions{AllowUnready: true, CABundle: path}); safeTransportCode(err) != "invalid_ca_bundle" {
			t.Fatal("invalid CA bundle accepted")
		}
	}
}
func safeTransportCode(err error) string {
	var te *sdkErrs.TransportError
	if errors.As(err, &te) {
		return te.Code
	}
	if err == nil {
		return ""
	}
	return "other"
}
