package bank

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

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
