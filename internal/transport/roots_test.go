package transport

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
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
