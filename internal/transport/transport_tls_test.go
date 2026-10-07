package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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
