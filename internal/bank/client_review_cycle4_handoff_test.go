package bank

import (
	"context"
	"errors"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// This is client-helper composition, not PINAuth construction/login. All I/O
// methods belong to local synthetic fixtures; credential POST is forbidden.
func TestClientCycle4InitialOwnershipHandoffMatrix(t *testing.T) {
	for _, mode := range []string{"shared-factory", "direct-auth", "stable-value-alias", "alias-with-error", "fresh", "fresh-factory-error", "fresh-nil-jar"} {
		t.Run(mode, func(t *testing.T) {
			b := i3Bundle(t, "handoff-matrix")
			authRaw, next := i3NewTransport(t, b), i3NewTransport(t, b)
			var business sdkTransport.Transport = authRaw
			if mode == "stable-value-alias" {
				business = i3IdentityValue{authRaw, []byte{1}}
			}
			if mode == "fresh" || mode == "fresh-factory-error" || mode == "fresh-nil-jar" {
				business = next
			}
			factoryCalls := 0
			o, e := clientNormalizeOptions(ClientOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				factoryCalls++
				if mode == "alias-with-error" || mode == "fresh-factory-error" {
					return business, errors.New("synthetic private factory failure")
				}
				return business, nil
			}})
			if e != nil {
				t.Fatal(e)
			}
			if mode == "direct-auth" {
				o.AuthOptions.Transport = authRaw
			}
			o.AuthOptions.TransportFactory = func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return authRaw, nil
			}
			scope := clientNewInitialOwners(o.Transport)
			guarded := clientGuardAuthOptions(o.AuthOptions, scope.claim)
			auth, e := guarded.TransportFactory(b, sdkTransport.TransportOptions{})
			if e != nil {
				t.Fatal(e)
			}
			if e = auth.Close(); e != nil {
				t.Fatal(e)
			}
			if mode == "fresh-nil-jar" {
				next.jar = nil
			}
			c, e := clientNewSberClient(b, o, scope)
			if mode == "fresh" {
				if e != nil {
					t.Fatal(e)
				}
				defer c.Close()
				if len(c.core().owned) != 2 {
					t.Error("initial auth history not transferred into business lifetime")
				}
				if _, e := c.claimAuthTransport(i3IdentityValue{authRaw, []byte{2}}); e == nil {
					t.Error("later auth acquired closed initial owner")
				}
				if _, e := guarded.TransportFactory(b, sdkTransport.TransportOptions{}); e == nil {
					t.Error("initial auth capability remained open after adoption")
				}
				if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e != nil {
					t.Fatal(e)
				}
				if e = c.Close(); e != nil {
					t.Fatal(e)
				}
				if _, _, n, _ := next.counts(); n != 1 {
					t.Error("fresh business owner not closed exactly once")
				}
			} else {
				if c != nil || e == nil {
					t.Fatal("invalid handoff accepted")
				}
				if mode != "fresh-factory-error" && mode != "fresh-nil-jar" {
					var te *sdkErrs.TransportError
					if !errors.As(e, &te) || te.Code != "reused_transport" {
						t.Error("closed owner did not fail ownership boundary")
					}
				}
			}
			if n, g, closes, dead := authRaw.counts(); n != 0 || g != 0 || closes != 1 || dead != 0 {
				t.Error("initial owner read/closed again during handoff")
			}
			if factoryCalls != 1 {
				t.Error("first business factory retried")
			}
			if mode == "fresh-factory-error" || mode == "fresh-nil-jar" {
				if n, g, closes, dead := next.counts(); n != 0 || g != 0 || closes != 1 || dead != 0 {
					t.Error("fresh invalid candidate cleanup lost")
				}
			}
		})
	}
}
func TestClientCycle4InitialReservationsRemainBorrowed(t *testing.T) {
	b := i3Bundle(t, "reserved")
	borrowed := i3NewTransport(t, b)
	scope := clientNewInitialOwners(borrowed)
	guarded := clientGuardAuthOptions(sdkAuth.AuthOptions{Transport: borrowed}, scope.claim)
	if tr, e := guarded.TransportFactory(b, sdkTransport.TransportOptions{}); tr != nil || e == nil {
		t.Error("reserved injection acquired by auth helper")
	}
	o := ClientOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return i3IdentityValue{borrowed, []byte{2}}, errors.New("synthetic private")
	}}
	if c, e := clientNewSberClient(b, o, scope); c != nil || e == nil {
		t.Error("reserved owner adopted after clearing direct injection")
	}
	if n, g, closes, dead := borrowed.counts(); n != 0 || g != 0 || closes != 0 || dead != 0 {
		t.Error("borrowed candidate acquired or discarded")
	}
}
func TestClientCycle4InitialFreshDiscardKeepsRetryableCleanup(t *testing.T) {
	b := i3Bundle(t, "fresh-cleanup")
	authRaw, next := i3NewTransport(t, b), i3NewTransport(t, b)
	next.closeHook = func(n int) error {
		if n == 1 {
			return errors.New("synthetic private close")
		}
		return nil
	}
	scope := clientNewInitialOwners(nil)
	auth := clientGuardAuthOptions(sdkAuth.AuthOptions{Transport: authRaw}, scope.claim)
	tr, e := auth.TransportFactory(b, sdkTransport.TransportOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Close(); e != nil {
		t.Fatal(e)
	}
	o := ClientOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, context.Canceled
	}}
	c, e := clientNewSberClient(b, o, scope)
	var pending *ClientCleanupError
	if c != nil || !errors.As(e, &pending) || !errors.Is(e, context.Canceled) {
		t.Fatal("fresh business error lost context or cleanup ownership")
	}
	if e = pending.Close(); e != nil {
		t.Fatal(e)
	}
	if e = pending.Close(); e != nil {
		t.Fatal(e)
	}
	if _, _, closes, _ := next.counts(); closes != 2 {
		t.Error("fresh discard cleanup retry count wrong")
	}
	if _, _, closes, _ := authRaw.counts(); closes != 1 {
		t.Error("initial closed auth double cleanup")
	}
}
