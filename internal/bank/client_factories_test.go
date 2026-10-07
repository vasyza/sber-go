package bank

import (
	"bytes"
	"context"
	"errors"
	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func clientPrivatePath(t *testing.T) string {
	t.Helper()
	d := testPrivateDir(t)
	if e := os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	return filepath.Join(d, "synthetic-profile.json")
}
func TestClientFactoriesKeepCAAndNeverContactBankDuringConstruction(t *testing.T) {
	creds := sdkSession.SberCredentials{UFSSession: "session-fixture", UFSToken: "token-fixture"}
	b := clientFixture(t, "factory")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	var transports []*clientFakeTransport
	o := ClientOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: "explicit-synthetic-CA", Timeout: 123}, TransportFactory: func(got sdkSession.SessionBundle, options sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		if options.CABundle != "explicit-synthetic-CA" || options.AllowUnready || options.Retry != 0 {
			t.Error("factory option mismatch")
		}
		tr := clientFake(t, got)
		transports = append(transports, tr)
		return tr, nil
	}}
	c, e := NewSberClient(b, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = NewSberClientFromCredentials(creds, sdkSession.CredentialsBundleOptions{APIBase: b.APIBase, WebBase: b.WebBase, Deviceprint: b.Deviceprint}, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	c, e = NewSberClientFromSessionFile(path, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	for _, tr := range transports {
		calls, closes, _ := tr.counts()
		if calls != 0 || closes != 1 {
			t.Fatal("constructor performed I/O")
		}
	}
	if len(transports) != 3 {
		t.Fatal("not all constructors used factory")
	}
	if _, e = NewSberClientFromCredentials(creds, sdkSession.CredentialsBundleOptions{APIBase: b.APIBase}, o); e == nil {
		t.Fatal("unpaired hosts accepted")
	}
	if _, e = NewSberClientFromCredentials(sdkSession.SberCredentials{}, sdkSession.CredentialsBundleOptions{}, o); e == nil {
		t.Fatal("missing credentials accepted")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	var insecure *sdkErrs.InsecureSessionFile
	if _, e = NewSberClientFromSessionFile(path, o); !errors.As(e, &insecure) {
		t.Fatal("nonprivate profile accepted")
	}
	c, e = NewSberClientFromSessionFile(path, o, sdkSession.SessionLoadOptions{AllowNonPrivate: true})
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
	o.TransportOptions.Retry = 1
	if _, e = NewSberClient(b, o); e == nil {
		t.Fatal("retry enabled")
	}
	// The real DEFAULT transport is constructed and closed but no Get/Post is called.
	c, e = NewSberClientFromCredentials(creds, sdkSession.CredentialsBundleOptions{}, ClientOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.ExportCredentials(); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Existing closed lifetime must not become usable via a new operation.
	if _, e = c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed lifetime reused")
	}
}

func TestClientDefaultPINCanceledCleanupKeepsBothOwnershipAndCancellation(t *testing.T) {
	b := clientFixture(t, "pin-canceled-cleanup")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	auth := clientFake(t, b)
	auth.get = func(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		cancel()
		return nil, context.Canceled
	}
	var attempts atomic.Int32
	auth.close = func() error {
		if attempts.Add(1) == 1 {
			return errors.New("synthetic auth close failure")
		}
		return nil
	}
	c, e := NewSberClientFromPINProfile(ctx, path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: sdkAuth.AuthOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return auth, nil
	}}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil)
	var cleanup interface{ Close() error }
	if !errors.As(e, &cleanup) || !errors.Is(e, context.Canceled) {
		t.Fatal("canceled cleanup ownership was flattened")
	}
	if e = cleanup.Close(); e != nil {
		t.Fatal(e)
	}
}

func TestClientDefaultPINCleanupFailurePreservesRetryableOwnershipAndPrimaryError(t *testing.T) {
	b := clientFixture(t, "pin-cleanup")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	auth := clientFake(t, b)
	auth.get = func(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(403, `synthetic bootstrap rejection`), nil
	}
	var attempts atomic.Int32
	auth.close = func() error {
		if attempts.Add(1) == 1 {
			return errors.New("synthetic auth close failure")
		}
		return nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: sdkAuth.AuthOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return auth, nil
	}}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var cleanup interface{ Close() error }
	var pin *sdkErrs.PinAuthError
	if !errors.As(e, &cleanup) || !errors.As(e, &pin) {
		t.Fatal("auth cleanup failure/primary challenge lost")
	}
	if e = cleanup.Close(); e != nil {
		t.Fatal(e)
	}
	if e = cleanup.Close(); e != nil {
		t.Fatal(e)
	}
	calls, closes, _ := auth.counts()
	if calls != 1 || closes != 2 {
		t.Fatal("auth attempt replayed or failed cleanup lost")
	}
}

func TestClientCycle3AuthGuardGatesEveryFreshOrAliasedGeneration(t *testing.T) {
	b := clientFixture(t, "cycle3-dynamic-auth")
	old, auth, next := clientFake(t, b), clientFake(t, b), clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: old})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	calls := 0
	guarded := clientGuardAuthOptions(sdkAuth.AuthOptions{Transport: clientCycle3ValueOwner{auth, []byte{1}}, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		calls++
		if calls == 1 {
			return old, errors.New("synthetic borrowed factory failure")
		}
		return clientCycle3ValueOwner{next, []byte{2}}, nil
	}}, c.claimAuthTransport)
	first, err := guarded.TransportFactory(b, sdkTransport.TransportOptions{})
	if err != nil || first == nil || first.CookieJar() != auth.jar || first.(ClientTransportOwner).ClientTransportOwner() != auth {
		t.Fatal("fresh explicit value auth owner not supported")
	}
	// This also exercises transparently delegated generic POST, without auth
	// credentials, endpoint discovery, financial sends, or network transport.
	payload := map[string]any{"synthetic": "delegation"}
	if _, err = first.Post(context.Background(), "https://example.invalid/synthetic", payload, sdkTransport.RequestOptions{}); err != nil {
		t.Fatal(err)
	}
	got := auth.snapshotCalls()
	if len(got) != 1 || !reflect.DeepEqual(got[0].body, payload) {
		t.Fatal("auth adapter altered generic POST")
	}
	if rejected, err := guarded.TransportFactory(b, sdkTransport.TransportOptions{}); rejected != nil || err == nil {
		t.Fatal("later auth generation took business owner")
	}
	if _, closes, _ := old.counts(); closes != 0 {
		t.Fatal("factory error closed borrowed business owner")
	}
	second, err := guarded.TransportFactory(b, sdkTransport.TransportOptions{})
	if err != nil || second.CookieJar() != next.jar || calls != 2 {
		t.Fatal("direct injection reused instead of moving to original factory")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tr := range []*clientFakeTransport{old, auth, next} {
		if _, closes, _ := tr.counts(); closes != 1 {
			t.Fatal("auth/client cleanup duplicated a closing owner")
		}
	}
}

func TestClientCycle3ConcurrentAuthClaimsAcquireAnAliasOnlyOnce(t *testing.T) {
	b := clientFixture(t, "cycle3-parallel-auth")
	old, candidate := clientFake(t, b), clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: old})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	start := make(chan struct{})
	results := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := c.claimAuthTransport(clientCycle3ValueOwner{candidate, []byte{byte(i)}})
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("concurrent aliases acquired multiple closing owners")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, closes, _ := candidate.counts(); closes != 1 {
		t.Fatal("concurrent claimed owner close duplicated")
	}
}

func TestClientCycle3DefaultAuthRejectsBorrowedClosingOwners(t *testing.T) {
	for _, readiness := range []string{"ready", "unready"} {
		for _, route := range []string{"direct", "factory", "factory-error", "value-owner", "runtime-noncomparable", "inherited-business-factory"} {
			t.Run(readiness+"/"+route, func(t *testing.T) {
				b := clientFixture(t, "cycle3-auth-reserved")
				if readiness == "unready" {
					b.Cookies = nil
				}
				path := clientPrivatePath(t)
				if err := b.Save(path); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				old := clientFake(t, b)
				old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return clientResponse(401, `{}`), nil
				}
				old.get = func(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return clientResponse(403, `synthetic bootstrap rejection`), nil
				}
				opts := ClientOptions{Transport: old}
				var candidate sdkTransport.Transport = old
				if route == "value-owner" {
					candidate = clientCycle3ValueOwner{old, []byte{1}}
				}
				if route == "runtime-noncomparable" {
					candidate = clientCycle3InterfaceValue{old, []byte{1}}
				}
				if route == "direct" {
					opts.AuthOptions.Transport = candidate
				} else if route == "inherited-business-factory" {
					opts.TransportFactory = func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
						return candidate, nil
					}
				} else {
					opts.AuthOptions.TransportFactory = func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
						if route == "factory-error" {
							return candidate, errors.New("synthetic-private-factory-canary")
						}
						return candidate, nil
					}
				}
				c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, opts)
				if readiness == "ready" {
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					_, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
				} else if c != nil {
					defer c.Close()
					t.Error("unready auth ownership accepted")
				}
				var transport *sdkErrs.TransportError
				if !errors.As(err, &transport) || transport.Code != "reused_transport" {
					t.Error("borrowed auth closing owner was not rejected")
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(before, after) {
					t.Error("borrowed auth published a profile")
				}
				if _, closes, _ := old.counts(); closes != 0 {
					t.Error("borrowed owner closed on auth failure")
				}
				posts, gets := 0, 0
				for _, call := range old.snapshotCalls() {
					if call.method == "POST" {
						posts++
					} else {
						gets++
					}
				}
				expectedPosts := 0
				if readiness == "ready" {
					expectedPosts = 1
				}
				if posts != expectedPosts || gets != 0 {
					t.Error("rejected auth owner reached bootstrap or credential/business replay")
				}
				if c != nil {
					if err = c.Close(); err != nil {
						t.Fatal(err)
					}
					if _, closes, _ := old.counts(); closes != 1 {
						t.Error("borrowed business owner double closed")
					}
				}
			})
		}
	}
}

func TestClientCycle3FreshAuthFactoryErrorsKeepRetryableCleanup(t *testing.T) {
	for _, route := range []string{"factory-error", "nil-jar"} {
		t.Run(route, func(t *testing.T) {
			b := clientFixture(t, "cycle3-fresh-auth")
			path := clientPrivatePath(t)
			if err := b.Save(path); err != nil {
				t.Fatal(err)
			}
			old, candidate := clientFake(t, b), clientFake(t, b)
			old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(401, `{}`), nil
			}
			if route == "nil-jar" {
				candidate.jar = nil
			}
			var attempts atomic.Int32
			candidate.close = func() error {
				if attempts.Add(1) == 1 {
					return errors.New("synthetic close failure")
				}
				return nil
			}
			c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: sdkAuth.AuthOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				if route == "factory-error" {
					return candidate, errors.New("synthetic factory failure")
				}
				return candidate, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			var cleanup *ClientCleanupError
			if !errors.As(err, &cleanup) || attempts.Load() != 1 {
				t.Fatal("fresh failed auth construction leaked retryable closing ownership")
			}
			if err = cleanup.Close(); err != nil {
				t.Fatal(err)
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			if attempts.Load() != 2 {
				t.Fatal("auth cleanup retried or double closed")
			}
			if n, _, _ := candidate.counts(); n != 0 {
				t.Fatal("invalid auth candidate performed I/O")
			}
		})
	}
}

func TestClientCycle3CustomRenewalRetainsExplicitOptions(t *testing.T) {
	b := clientFixture(t, "cycle3-custom-auth")
	path := clientPrivatePath(t)
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	old, next, unused := clientFake(t, b), clientFake(t, b), clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	called := false
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, nil
	}, AuthOptions: sdkAuth.AuthOptions{Transport: unused}, Renewal: func(_ context.Context, b sdkSession.SessionBundle, _ sdkAuth.PINProvider, ao sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		called = true
		if ao.Transport != unused {
			t.Error("custom renewal injection replaced")
		}
		return b, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); err != nil || !called {
		t.Fatal("custom renewal disabled", err)
	}
	if n, closes, _ := unused.counts(); n != 0 || closes != 0 {
		t.Fatal("client claimed unused custom auth transport")
	}
}

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
