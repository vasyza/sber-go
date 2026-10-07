//go:build browser_integration

package browser

// This opt-in test launches the pinned native Go Playwright binding against
// real localhost TLS only. It uses fresh NSS profiles and synthetic trust roots.
import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"log/slog"
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
)

func TestNativeFirefoxRealTLSControls(t *testing.T) {
	driver := os.Getenv("SBER_GO_FIREFOX_DRIVER")
	binary := os.Getenv("SBER_GO_FIREFOX_EXECUTABLE")
	certutil := os.Getenv("SBER_GO_CERTUTIL")
	root := os.Getenv("SBER_GO_BROWSER_PROBE_ROOT")
	if driver == "" || binary == "" || certutil == "" || root == "" {
		t.Fatal("explicit scoped native driver, executable, certutil and probe root required")
	}
	runtime, err := pw.Run(&pw.RunOptions{DriverDirectory: driver, Verbose: false, Stdout: io.Discard, Stderr: io.Discard, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal("native driver unavailable")
	}
	defer runtime.Stop()
	for _, tc := range []struct {
		name                        string
		trusted, wrongHost, expired bool
		want                        string
	}{
		{"verified", true, false, false, ""}, {"clean_untrusted", false, false, false, "SEC_ERROR_UNKNOWN_ISSUER"},
		{"wrong_hostname", true, true, false, "SSL_ERROR_BAD_CERT_DOMAIN"}, {"expired", true, false, true, "SEC_ERROR_EXPIRED_CERTIFICATE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic NSS CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: now.Add(-2 * time.Hour), NotAfter: now.Add(2 * time.Hour)}
			caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			leafkey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "Synthetic localhost"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
			if tc.wrongHost {
				leaf.IPAddresses = nil
				leaf.DNSNames = []string{"wrong.synthetic.invalid"}
			}
			if tc.expired {
				leaf.NotAfter = now.Add(-time.Minute)
			}
			leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafkey.PublicKey, key)
			if err != nil {
				t.Fatal(err)
			}
			var hits atomic.Int64
			s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); io.WriteString(w, "synthetic verified TLS") }))
			s.Config.ErrorLog = log.New(io.Discard, "", 0)
			s.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafkey}}}
			s.StartTLS()
			defer s.Close()
			profile, err := os.MkdirTemp(root, "loopback-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(profile)
			os.Chmod(profile, 0700)
			if out, err := exec.Command(certutil, "-N", "--empty-password", "-d", "sql:"+profile).CombinedOutput(); err != nil {
				_ = out
				t.Fatal("NSS init failed")
			}
			if tc.trusted {
				pempath := filepath.Join(profile, "public-ca.pem")
				if err := os.WriteFile(pempath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(certutil, "-A", "-a", "-d", "sql:"+profile, "-n", "synthetic-public-root", "-t", "C,,", "-i", pempath).CombinedOutput(); err != nil {
					_ = out
					t.Fatal("NSS scoped trust failed")
				}
			}
			c, err := runtime.Firefox.LaunchPersistentContext(profile, pw.BrowserTypeLaunchPersistentContextOptions{ExecutablePath: pw.String(binary), Headless: pw.Bool(true), IgnoreHttpsErrors: pw.Bool(false), JavaScriptEnabled: pw.Bool(true), AcceptDownloads: pw.Bool(false), Env: safeBrowserEnvironment(os.Environ()), FirefoxUserPrefs: map[string]any{"network.proxy.type": 0, "network.http.redirection-limit": 0}, Timeout: pw.Float(10000)})
			if err != nil {
				t.Fatal("ordinary native Firefox launch failed")
			}
			defer c.Close()
			p, err := c.NewPage()
			if err != nil {
				t.Fatal(err)
			}
			r, err := p.Goto(s.URL, pw.PageGotoOptions{Timeout: pw.Float(10000), WaitUntil: pw.WaitUntilStateDomcontentloaded})
			if tc.want == "" {
				if err != nil || r.Status() != 200 || hits.Load() != 1 {
					t.Fatal("verified Firefox TLS control failed")
				}
				t.Log("TLS accepted with scoped NSS CA")
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || hits.Load() != 0 {
				t.Fatal("negative TLS result inconclusive or certificate accepted")
			}
			t.Log("TLS rejected: " + tc.want)
		})
	}
}
