package auth

import (
	"context"
	"errors"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestAuthUnadoptedReplacementCleanupCanRetry(t *testing.T) {
	original := newAuthScript(t)
	replacement := &authFlakyClose{newAuthScript(t)}
	started, release := make(chan struct{}), make(chan struct{})
	provider := sdkTransport.BrowserBootstrapFunc(func(context.Context, sdkSession.SessionBundle, string) (sdkTransport.BrowserBootstrapResult, error) {
		return sdkTransport.BrowserBootstrapResult{HTML: authHTML(false), URL: sdkTransport.PublicBootstrapURL, Browser: sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "user-agent", Value: "synthetic"}}}}, nil
	})
	a, e := NewPINAuth(authBundle(t), AuthOptions{Transport: original, BrowserBootstrap: provider, BrowserFirst: true, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		close(started)
		<-release
		return replacement, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := a.LoadConfig(context.Background()); done <- e }()
	<-started
	_ = a.Close()
	close(release)
	if e := <-done; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed factory result adopted")
	}
	if a.transport != original || a.config != nil {
		t.Fatal("closed factory changed active state")
	}
	if e := a.Close(); e != nil || replacement.closes != 2 {
		t.Fatal("unadopted failed cleanup was lost")
	}
}
func TestAuthFailedFactoryReturnedTransportOwnedForCleanup(t *testing.T) {
	original := newAuthScript(t)
	replacement := &authFlakyClose{newAuthScript(t)}
	provider := sdkTransport.BrowserBootstrapFunc(func(context.Context, sdkSession.SessionBundle, string) (sdkTransport.BrowserBootstrapResult, error) {
		return sdkTransport.BrowserBootstrapResult{HTML: authHTML(false), URL: sdkTransport.PublicBootstrapURL, Browser: sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "user-agent", Value: "synthetic"}}}}, nil
	})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: original, BrowserBootstrap: provider, BrowserFirst: true, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return replacement, errors.New("private factory URL")
	}})
	_, e := a.LoadConfig(context.Background())
	authErrorCode(t, e, "browser_bootstrap_failed")
	if e = a.Close(); e != nil || replacement.closes != 2 {
		t.Fatal("failed factory leaked a returned transport")
	}
}
