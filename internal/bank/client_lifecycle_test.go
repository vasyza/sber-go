package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientPINFactoryCancellationCannotReturnALiveClient(t *testing.T) {
	b := clientFixture(t, "cancel-factory")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	tr := clientFake(t, b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, e := NewSberClientFromPINProfile(ctx, path, func(context.Context) (string, error) { t.Error("ready profile logged in"); return "", nil }, ClientOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		cancel()
		return tr, nil
	}})
	if c != nil {
		defer c.Close()
	}
	if !errors.Is(e, context.Canceled) {
		t.Fatal("canceled constructor returned success")
	}
	calls, closes, _ := tr.counts()
	if calls != 0 || closes != 1 {
		t.Fatal("canceled constructor leaked transport")
	}
}
func TestClientMutationSequenceRetainsCancellationAndUncertainty(t *testing.T) {
	b := clientFixture(t, "scope-cancel")
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return nil, context.Canceled
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	e = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		_, e := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", false)
		return e
	})
	var uncertain *sdkErrs.MutationUncertain
	if !errors.Is(e, context.Canceled) || !errors.As(e, &uncertain) {
		t.Fatal("scope collapsed after-send uncertainty")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e = c.MutationSequence(ctx, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		cancel()
		return nil
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatal("canceled scope silently succeeded")
	}
}

func TestClientFailedConstructorReturnsRetryableCleanupOwnership(t *testing.T) {
	for _, mode := range []string{"factory-error", "missing-jar", "canceled-pin"} {
		t.Run(mode, func(t *testing.T) {
			b := clientFixture(t, "construct-cleanup")
			tr := clientFake(t, b)
			var attempts atomic.Int32
			tr.close = func() error {
				if attempts.Add(1) == 1 {
					return errors.New("synthetic secret close failure")
				}
				return nil
			}
			var c *SberClient
			var e error
			switch mode {
			case "factory-error":
				c, e = NewSberClient(b, ClientOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					return tr, errors.New("synthetic secret factory failure")
				}})
			case "missing-jar":
				tr.jar = nil
				c, e = NewSberClient(b, ClientOptions{Transport: tr})
			case "canceled-pin":
				path := clientPrivatePath(t)
				if err := b.Save(path); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				c, e = NewSberClientFromPINProfile(ctx, path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					cancel()
					return tr, nil
				}})
			}
			if c != nil || e == nil {
				t.Fatal("failed constructor returned ready client")
			}
			var cleanup interface{ Close() error }
			if !errors.As(e, &cleanup) {
				t.Fatal("failed cleanup ownership discarded")
			}
			if mode == "canceled-pin" && !errors.Is(e, context.Canceled) {
				t.Fatal("cleanup error hid cancellation")
			}
			for _, text := range []string{e.Error(), fmt.Sprintf("%#v", e)} {
				if strings.Contains(text, "synthetic secret") {
					t.Fatal("unsafe cleanup diagnostic")
				}
			}
			data, err := json.Marshal(e)
			if err != nil || strings.Contains(string(data), "synthetic secret") {
				t.Fatal("unsafe cleanup JSON")
			}
			if e = cleanup.Close(); e != nil {
				t.Fatal(e)
			}
			if e = cleanup.Close(); e != nil {
				t.Fatal(e)
			}
			if attempts.Load() != 2 {
				t.Fatal("cleanup retry lost or repeated")
			}
		})
	}
}

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
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers++; return "13579", nil }, ClientOptions{Transport: old, TransportOptions: sdkTransport.TransportOptions{CABundle: "synthetic-ca"}, AuthOptions: sdkAuth.AuthOptions{TransportFactory: func(got sdkSession.SessionBundle, to sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
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

func TestClientPersistentRejectedEpochCoalescesIncludingPostSwapArrivals(t *testing.T) {
	b := clientFixture(t, "epoch-old")
	fresh := clientFixture(t, "epoch-new")
	latest := clientFixture(t, "epoch-latest")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	last := clientFake(t, latest)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	retryStarted := make(chan struct{})
	releaseRetry := make(chan struct{})
	var retryCount atomic.Int32
	next.post = func(ctx context.Context, _ string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if retryCount.Add(1) == 1 {
			close(retryStarted)
			select {
			case <-releaseRetry:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return clientResponse(401, `{}`), nil
	}
	var renewals, builds atomic.Int32
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old,
		Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
			if renewals.Add(1) == 1 {
				return fresh, nil
			}
			return latest, nil
		},
		TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			if builds.Add(1) == 1 {
				return next, nil
			}
			return last, nil
		},
	})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	results := make(chan error, 10)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); results <- e }()
	select {
	case <-retryStarted:
	case e := <-results:
		t.Fatalf("retry not started: %v", e)
	}
	// These all arrive AFTER the replacement is committed, while its retry is
	// pending. Advancing epoch at swap instead of at completion causes a storm.
	for i := 0; i < 9; i++ {
		ctx, queued := clientQueue()
		go func() { _, e := c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); results <- e }()
		<-queued
	}
	close(releaseRetry)
	for i := 0; i < 10; i++ {
		e := <-results
		var expired *sdkErrs.AuthenticationExpired
		if !errors.As(e, &expired) {
			t.Fatal("persistent rejection hidden")
		}
	}
	if renewals.Load() != 1 || builds.Load() != 1 {
		t.Fatal("persistent rejected epoch caused repeated login")
	}
	if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e != nil {
		t.Fatal("later epoch could not renew")
	}
	if renewals.Load() != 2 || builds.Load() != 2 {
		t.Fatal("later generation never got its own attempt")
	}
	oldCalls, _, _ := old.counts()
	nextCalls, _, _ := next.counts()
	lastCalls, _, _ := last.counts()
	if oldCalls != 1 || nextCalls != 11 || lastCalls != 1 {
		t.Fatal("wrong read replay bounds")
	}
}
func TestClientFailedRenewalEpochAllowsLaterIndependentAttempt(t *testing.T) {
	b := clientFixture(t, "failed-epoch")
	fresh := clientFixture(t, "retry-epoch")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var providers, renewals atomic.Int32
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) {
		if providers.Add(1) == 1 {
			close(started)
			<-release
			return "", errors.New("provider failure")
		}
		return "13579", nil
	}, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, nil
	}, Renewal: func(ctx context.Context, _ sdkSession.SessionBundle, p sdkAuth.PINProvider, _ sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		if _, e := p(ctx); e != nil {
			return sdkSession.SessionBundle{}, e
		}
		renewals.Add(1)
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	results := make(chan error, 5)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); results <- e }()
	select {
	case <-started:
	case e := <-results:
		t.Fatalf("provider not called: %v", e)
	}
	for i := 0; i < 4; i++ {
		ctx, queued := clientQueue()
		go func() { _, e := c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); results <- e }()
		<-queued
	}
	close(release)
	for i := 0; i < 5; i++ {
		if e := <-results; e == nil {
			t.Fatal("rejected generation succeeded")
		}
	}
	if providers.Load() != 1 || renewals.Load() != 0 {
		t.Fatal("failed refresh repeated for waiters")
	}
	if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e != nil {
		t.Fatal(e)
	}
	if providers.Load() != 2 || renewals.Load() != 1 {
		t.Fatal("later attempt disabled")
	}
}
func TestClientRetiredCleanupDoesNotAbortReadsAndIsRetriedOnlyByClose(t *testing.T) {
	b := clientFixture(t, "cleanup-old")
	fresh := clientFixture(t, "cleanup-new")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	var closes atomic.Int32
	old.close = func() error {
		if closes.Add(1) < 3 {
			return errors.New("retired cleanup failed")
		}
		return nil
	}
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for i := 0; i < 3; i++ {
		if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e != nil {
			t.Fatal("retired cleanup broke committed read")
		}
	}
	if closes.Load() != 1 {
		t.Fatal("read retried retired cleanup")
	}
	if e = c.Close(); e == nil {
		t.Fatal("failed Close hidden")
	}
	if closes.Load() != 2 {
		t.Fatal("Close did not retry cleanup")
	}
	if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("cleanup failure resurrected client")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	_, nextCloses, _ := next.counts()
	if closes.Load() != 3 || nextCloses != 1 {
		t.Fatal("successful cleanup repeated")
	}
}
func TestClientPINWarmUpUsesTheSameBoundedReadRenewal(t *testing.T) {
	b := clientFixture(t, "warm-old")
	fresh := clientFixture(t, "warm-new")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(403, `{}`), nil
	}
	var renewals atomic.Int32
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		renewals.Add(1)
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	a, _, _ := old.counts()
	n, _, _ := next.counts()
	if a != 1 || n != 1 || renewals.Load() != 1 {
		t.Fatal("warmup renewal or debounce incorrect")
	}
}

// All transport fixtures are generated, in-process, and incapable of I/O.
type clientCall struct {
	method, target string
	body           map[string]any
	options        sdkTransport.RequestOptions
}
type clientFakeTransport struct {
	mu                 sync.Mutex
	jar                *sdkSession.CookieJar
	calls              []clientCall
	closed, closeCalls int
	get                func(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error)
	post               func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error)
	close              func() error
}

func clientFixture(t *testing.T, suffix string) sdkSession.SessionBundle {
	t.Helper()
	b, e := (sdkSession.SberCredentials{UFSSession: "session-" + suffix, UFSToken: "token-" + suffix}).ToBundle(sdkSession.CredentialsBundleOptions{APIBase: "https://web-node-" + suffix + ".online.sberbank.ru", WebBase: "https://web-" + suffix + ".online.sberbank.ru", Deviceprint: sdkTransport.PtrString("version=1.7.3&fixture=true")})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func clientFake(t *testing.T, b sdkSession.SessionBundle) *clientFakeTransport {
	t.Helper()
	j, e := sdkSession.NewCookieJar(b.Cookies)
	if e != nil {
		t.Fatal(e)
	}
	return &clientFakeTransport{jar: j}
}
func (f *clientFakeTransport) record(c clientCall) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}
func (f *clientFakeTransport) Get(c context.Context, u string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	f.record(clientCall{method: "GET", target: u, options: o})
	if f.get != nil {
		return f.get(c, u, o)
	}
	return nil, fmt.Errorf("unexpected synthetic GET")
}
func (f *clientFakeTransport) Post(c context.Context, u string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	f.record(clientCall{method: "POST", target: u, body: p, options: o})
	if f.post != nil {
		return f.post(c, u, p, o)
	}
	return clientResponse(200, `{"success":true}`), nil
}
func (f *clientFakeTransport) PostForm(context.Context, string, map[string]string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	panic("credential POST forbidden in fixture")
}
func (f *clientFakeTransport) CookieJar() *sdkSession.CookieJar { return f.jar }
func (f *clientFakeTransport) Close() error {
	f.mu.Lock()
	f.closeCalls++
	f.mu.Unlock()
	if f.close != nil {
		if e := f.close(); e != nil {
			return e
		}
	}
	f.mu.Lock()
	f.closed++
	f.mu.Unlock()
	return nil
}
func (f *clientFakeTransport) counts() (int, int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls), f.closeCalls, f.closed
}
func (f *clientFakeTransport) snapshotCalls() []clientCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]clientCall(nil), f.calls...)
}
func clientResponse(status int, raw string) *sdkTransport.Response {
	return &sdkTransport.Response{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json"}}, Content: []byte(raw)}
}

func mustClientURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, e := url.Parse(s)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
func TestClientOwnsRotatingTransportAndRedacts(t *testing.T) {
	b := clientFixture(t, "old")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	// Input identities are copied and a complete Set-Cookie jar remains authoritative.
	b.Cookies[0].Value = "caller-corruption"
	*b.Deviceprint = "caller-corruption"
	u := mustClientURL(t, sdkSession.AppOrigin+"/")
	if e := tr.jar.ApplySetCookie(u, []string{"UFS-SESSION=session-new; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=Lax", "UFS-TOKEN=token-new; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly"}); e != nil {
		t.Fatal(e)
	}
	exported, e := c.ExportSession()
	if e != nil {
		t.Fatal(e)
	}
	creds, e := c.ExportCredentials()
	if e != nil || creds.UFSSession != "session-new" || creds.UFSToken != "token-new" {
		t.Fatalf("bad rotated credentials: %v", e)
	}
	if *exported.Deviceprint != "version=1.7.3&fixture=true" {
		t.Fatal("aliased identity")
	}
	exported.Cookies[0].Value = "export-corruption"
	*exported.Deviceprint = "export-corruption"
	next, e := c.ExportSession()
	if e != nil || *next.Deviceprint != "version=1.7.3&fixture=true" {
		t.Fatal("aliased export")
	}
	for _, s := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%+v", ClientOptions{Transport: tr, SessionPath: "synthetic-sensitive-path"})} {
		if strings.Contains(s, "session-new") || strings.Contains(s, "synthetic-sensitive-path") {
			t.Fatal("unredacted client")
		}
	}
	raw, e := json.Marshal(c)
	if e != nil || strings.Contains(string(raw), "token-new") {
		t.Fatal("unredacted JSON")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	calls, closes, _ := tr.counts()
	if calls != 0 || closes != 1 {
		t.Fatalf("unexpected lifecycle calls %d %d", calls, closes)
	}
}
