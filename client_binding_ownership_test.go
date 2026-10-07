package sber

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
)

func TestClientBindingClosedReadsReturnErrClosedWithoutCallbacksOrResurrection(t *testing.T) {
	c, tr := clientBindingScript(t, true, []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}})
	p := clientBindingPortfolio(t, c, false)
	issuer := c.Transfers()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	reads := []struct {
		name string
		call func() error
	}{
		{"products", func() error { _, e := c.Products().Get(ctx, false); return e }},
		{"portfolio", func() error { _, e := c.Portfolio(ctx, true); return e }},
		{"accounts", func() error { _, e := c.Accounts().List(ctx, false); return e }},
		{"cards-list", func() error { _, e := c.Cards().List(ctx, false); return e }},
		{"cards-info", func() error { _, e := c.Cards().Info(ctx, 4004); return e }},
		{"operations", func() error {
			_, e := c.Operations().Page(ctx, OperationsPageOptions{Limit: 1, From: "2026-07-01"})
			return e
		}},
		{"analytics", func() error { _, e := c.Analytics().Amounts(ctx, "2026-07-01", "2026-07-31"); return e }},
		{"session-export", func() error { _, e := c.Session().Export(); return e }},
		{"session-credentials", func() error { _, e := c.Session().Credentials(); return e }},
		{"session-warmup", func() error { return c.Session().WarmUp(ctx) }},
		{"entity-operations", func() error {
			_, e := p.Cards()[0].Operations(ctx, OperationsQuery{Limit: 1, MaxPages: 1, From: "2026-07-01"})
			return e
		}},
		{"requester-mutate", func() error {
			_, e := c.Products().requester.Mutate(ctx, ChangeProductNamePath, nil, nil, "/app/cards/details/4004", false)
			return e
		}},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			if err := read.call(); !errors.Is(err, ErrClosed) {
				t.Fatalf("closed bound read: %v", err)
			}
		})
	}
	callback := false
	err := c.Products().requester.MutationSequence(ctx, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		callback = true
		return nil
	})
	if !errors.Is(err, ErrClosed) || callback {
		t.Fatal("closed resource sequence resurrected callback")
	}
	// Existing resource mutation contracts classify attempted failures as uncertain;
	// do not rewrite them just to expose the core ErrClosed sentinel.
	if _, err := issuer.Start(ctx); err == nil {
		t.Fatal("closed workflow accepted")
	}
	if err := p.Cards()[0].Rename(ctx, "Closed fixture"); err == nil {
		t.Fatal("closed entity rename accepted")
	}
	if c.Transfers() != issuer || p.Transfers() != issuer {
		t.Fatal("close silently replaced workflow lifetime")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if n, closes, _ := tr.counts(); n != 1 || closes != 1 {
		t.Fatal("close resurrected requests or repeated transport cleanup")
	}
}

func TestClientBindingTypedPortfolioRenewalPersistsBeforeRetryAndRetainsIssuer(t *testing.T) {
	oldBundle, fresh := clientFixture(t, "binding-renew-old"), clientFixture(t, "binding-renew-new")
	path := clientPrivatePath(t)
	if err := oldBundle.Save(path); err != nil {
		t.Fatal(err)
	}
	old, next := clientFake(t, oldBundle), clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var builds, renewals, pins atomic.Int32
	next.post = func(_ context.Context, target string, _ map[string]any, _ RequestOptions) (*Response, error) {
		saved, err := LoadSessionBundle(path)
		if err != nil || saved.APIBase != fresh.APIBase || saved.Cookies[0].Value != fresh.Cookies[0].Value {
			t.Error("typed retry preceded durable session replacement")
		}
		if target != fresh.APIBase+ProductsPath {
			t.Error("typed retry retained old host")
		}
		return clientBindingResponse(t, resourcePortfolioResponse()), nil
	}
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { pins.Add(1); return "synthetic-pin", nil }, ClientOptions{
		TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) {
			if builds.Add(1) == 1 {
				return old, nil
			}
			return next, nil
		},
		Renewal: func(ctx context.Context, _ SessionBundle, provider PINProvider, _ AuthOptions) (SessionBundle, error) {
			renewals.Add(1)
			if _, err := provider(ctx); err != nil {
				return SessionBundle{}, err
			}
			return fresh, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	before := c.core().resources
	issuer, products := c.Transfers(), c.Products()
	p := clientBindingPortfolio(t, c, true)
	if p.Transfers() != issuer || c.core().resources != before || c.Products() != products {
		t.Fatal("renewal replaced typed resources/issuer")
	}
	accounts, err := c.Accounts().List(context.Background(), false)
	if err != nil || accounts[0].bankBinding().transfers != issuer {
		t.Fatal("post-renewal shortcut binding")
	}
	creds, err := c.Session().Credentials()
	if err != nil || creds.UFSSession != "session-binding-renew-new" {
		t.Fatal("session API retained old credentials")
	}
	if builds.Load() != 2 || renewals.Load() != 1 || pins.Load() != 1 {
		t.Fatal("binding added repeated auth/transport work")
	}
	if n, closes, _ := old.counts(); n != 1 || closes != 1 {
		t.Fatal("old transport lifecycle changed")
	}
	if n, closes, _ := next.counts(); n != 2 || closes != 0 {
		t.Fatal("new transport request ownership changed")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if n, closes, _ := next.counts(); n != 2 || closes != 1 {
		t.Fatal("new transport not closed once")
	}
	if _, err := c.Portfolio(context.Background(), false); !errors.Is(err, ErrClosed) {
		t.Fatal("typed portfolio reopened a closed PIN client")
	}
	if builds.Load() != 2 || renewals.Load() != 1 || pins.Load() != 1 {
		t.Fatal("closed convenience access resurrected auth")
	}
}

func TestClientBindingRejectedProductsCannotPublishOrRenew(t *testing.T) {
	bundle := clientFixture(t, "binding-rejected")
	path := clientPrivatePath(t)
	if err := bundle.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tr := clientFake(t, bundle)
	tr.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		if err := tr.jar.ApplySetCookie(mustClientURL(t, bundle.APIBase+ProductsPath), []string{"UFS-SESSION=synthetic-rejected-rotation; Domain=online.sberbank.ru; Path=/; Secure"}); err != nil {
			t.Error(err)
		}
		return clientResponse(200, `{"success":false,"error":{"code":"fixture-rejected"},"body":{"sections":{}}}`), nil
	}
	var renewals, providers atomic.Int32
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers.Add(1); return "synthetic-pin", nil }, ClientOptions{Transport: tr, Renewal: func(context.Context, SessionBundle, PINProvider, AuthOptions) (SessionBundle, error) {
		renewals.Add(1)
		return bundle, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, err := c.Portfolio(context.Background(), true)
	var rejected *APIRejected
	if p != nil || !errors.As(err, &rejected) || rejected.Code != "fixture-rejected" {
		t.Fatal("source rejection became a typed successful/empty snapshot")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected response persisted rotated cookies")
	}
	if renewals.Load() != 0 || providers.Load() != 0 {
		t.Fatal("definite rejection triggered login")
	}
	if n, _, _ := tr.counts(); n != 1 {
		t.Fatal("rejected read replayed")
	}
}

func TestClientBindingRenewalPersistenceFailureCannotReturnTypedSnapshot(t *testing.T) {
	bundle, fresh := clientFixture(t, "binding-save-old"), clientFixture(t, "binding-save-new")
	path := clientPrivatePath(t)
	if err := bundle.Save(path); err != nil {
		t.Fatal(err)
	}
	old, next := clientFake(t, bundle), clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var renewal atomic.Int32
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic-pin", nil }, ClientOptions{Transport: old, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return next, nil }, Renewal: func(context.Context, SessionBundle, PINProvider, AuthOptions) (SessionBundle, error) {
		renewal.Add(1)
		return fresh, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	issuer := c.Transfers()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := c.Portfolio(context.Background(), false)
	if err == nil || p != nil {
		t.Fatal("typed snapshot bypassed failed renewal publication")
	}
	if n, _, _ := next.counts(); n != 0 {
		t.Fatal("typed retry began before persistence")
	}
	exported, err := c.Session().Export()
	if err != nil || exported.APIBase != bundle.APIBase || c.Transfers() != issuer {
		t.Fatal("failed renewal changed binding or committed identity")
	}
	if renewal.Load() != 1 {
		t.Fatal("renewal persistence failure replayed login")
	}
}
