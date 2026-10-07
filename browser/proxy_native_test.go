//go:build browser_integration

package browser

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pw "github.com/mxschmitt/playwright-go"
	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/testproxy"
)

func nativeProxyCertificate(t *testing.T, name string) (*tls.Config, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic " + name + " test CA"}, IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafKey}}}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func TestNativeFirefoxProxyConnectionsAndTLS(t *testing.T) {
	driver, binary, certutil, root := os.Getenv("SBER_GO_FIREFOX_DRIVER"), os.Getenv("SBER_GO_FIREFOX_EXECUTABLE"), os.Getenv("SBER_GO_CERTUTIL"), os.Getenv("SBER_GO_BROWSER_PROBE_ROOT")
	if driver == "" || binary == "" || certutil == "" || root == "" {
		t.Fatal("explicit scoped native browser paths are required")
	}
	for _, tc := range []struct {
		name, scheme                                       string
		auth, rejectLogin, rejectOriginTLS, rejectProxyTLS bool
	}{
		{"http", "http", false, false, false, false}, {"http-auth", "http", true, false, false, false},
		{"https-auth", "https", true, false, false, false}, {"socks5", "socks5", false, false, false, false},
		{"socks5-auth", "socks5", true, false, false, false}, {"rejected-login", "http", true, true, false, false},
		{"untrusted-origin", "socks5", true, false, true, false}, {"untrusted-https-proxy", "https", true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := os.MkdirTemp(root, "proxy-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(profile)
			if err := os.Chmod(profile, 0700); err != nil {
				t.Fatal(err)
			}
			if _, err := exec.Command(certutil, "-N", "--empty-password", "-d", "sql:"+profile).CombinedOutput(); err != nil {
				t.Fatal("synthetic NSS initialization failed")
			}
			trust := func(name string, ca []byte) {
				t.Helper()
				path := filepath.Join(profile, name+".pem")
				if err := os.WriteFile(path, ca, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := exec.Command(certutil, "-A", "-a", "-d", "sql:"+profile, "-n", name, "-t", "C,,", "-i", path).CombinedOutput(); err != nil {
					t.Fatal("synthetic NSS CA import failed")
				}
			}
			var hits atomic.Int32
			originTLS, originCA := nativeProxyCertificate(t, "origin")
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Proxy-Authorization") != "" {
					t.Error("proxy credentials reached origin")
				}
				hits.Add(1)
				w.Header().Set("Content-Type", "text/plain")
				io.WriteString(w, "synthetic proxy page")
			}))
			origin.TLS = originTLS
			origin.Config.ErrorLog = log.New(io.Discard, "", 0)
			origin.StartTLS()
			defer origin.Close()
			if !tc.rejectOriginTLS {
				trust("origin-ca", originCA)
			}
			username, password := "", ""
			if tc.auth {
				username, password = "synthetic-user", "synthetic-password"
			}
			var address string
			var stats *testproxy.Stats
			if tc.scheme == "socks5" {
				host, counters := testproxy.SOCKS5(t, username, password, strings.TrimPrefix(origin.URL, "https://"))
				address, stats = "socks5://"+host, counters
			} else if tc.scheme == "https" {
				proxyTLS, proxyCA := nativeProxyCertificate(t, "proxy")
				server, counters := testproxy.HTTPWithTLS(t, proxyTLS, username, password, strings.TrimPrefix(origin.URL, "https://"))
				address, stats = server.URL, counters
				if !tc.rejectProxyTLS {
					trust("proxy-ca", proxyCA)
				}
			} else {
				server, counters := testproxy.HTTP(t, false, username, password, strings.TrimPrefix(origin.URL, "https://"))
				address, stats = server.URL, counters
			}
			if tc.rejectLogin {
				password = "wrong-synthetic-password"
			}
			provider, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile, DriverDir: driver, FirefoxExecutable: binary, Timeout: 10 * time.Second,
				Proxy: sber.ProxyOptions{URL: address, Username: username, Password: password}})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			options, closeProxy, err := provider.browserLaunchOptions(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer closeProxy()
			browser, cleanup, err := provider.launch(options)
			if err != nil {
				t.Fatal("native proxy browser launch failed")
			}
			defer cleanup()
			defer browser.Close()
			page, err := browser.NewPage()
			if err != nil {
				t.Fatal("native proxy page creation failed")
			}
			response, err := page.Goto(origin.URL, pw.PageGotoOptions{Timeout: pw.Float(5000), WaitUntil: pw.WaitUntilStateDomcontentloaded})
			if tc.rejectLogin || tc.rejectOriginTLS || tc.rejectProxyTLS {
				if err == nil || hits.Load() != 0 {
					t.Fatal("browser bypassed a rejected proxy or TLS certificate")
				}
				return
			}
			if err != nil || response == nil || response.Status() != 200 || hits.Load() != 1 || stats.Authorized.Load() < 1 {
				t.Fatal("native browser proxy connection failed")
			}
		})
	}
}
