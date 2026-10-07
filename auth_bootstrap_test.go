package sber

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestAuthBrowserAtomicHandoff(t *testing.T) {
	for _, first := range []bool{false, true} {
		t.Run(map[bool]string{false: "recognized-fallback", true: "explicit-first"}[first], func(t *testing.T) {
			original := newAuthScript(t)
			u, _ := url.Parse(PublicBootstrapURL)
			_ = original.jar.ApplySetCookie(u, []string{"observed=pre-probe; Path=/; Secure"})
			if !first {
				original.steps = []authStep{{method: "GET", target: PublicBootstrapURL, response: authPage(`<script src="/TSPD/a"></script>enable javascript`), run: func(context.Context) {
					_ = original.jar.ApplySetCookie(u, []string{"observed=probe-rotation; Path=/; Secure"})
				}}}
			}
			replacement := newAuthScript(t)
			called := 0
			provider := BrowserBootstrapFunc(func(ctx context.Context, b SessionBundle, target string) (BrowserBootstrapResult, error) {
				called++
				if target != PublicBootstrapURL || len(b.Cookies) != 1 || b.Cookies[0].Value != "pre-probe" {
					t.Error("bootstrap did not freeze pre-probe jar")
				}
				return BrowserBootstrapResult{HTML: authHTML(false), URL: PublicBootstrapURL, Browser: BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "observed-synthetic-ua"}}}, Cookies: []CookieRecord{{Name: "rendered", Value: "synthetic", Domain: AuthCookieDomain, Path: "/", Secure: true}}}, nil
			})
			factory := func(b SessionBundle, o TransportOptions) (Transport, error) {
				if o.CABundle != "synthetic-ca-path" || !o.AllowUnready || o.Retry != 0 || b.Browser.AsMap()["user-agent"] != "observed-synthetic-ua" {
					t.Error("transport options/identity lost")
				}
				replacement.jar, _ = NewCookieJar(b.Cookies)
				return replacement, nil
			}
			a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: original, TransportFactory: factory, TransportOptions: TransportOptions{CABundle: "synthetic-ca-path"}, BrowserBootstrap: provider, BrowserFirst: first})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = a.LoadConfig(context.Background()); err != nil {
				t.Fatal(err)
			}
			b, err := a.ExportSession()
			if err != nil || len(b.Cookies) != 1 || b.Cookies[0].Name != "rendered" || called != 1 || original.closes != 1 || replacement.calls != 0 {
				t.Fatal("handoff not atomic or performed second GET")
			}
			_ = a.Close()
		})
	}
}
func TestAuthMalformedPageNeverAutoFallsBack(t *testing.T) {
	called := false
	s := newAuthScript(t, authStep{method: "GET", target: PublicBootstrapURL, response: authPage("malformed but not recognized")})
	provider := BrowserBootstrapFunc(func(context.Context, SessionBundle, string) (BrowserBootstrapResult, error) {
		called = true
		return BrowserBootstrapResult{}, nil
	})
	a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s, BrowserBootstrap: provider})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.LoadConfig(context.Background())
	authErrorCode(t, err, "invalid_frontend_config")
	if called {
		t.Fatal("malformed fallback broadened")
	}
	_ = a.Close()
}
func TestAuthBrowserCancellationAndCloseRejectAdoption(t *testing.T) {
	for _, closing := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "close"}[closing], func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			original := newAuthScript(t)
			replacement := newAuthScript(t)
			factoryCalls := 0
			provider := BrowserBootstrapFunc(func(ctx context.Context, b SessionBundle, target string) (BrowserBootstrapResult, error) {
				close(started)
				<-release
				return BrowserBootstrapResult{HTML: authHTML(false), URL: PublicBootstrapURL, Browser: BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "synthetic"}}}}, nil
			})
			a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: original, BrowserFirst: true, BrowserBootstrap: provider, BrowserBootstrapTimeout: time.Second, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { factoryCalls++; return replacement, nil }})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { _, e := a.LoadConfig(ctx); done <- e }()
			<-started
			if closing {
				_ = a.Close()
			} else {
				cancel()
			}
			close(release)
			e := <-done
			if closing && !errors.Is(e, ErrClosed) || !closing && !errors.Is(e, context.Canceled) {
				t.Fatal("lost lifetime error")
			}
			if factoryCalls != 0 {
				t.Fatal("canceled bootstrap constructed/adopted transport")
			}
			_ = a.Close()
		})
	}
}
