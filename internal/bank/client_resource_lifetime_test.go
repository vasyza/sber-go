package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientBindingCycleDoesNotExpandFormattingOrExportPrivateState(t *testing.T) {
	c, _ := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	p := clientBindingPortfolio(t, c, false)
	values := []any{c.Products(), *c.Products(), c.Operations(), *c.Operations(), c.Accounts(), *c.Accounts(), c.Cards(), *c.Cards(), c.Transfers(), *c.Transfers(), c.Analytics(), *c.Analytics(), c.Session(), *c.Session(), p, *p, p.Cards()[0], *p.Cards()[0], p.Accounts()[0], *p.Accounts()[0]}
	for _, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%p", "%w", "%t"} {
			out := fmt.Sprintf(verb, value)
			if len(out) > 8192 || strings.Contains(out, "session-binding-access") || strings.Contains(out, "token-binding-access") || strings.Contains(out, "version=1.7.3") {
				t.Fatalf("bound graph formatting traversed private requester state: type=%T verb=%s length=%d session=%v token=%v deviceprint=%v", value, verb, len(out), strings.Contains(out, "session-binding-access"), strings.Contains(out, "token-binding-access"), strings.Contains(out, "version=1.7.3"))
			}
			if strings.Contains(fmt.Errorf("diagnostic "+verb, value).Error(), "token-binding-access") {
				t.Fatal("error formatting leaked bound client")
			}
		}
		raw, err := json.Marshal(value)
		if err != nil || strings.Contains(string(raw), "session-binding-access") || strings.Contains(string(raw), "token-binding-access") {
			t.Fatal("bound graph JSON exported private requester")
		}
	}
	if _, err := c.Products().Get(context.Background(), false); err != nil {
		t.Fatal(err)
	}
}

func TestClientBindingEveryConstructorRetainsOneBundleWithoutExtraEffects(t *testing.T) {
	for _, allow := range []bool{false, true} {
		for _, name := range []string{"bundle", "credentials", "session-file", "ready-pin-profile", "synthetic-files"} {
			t.Run(name+map[bool]string{false: "-read-only", true: "-opt-in"}[allow], func(t *testing.T) {
				bundle := clientFixture(t, "binding-constructor")
				path := clientPrivatePath(t)
				if err := bundle.Save(path); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var builds, renewals, providers atomic.Int32
				var tr *clientFakeTransport
				options := ClientOptions{AllowMutations: allow, TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					builds.Add(1)
					tr = clientFake(t, b)
					return tr, nil
				}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
					renewals.Add(1)
					return bundle, nil
				}}
				var c *SberClient
				switch name {
				case "bundle":
					c, err = NewSberClient(bundle, options)
				case "credentials":
					c, err = NewSberClientFromCredentials(sdkSession.SberCredentials{UFSSession: "synthetic-constructor-session", UFSToken: "synthetic-constructor-token"}, sdkSession.CredentialsBundleOptions{APIBase: bundle.APIBase, WebBase: bundle.WebBase, Deviceprint: bundle.Deviceprint}, options)
				case "session-file":
					c, err = NewSberClientFromSessionFile(path, options)
				case "ready-pin-profile":
					c, err = NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers.Add(1); return "synthetic-pin", nil }, options)
				case "synthetic-files":
					// Generated here; no real HAR/session source is ever read.
					har := clientPrivatePath(t)
					raw := `{"log":{"entries":[{"request":{"method":"GET","url":"https://web2.online.sberbank.ru/main","headers":[]},"response":{"headers":[],"content":{}}},{"request":{"method":"POST","url":"https://web-node2.online.sberbank.ru/main-screen/rest/v2/m1/web/section/meta","headers":[],"cookies":[{"name":"UFS-SESSION","value":"synthetic-import-cookie","domain":"online.sberbank.ru","path":"/","secure":true}]},"response":{"headers":[],"content":{}}}]}}`
					if err = os.WriteFile(har, []byte(raw), 0600); err != nil {
						t.Fatal(err)
					}
					c, err = NewSberClientFromFiles(har, "", options)
				}
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				if !clientBindingOwnsRequester(c, c.Products().requester) || !clientBindingOwnsRequester(c, c.Operations().requester) || !clientBindingOwnsRequester(c, c.Accounts().requester) || !clientBindingOwnsRequester(c, c.Cards().requester) || !clientBindingOwnsRequester(c, c.Transfers().requester) || !clientBindingOwnsRequester(c, c.Analytics().requester) || !clientBindingOwnsRequester(c, c.Session().requester) {
					t.Fatal("constructor detached resource ownership")
				}
				if c.Accounts().binding != c.Cards().binding || c.Accounts().binding.transfers != c.Transfers() || c.Accounts().binding.operations != c.Operations() || c.Accounts().binding.cards != c.Cards() {
					t.Fatal("constructor created separate shortcut workflow owners")
				}
				if c.core().options.AllowMutations != allow || c.Cards().options.AllowMutations != allow || c.Transfers().options.AllowMutations != allow || c.Accounts().binding.options.AllowMutations != allow {
					t.Fatal("constructor mutation policy differs")
				}
				if builds.Load() != 1 || renewals.Load() != 0 || providers.Load() != 0 {
					t.Fatal("binding added transport/auth effects")
				}
				if n, _, _ := tr.counts(); n != 0 {
					t.Fatal("constructor made network request")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("resource initialization changed persistence")
				}
			})
		}
	}
}

func TestClientBindingConcurrentGettersAndReadsKeepOneIssuer(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	other, _ := clientBindingSyntheticClient(t, resourcePortfolioResponse())
	if c.Transfers() == other.Transfers() || c.Products() == other.Products() || c.Accounts().binding == other.Accounts().binding {
		t.Fatal("global client interning")
	}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			products, operations, accounts := c.Products(), c.Operations(), c.Accounts()
			cards, transfers, analytics := c.Cards(), c.Transfers(), c.Analytics()
			sessionAPI := c.Session()
			if products != c.Products() || operations != c.Operations() || accounts != c.Accounts() || cards != c.Cards() || transfers != c.Transfers() || analytics != c.Analytics() || sessionAPI != c.Session() {
				t.Error("getter cache unstable")
			}
			p, err := c.Portfolio(context.Background(), false)
			if err != nil {
				t.Error(err)
				return
			}
			if p.Transfers() != c.Transfers() || p.Cards()[0].bankBinding() != c.Accounts().binding || p.Cards()[0].Account().Cards()[0] != p.Cards()[0] {
				t.Error("concurrent snapshot detached")
			}
			cloned := p.Raw()
			cloned.Cards[0].Name = "caller-edit"
		}()
	}
	wg.Wait()
	if n, _, _ := tr.counts(); n != 24 {
		t.Fatal("concurrent reads added/dropped requests")
	}
}

func TestClientBindingHistoryCalendarDoesNotUseMonotonicWarmupClock(t *testing.T) {
	bundle := clientFixture(t, "binding-clock")
	tr := clientFake(t, bundle)
	var monotonicCalls atomic.Int32
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientBindingResponse(t, resourceOperationsResponse()), nil
	}
	c, err := NewSberClient(bundle, ClientOptions{Transport: tr, Monotonic: func() time.Time { monotonicCalls.Add(1); return time.Unix(1, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	before := time.Now().In(domainMoscow).AddDate(0, 0, -120).Truncate(time.Second)
	if _, err := c.Operations().Page(context.Background()); err != nil {
		t.Fatal(err)
	}
	after := time.Now().In(domainMoscow).AddDate(0, 0, -120)
	calls := tr.snapshotCalls()
	from, err := time.ParseInLocation("02.01.2006T15:04:05", calls[0].body["from"].(string), domainMoscow)
	if err != nil || from.Before(before) || from.After(after) || monotonicCalls.Load() != 0 {
		t.Fatal("history used monotonic/debounce clock instead of resource calendar default")
	}
	if !reflect.DeepEqual(calls[0].body["paginationSize"], 31) {
		t.Fatal("history default changed")
	}
	if err := c.Session().WarmUp(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := c.Session().WarmUp(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if monotonicCalls.Load() == 0 {
		t.Fatal("warmup no longer uses client monotonic clock")
	}
	if n, _, _ := tr.counts(); n != 2 {
		t.Fatal("history warmed session or debounce ignored")
	}
}

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
			if err := read.call(); !errors.Is(err, sdkErrs.ErrClosed) {
				t.Fatalf("closed bound read: %v", err)
			}
		})
	}
	callback := false
	err := c.Products().requester.MutationSequence(ctx, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		callback = true
		return nil
	})
	if !errors.Is(err, sdkErrs.ErrClosed) || callback {
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
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var builds, renewals, pins atomic.Int32
	next.post = func(_ context.Context, target string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		saved, err := sdkSession.LoadSessionBundle(path)
		if err != nil || saved.APIBase != fresh.APIBase || saved.Cookies[0].Value != fresh.Cookies[0].Value {
			t.Error("typed retry preceded durable session replacement")
		}
		if target != fresh.APIBase+ProductsPath {
			t.Error("typed retry retained old host")
		}
		return clientBindingResponse(t, resourcePortfolioResponse()), nil
	}
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { pins.Add(1); return "synthetic-pin", nil }, ClientOptions{
		TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			if builds.Add(1) == 1 {
				return old, nil
			}
			return next, nil
		},
		Renewal: func(ctx context.Context, _ sdkSession.SessionBundle, provider sdkAuth.PINProvider, _ sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
			renewals.Add(1)
			if _, err := provider(ctx); err != nil {
				return sdkSession.SessionBundle{}, err
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
	if _, err := c.Portfolio(context.Background(), false); !errors.Is(err, sdkErrs.ErrClosed) {
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
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if err := tr.jar.ApplySetCookie(mustClientURL(t, bundle.APIBase+ProductsPath), []string{"UFS-SESSION=synthetic-rejected-rotation; Domain=online.sberbank.ru; Path=/; Secure"}); err != nil {
			t.Error(err)
		}
		return clientResponse(200, `{"success":false,"error":{"code":"fixture-rejected"},"body":{"sections":{}}}`), nil
	}
	var renewals, providers atomic.Int32
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { providers.Add(1); return "synthetic-pin", nil }, ClientOptions{Transport: tr, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		renewals.Add(1)
		return bundle, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, err := c.Portfolio(context.Background(), true)
	var rejected *sdkErrs.APIRejected
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
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var renewal atomic.Int32
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic-pin", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
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
