package sber

import (
	"context"
	"errors"
	"testing"
)

func TestAuthUnadoptedReplacementCleanupCanRetry(t *testing.T) {
	original := newAuthScript(t)
	replacement := &authFlakyClose{newAuthScript(t)}
	started, release := make(chan struct{}), make(chan struct{})
	provider := BrowserBootstrapFunc(func(context.Context, SessionBundle, string) (BrowserBootstrapResult, error) {
		return BrowserBootstrapResult{HTML: authHTML(false), URL: PublicBootstrapURL, Browser: BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "synthetic"}}}}, nil
	})
	a, e := NewPINAuth(authBundle(t), AuthOptions{Transport: original, BrowserBootstrap: provider, BrowserFirst: true, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) {
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
	if e := <-done; !errors.Is(e, ErrClosed) {
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
	provider := BrowserBootstrapFunc(func(context.Context, SessionBundle, string) (BrowserBootstrapResult, error) {
		return BrowserBootstrapResult{HTML: authHTML(false), URL: PublicBootstrapURL, Browser: BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "synthetic"}}}}, nil
	})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: original, BrowserBootstrap: provider, BrowserFirst: true, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) {
		return replacement, errors.New("private factory URL")
	}})
	_, e := a.LoadConfig(context.Background())
	authErrorCode(t, e, "browser_bootstrap_failed")
	if e = a.Close(); e != nil || replacement.closes != 2 {
		t.Fatal("failed factory leaked a returned transport")
	}
}
