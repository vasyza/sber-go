package browser

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pw "github.com/mxschmitt/playwright-go"
	sber "github.com/vasyza/sber-go"
)

func TestPublicBootstrapRequestAllowlist(t *testing.T) {
	target := sber.PublicBootstrapURL
	for _, tt := range []struct {
		url, method, kind string
		main, allowed     bool
	}{
		{target, "GET", "document", true, true}, {target, "POST", "document", true, false},
		{"https://online.sberbank.ru/TSPD/synthetic", "GET", "document", false, true},
		{"https://online.sberbank.ru/CSAFront/synthetic.js?v=1", "GET", "script", false, true},
		{"https://online.sberbank.ru/CSAFront/authMainJson.do.js", "GET", "script", false, false},
		{"https://online.sberbank.ru/CSAFront/%61pi/v1/pin/begin", "GET", "fetch", false, false},
		{"https://online.sberbank.ru/captcha.png", "GET", "image", false, false},
		{"https://evil.invalid/static.js", "GET", "script", false, false},
		{"http://online.sberbank.ru/TSPD/synthetic.js", "GET", "script", false, false},
		{target, "GET", "document", false, false},
		{"https://online.sberbank.ru/TSPD/synthetic", "GET", "document", true, false},
	} {
		if got := publicBootstrapRequest(tt.url, tt.method, tt.kind, target, tt.main); got != tt.allowed {
			t.Fatal("unsafe/incompatible route decision")
		}
	}
}
func TestPlaywrightCookieRoundtripAndPartitionRejection(t *testing.T) {
	ss := pw.SameSiteAttribute("Strict")
	raw := pw.Cookie{Name: "synthetic", Value: "synthetic-secret", Domain: ".online.sberbank.ru", Path: "/CSAFront", Expires: 4102444800, HttpOnly: true, Secure: true, SameSite: &ss}
	record, err := cookieFromPlaywright(raw)
	if err != nil {
		t.Fatal(err)
	}
	if record.HostOnly || record.Domain != "online.sberbank.ru" || !record.HTTPOnly || *record.SameSite != "Strict" || *record.Expires != 4102444800 {
		t.Fatal("browser cookie metadata lost")
	}
	exported := cookieToPlaywright(record)
	if *exported.Domain != ".online.sberbank.ru" || *exported.SameSite != ss || *exported.Expires != raw.Expires || !*exported.HttpOnly {
		t.Fatal("seed metadata lost")
	}
	for _, edit := range []func(*pw.Cookie){func(c *pw.Cookie) { x := "https://partition.invalid"; c.PartitionKey = &x }, func(c *pw.Cookie) { c.Expires = math.NaN() }, func(c *pw.Cookie) { c.Expires = math.Inf(1) }, func(c *pw.Cookie) { c.Expires = 0 }, func(c *pw.Cookie) { s := pw.SameSiteAttribute("invalid"); c.SameSite = &s }} {
		bad := raw
		edit(&bad)
		_, err := cookieFromPlaywright(bad)
		var pin *sber.PinAuthError
		if !errors.As(err, &pin) || pin.Code != "unsupported_browser_state" {
			t.Fatal("unsupported metadata accepted")
		}
	}
	raw.Expires = -1
	raw.Domain = "online.sberbank.ru"
	raw.SameSite = nil
	r, err := cookieFromPlaywright(raw)
	if err != nil || r.Expires != nil || !r.HostOnly {
		t.Fatal("session/host-only metadata changed")
	}
}
func TestFirefoxSetupAndSafeEnvironment(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	provider, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Bootstrap(context.Background(), sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin}, "https://evil.invalid/")
	var pin *sber.PinAuthError
	if !errors.As(err, &pin) || pin.Code != "unsafe_bootstrap_request" {
		t.Fatal("unsafe target not rejected")
	}
	if err := os.Chmod(profile, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile}); err == nil {
		t.Fatal("public profile accepted")
	}
	os.Chmod(profile, 0700)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(profile, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: link}); err == nil {
		t.Fatal("profile symlink accepted")
	}
	for _, timeout := range []time.Duration{-time.Second, 121 * time.Second} {
		if _, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile, Timeout: timeout}); err == nil {
			t.Fatal("unbounded timeout accepted")
		}
	}
	env := safeBrowserEnvironment([]string{"DISPLAY=:99", "HTTPS_PROXY=synthetic", "all_proxy=synthetic", "MOZ_DISABLE_CONTENT_SANDBOX=1", "DEBUGP=1", "DEBUG=pw:api", "PWDEBUG=1", "SSLKEYLOGFILE=synthetic", "MOZ_LOG=synthetic", "PLAYWRIGHT_FIREFOX_POLICIES_JSON=synthetic"})
	if len(env) != 1 || env["DISPLAY"] != ":99" {
		t.Fatal("dangerous environment propagated")
	}
	raw, _ := json.Marshal(provider)
	if strings.Contains(string(raw), profile) {
		t.Fatal("provider serialized internal profile")
	}
}
