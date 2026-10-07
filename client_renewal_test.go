package sber

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Done is observed at the gate's select, after arrival epoch capture. This makes
// contention tests deterministic without sleeps or scheduler assumptions.
type clientQueuedContext struct {
	context.Context
	once   sync.Once
	queued chan struct{}
}

func (c *clientQueuedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.queued) })
	return c.Context.Done()
}
func clientQueue() (*clientQueuedContext, <-chan struct{}) {
	ch := make(chan struct{})
	return &clientQueuedContext{Context: context.Background(), queued: ch}, ch
}

func TestClientPINReadRenewsOncePersistsBeforeRetryAndOwnsBothTransports(t *testing.T) {
	b := clientFixture(t, "old")
	b.Cookies = append(b.Cookies, CookieRecord{Name: "remembered", Value: "fixture", Domain: AuthCookieDomain, Path: "/CSAFront", Secure: true, HostOnly: true})
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	fresh := clientFixture(t, "new")
	fresh.Cookies = append(fresh.Cookies, b.Cookies[2])
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var builds, pins, renewals atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	o := ClientOptions{TransportOptions: TransportOptions{CABundle: "synthetic-CA"}, BrowserBootstrapTimeout: 7 * time.Second,
		TransportFactory: func(got SessionBundle, to TransportOptions) (Transport, error) {
			if to.CABundle != "synthetic-CA" {
				t.Error("CA not forwarded")
			}
			if builds.Add(1) == 1 {
				return old, nil
			}
			return next, nil
		},
		Renewal: func(ctx context.Context, got SessionBundle, provider PINProvider, ao AuthOptions) (SessionBundle, error) {
			if ao.TransportOptions.CABundle != "synthetic-CA" || ao.BrowserBootstrapTimeout != 7*time.Second {
				t.Error("auth options not forwarded")
			}
			if len(got.Cookies) != 3 || got.Cookies[2].Path != "/CSAFront" {
				t.Error("remembered profile not renewed")
			}
			pin, e := provider(ctx)
			if e != nil || pin != "13579" {
				t.Error("provider not used")
			}
			renewals.Add(1)
			close(started)
			<-release
			return fresh, nil
		}}
	next.post = func(_ context.Context, u string, _ map[string]any, _ RequestOptions) (*Response, error) {
		saved, e := LoadSessionBundle(path)
		if e != nil || saved.APIBase != fresh.APIBase {
			t.Error("retry before profile publication")
		}
		if u != fresh.APIBase+"/uoh-bh/v1/operations/list" {
			t.Error("stale business host")
		}
		return clientResponse(200, `{"success":true}`), nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { pins.Add(1); return "13579", nil }, o)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	results := make(chan error, 13)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); results <- e }()
	select {
	case <-started:
	case e := <-results:
		t.Fatalf("not renewed: %v", e)
	}
	for i := 0; i < 12; i++ {
		ctx, queued := clientQueue()
		go func() { _, e := c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); results <- e }()
		<-queued
	}
	close(release)
	for i := 0; i < 13; i++ {
		if e := <-results; e != nil {
			t.Fatal(e)
		}
	}
	if pins.Load() != 1 || renewals.Load() != 1 || builds.Load() != 2 {
		t.Fatal("duplicate credential/login attempt")
	}
	oldCalls, oldCloses, _ := old.counts()
	nextCalls, nextCloses, _ := next.counts()
	if oldCalls != 1 || oldCloses != 1 || nextCalls != 13 || nextCloses != 0 {
		t.Fatalf("wrong transport lifecycle %d %d %d %d", oldCalls, oldCloses, nextCalls, nextCloses)
	}
	exported, e := c.ExportSession()
	if e != nil || exported.APIBase != fresh.APIBase {
		t.Fatal("replacement not committed")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	_, oldCloses, _ = old.counts()
	_, nextCloses, _ = next.counts()
	if oldCloses != 1 || nextCloses != 1 {
		t.Fatal("owned cleanup repeated or leaked")
	}
}

func TestClientPINProfileUnreadyRenewsBeforeTransportConstruction(t *testing.T) {
	b := clientFixture(t, "unready")
	b.Cookies = nil
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	fresh := clientFixture(t, "initial")
	renewals, builds := 0, 0
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{
		Renewal: func(ctx context.Context, _ SessionBundle, p PINProvider, _ AuthOptions) (SessionBundle, error) {
			renewals++
			if _, e := p(ctx); e != nil {
				t.Error(e)
			}
			return fresh, nil
		},
		TransportFactory: func(got SessionBundle, _ TransportOptions) (Transport, error) {
			builds++
			saved, e := LoadSessionBundle(path)
			if e != nil || saved.APIBase != fresh.APIBase {
				t.Error("initial transport before persistence")
			}
			return clientFake(t, got), nil
		},
	})
	if e != nil {
		t.Fatal(e)
	}
	c.Close()
	if renewals != 1 || builds != 1 {
		t.Fatal("wrong initial renewal")
	}
	b.Deviceprint = nil
	if e = b.Save(path); e != nil {
		t.Fatal(e)
	}
	if _, e = NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { t.Fatal("provider called for missing identity"); return "", nil }, ClientOptions{}); e == nil {
		t.Fatal("profile without identity accepted")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	var insecure *InsecureSessionFile
	if _, e = NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "", nil }, ClientOptions{}); !errors.As(e, &insecure) {
		t.Fatal("nonprivate PIN profile accepted")
	}
}
