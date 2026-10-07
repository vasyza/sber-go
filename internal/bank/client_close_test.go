package bank

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientCloseCancelsReadAndWaitsForTransportUseWithoutResurrection(t *testing.T) {
	b := clientFixture(t, "close-read")
	tr := clientFake(t, b)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	tr.post = func(ctx context.Context, _ string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		return clientResponse(200, `{"success":true}`), nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); result <- e }()
	<-started
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	<-canceled
	_, closes, _ := tr.counts()
	if closes != 0 {
		t.Fatal("Close tore down in-use transport")
	}
	if _, e = c.ExportSession(); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed state not immediate")
	}
	close(release)
	if e = <-result; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("late read succeeded after Close")
	}
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
	if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("read resurrected")
	}
	if e = c.WarmUp(context.Background(), false); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("warmup resurrected")
	}
}
func TestClientCloseRacingRenewalFactoryOwnsDiscardedCandidateAndCannotPublish(t *testing.T) {
	b := clientFixture(t, "close-old")
	fresh := clientFixture(t, "close-new")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var ctxRenewal context.Context
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, Renewal: func(ctx context.Context, _ sdkSession.SessionBundle, _ sdkAuth.PINProvider, _ sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		ctxRenewal = ctx
		return fresh, nil
	}, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		close(started)
		<-release
		return next, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); result <- e }()
	select {
	case <-started:
	case e := <-result:
		t.Fatalf("factory not reached: %v", e)
	}
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	<-ctxRenewal.Done()
	close(release)
	if e = <-result; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("renewal completed after Close")
	}
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("closed renewal published profile")
	}
	a, ac, _ := old.counts()
	n, nc, _ := next.counts()
	if a != 1 || n != 0 || ac != 1 || nc != 1 {
		t.Fatal("discarded candidate leaked or used")
	}
	c.core().mu.Lock()
	retained := c.core().pinProvider
	c.core().mu.Unlock()
	if retained != nil {
		t.Fatal("closed client retained PIN provider")
	}
}
func TestClientCloseCannotTearDownBetweenSequencePOSTs(t *testing.T) {
	b := clientFixture(t, "close-scope")
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	paused := make(chan struct{})
	release := make(chan struct{})
	scopeDone := make(chan error, 1)
	go func() {
		scopeDone <- c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
			if _, e := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", false); e != nil {
				return e
			}
			close(paused)
			<-release
			_, e := send(context.Background(), "/bh-confirmation/v3/workflow2", nil, nil, "/app/test", true)
			return e
		})
	}()
	<-paused
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	<-c.core().root.Done()
	_, closes, _ := tr.counts()
	if closes != 0 {
		t.Fatal("sequence transport torn down mid-workflow")
	}
	close(release)
	if e = <-scopeDone; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed sequence sent next request")
	}
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
	calls, closes, _ := tr.counts()
	if calls != 1 || closes != 1 {
		t.Fatal("closed sequence replayed or leaked")
	}
}
func TestClientConcurrentCloseClosesEverySuccessfulOwnerOnlyOnce(t *testing.T) {
	b := clientFixture(t, "parallel-close")
	tr := clientFake(t, b)
	started := make(chan struct{})
	release := make(chan struct{})
	var attempts atomic.Int32
	tr.close = func() error {
		if attempts.Add(1) == 1 {
			close(started)
			<-release
		}
		return nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if e := c.Close(); e != nil {
			t.Error(e)
		}
	}()
	<-started
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := c.Close(); e != nil {
				t.Error(e)
			}
		}()
	}
	close(release)
	wg.Wait()
	if attempts.Load() != 1 {
		t.Fatal("parallel Close repeated successful cleanup")
	}
}
func TestClientDefaultPINFlowUsesOnlyInjectedAuthBootstrapAndClosesIt(t *testing.T) {
	b := clientFixture(t, "default-pin")
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
	providers, authBuilds := 0, 0
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers++; return "13579", nil }, ClientOptions{Transport: old, TransportOptions: sdkTransport.TransportOptions{CABundle: "synthetic-ca"}, BrowserBootstrapTimeout: 4 * time.Second, AuthOptions: sdkAuth.AuthOptions{TransportFactory: func(got sdkSession.SessionBundle, to sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		authBuilds++
		if !to.AllowUnready || to.CABundle != "synthetic-ca" {
			t.Error("auth CA/options not forwarded")
		}
		return auth, nil
	}}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var pin *sdkErrs.PinAuthError
	if !errors.As(e, &pin) || providers != 1 || authBuilds != 1 {
		t.Fatal("default auth failure not surfaced")
	}
	calls, closes, _ := auth.counts()
	if calls != 1 || closes != 1 || auth.snapshotCalls()[0].method != "GET" {
		t.Fatal("default synthetic auth attempted credential POST or leaked")
	}
}
