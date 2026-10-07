package sber

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestClientDefaultPINCanceledCleanupKeepsBothOwnershipAndCancellation(t *testing.T) {
	b := clientFixture(t, "pin-canceled-cleanup")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	auth := clientFake(t, b)
	auth.get = func(context.Context, string, RequestOptions) (*Response, error) {
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
	c, e := NewSberClientFromPINProfile(ctx, path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: AuthOptions{TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return auth, nil }}})
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
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	auth := clientFake(t, b)
	auth.get = func(context.Context, string, RequestOptions) (*Response, error) {
		return clientResponse(403, `synthetic bootstrap rejection`), nil
	}
	var attempts atomic.Int32
	auth.close = func() error {
		if attempts.Add(1) == 1 {
			return errors.New("synthetic auth close failure")
		}
		return nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: AuthOptions{TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return auth, nil }}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var cleanup interface{ Close() error }
	var pin *PinAuthError
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
