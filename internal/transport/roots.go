package transport

import (
	"crypto/x509"
	_ "embed"
	"io"
	"os"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

// This public root is read at build time. No external bank certificate file,
// download or system trust-store installation is needed by the native client.
// Provenance and rotation instructions are in certificates/README.md.
//
//go:embed certificates/russian_trusted_root_ca.pem
var bundledBankRoot string

func applicationRoots(explicit string) (*x509.CertPool, error) {
	if explicit != "" {
		pool := certificatePoolFromFile(explicit)
		if pool == nil {
			return nil, &sdkErrs.TransportError{Code: "invalid_ca_bundle"}
		}
		return pool, nil
	}
	// Do not consult SSL_CERT_FILE/SSL_CERT_DIR (SystemCertPool does on Unix).
	return defaultApplicationRoots([]string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/ca-bundle.pem",
		"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
		"/etc/ssl/cert.pem",
	})
}

func defaultApplicationRoots(systemPaths []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, path := range systemPaths {
		if system := certificatePoolFromFile(path); system != nil {
			pool = system
			break
		}
	}
	if !pool.AppendCertsFromPEM([]byte(bundledBankRoot)) {
		return nil, &sdkErrs.TransportError{Code: "invalid_ca_bundle"}
	}
	return pool, nil
}

func certificatePoolFromFile(path string) *x509.CertPool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	const maxBundleBytes = 16 * 1024 * 1024
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBundleBytes {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBundleBytes+1))
	if err != nil || len(data) > maxBundleBytes {
		return nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		return nil
	}
	return pool
}
