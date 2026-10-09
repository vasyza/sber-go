package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkAuth "github.com/vasyza/sber-sdk/internal/auth"
	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

// A fresh independent fixture: no socket/client, no credential form path.
type i3Transport struct {
	mu                               sync.Mutex
	jar                              *sdkSession.CookieJar
	posts, gets, closes, closedPosts int
	closed                           bool
	post                             func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error)
	closeHook                        func(int) error
}

func i3Bundle(t *testing.T, suffix string) sdkSession.SessionBundle {
	t.Helper()
	b, e := (sdkSession.SberCredentials{UFSSession: "i3-session-" + suffix, UFSToken: "i3-token-" + suffix}).ToBundle(sdkSession.CredentialsBundleOptions{APIBase: "https://web-node3.online.sberbank.ru", WebBase: "https://web3.online.sberbank.ru", Deviceprint: sdkTransport.PtrString("i3-device-" + suffix), AntifraudDeviceprint: sdkTransport.PtrString("i3-antifraud-" + suffix), Browser: sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "user-agent", Value: "i3-private-header-" + suffix}}}})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func i3NewTransport(t *testing.T, b sdkSession.SessionBundle) *i3Transport {
	t.Helper()
	j, e := sdkSession.NewCookieJar(b.Cookies)
	if e != nil {
		t.Fatal(e)
	}
	return &i3Transport{jar: j}
}
func i3Response(status int, text string) *sdkTransport.Response {
	return &sdkTransport.Response{StatusCode: status, Content: []byte(text), Headers: http.Header{"Content-Type": []string{"application/json"}}}
}
func (tr *i3Transport) Get(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	tr.mu.Lock()
	tr.gets++
	tr.mu.Unlock()
	return nil, errors.New("independent synthetic unexpected GET")
}
func (tr *i3Transport) Post(ctx context.Context, target string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	tr.mu.Lock()
	tr.posts++
	closed := tr.closed
	if closed {
		tr.closedPosts++
	}
	tr.mu.Unlock()
	if closed {
		return nil, sdkErrs.ErrClosed
	}
	if tr.post != nil {
		return tr.post(ctx, target, p, o)
	}
	return i3Response(200, `{"success":true}`), nil
}
func (*i3Transport) PostForm(context.Context, string, map[string]string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	panic("credential form path forbidden in independent fixture")
}
func (tr *i3Transport) CookieJar() *sdkSession.CookieJar { return tr.jar }
func (tr *i3Transport) Close() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.closes++
	if tr.closeHook != nil {
		if e := tr.closeHook(tr.closes); e != nil {
			return e
		}
	}
	tr.closed = true
	return nil
}
func (tr *i3Transport) counts() (posts, gets, closes, closedPosts int) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.posts, tr.gets, tr.closes, tr.closedPosts
}
func i3Client(t *testing.T, b sdkSession.SessionBundle, tr sdkTransport.Transport, o ClientOptions) *SberClient {
	t.Helper()
	o.Transport = tr
	c, e := NewSberClient(b, o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
func i3Profile(t *testing.T, b sdkSession.SessionBundle) string {
	t.Helper()
	p := filepath.Join(testPrivateDir(t), "synthetic-profile.json")
	if e := b.Save(p); e != nil {
		t.Fatal(e)
	}
	return p
}
func i3HasMarker(text string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

func TestIndependentClient3FallbackNegativeControlAndGraphPrivacy(t *testing.T) {
	b := i3Bundle(t, "privacy-control")
	tr := i3NewTransport(t, b)
	c := i3Client(t, b, tr, ClientOptions{SessionPath: "i3-private-profile-path"})
	value := *c
	markers := []string{b.Cookies[0].Value, b.Cookies[1].Value, *b.Deviceprint, *b.AntifraudDeviceprint, b.Browser.Headers[0].Value, "i3-private-profile-path"}
	// Plain raw storage must be detectable through the same fmt misuse.
	raw := struct {
		cookies []sdkSession.CookieRecord
		path    string
	}{b.Cookies, "i3-private-profile-path"}
	badVerb := "%w"
	if !i3HasMarker(fmt.Errorf(badVerb, raw).Error(), markers) {
		t.Fatal("unsupported-fmt negative control did not expose raw mirror")
	}
	p := struct{ private any }{value}
	values := []any{c, value, &value, &c, []any{c, value}, map[any]any{value: c}, p, &p, reflect.ValueOf(p).Field(0), reflect.ValueOf(value), c.Products(), c.Operations(), c.Accounts(), c.Cards(), c.Transfers(), c.Analytics(), c.Session(), c.core().resources, c.core().resourcesOwnerForTest()}
	formats := []string{"%w", "%#w", "%p", "%#p", "%#v", "%x", "%X", "%s", "%d", "%T", "%+q", "%[2]w", "%w %w", "%0.3f"}
	for vi, v := range values {
		t.Run(fmt.Sprintf("surface-%02d", vi), func(t *testing.T) {
			for _, format := range formats {
				var buf bytes.Buffer
				log.New(&buf, "", 0).Printf(format, v)
				parts := []string{fmt.Sprintf(format, v), fmt.Errorf(format, v).Error(), buf.String()}
				buf.Reset()
				slog.New(slog.NewTextHandler(&buf, nil)).Info("synthetic", slog.Any("surface", v))
				parts = append(parts, buf.String())
				for _, text := range parts {
					if i3HasMarker(text, markers) {
						t.Fatal("ordinary fallback diagnostic leaked synthetic private state")
					}
				}
			}
		})
	}
	creds, e := value.ExportCredentials()
	if e != nil || creds.UFSSession != b.Cookies[0].Value && creds.UFSSession != b.Cookies[1].Value {
		t.Fatal("explicit export control lost synthetic state")
	}
	for _, v := range []any{c, value, []any{c, value}, map[string]any{"client": value}} {
		data, e := json.Marshal(v)
		if e != nil || i3HasMarker(string(data), markers) {
			t.Fatal("ordinary JSON privacy failure")
		}
	}
}

// Probe helper only; production files are unchanged.
func (s *clientState) resourcesOwnerForTest() any { return (*s.resources).requester }

func i3HAR(t *testing.T, field string, value any, prior bool) string {
	t.Helper()
	cookie := map[string]any{"name": "SID", "value": "synthetic-only-expiry", "domain": "online.sberbank.ru", "path": "/", "secure": true}
	if field != "" {
		cookie[field] = value
	}
	entries := []any{}
	if prior {
		entries = append(entries, map[string]any{"request": map[string]any{"url": "https://web3.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{map[string]any{"name": "SID", "value": "synthetic-prior", "domain": "online.sberbank.ru", "path": "/", "secure": true}, map[string]any{"name": "OTHER", "value": "synthetic-retained", "domain": "online.sberbank.ru", "path": "/", "secure": true}}}})
	}
	entries = append(entries, map[string]any{"startedDateTime": "2026-07-13T09:00:00Z", "request": map[string]any{"url": "https://web3.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{cookie}}}, map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node3.online.sberbank.ru/uoh-bh/v1/operations/list"}})
	raw, e := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
	if e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(testPrivateDir(t), "synthetic.har")
	if e = os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestIndependentClient3ExplicitExpiryBoundaryMatrix(t *testing.T) {
	for _, field := range []string{"expires", "maxAge", "max-age"} {
		for i, n := range []string{"-1", "-1e25", "-9223372036854775808", "-9223372036854775809", "-9.223372036854776e18", "-" + strings.Repeat("9", 90)} {
			t.Run(fmt.Sprintf("expired-%s-%d", field, i), func(t *testing.T) {
				builds := 0
				c, e := NewSberClientFromFiles(i3HAR(t, field, json.Number(n), true), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					builds++
					return i3NewTransport(t, b), nil
				}})
				if e != nil {
					t.Fatal("otherwise-ready import rejected", e)
				}
				defer c.Close()
				b, e := c.ExportSession()
				if e != nil || builds != 1 || len(b.Cookies) != 1 || b.Cookies[0].Name != "OTHER" {
					t.Fatal("expired spelling resurrected matching cookie")
				}
			})
		}
	}
	for _, field := range []string{"expires", "maxAge", "max-age"} {
		for i, n := range []string{"1e309", "-1e309", "1.23e9999", "-4.56e9999"} {
			t.Run(fmt.Sprintf("nonfinite-%s-%d", field, i), func(t *testing.T) {
				builds := 0
				c, e := NewSberClientFromFiles(i3HAR(t, field, json.Number(n), false), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					builds++
					return i3NewTransport(t, b), nil
				}})
				if c != nil {
					_ = c.Close()
				}
				if e == nil || c != nil || builds != 0 {
					t.Fatal("nonfinite explicit lifetime fell back to session")
				}
			})
		}
	}
	for i, n := range []string{"9223372036854775807", "9223372036854775808", "1e28", strings.Repeat("8", 95)} {
		t.Run(fmt.Sprintf("future-cap-%d", i), func(t *testing.T) {
			c, e := NewSberClientFromFiles(i3HAR(t, "expires", json.Number(n), false), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return i3NewTransport(t, b), nil
			}})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			b, e := c.ExportSession()
			if e != nil || len(b.Cookies) != 1 || b.Cookies[0].Expires == nil || *b.Cookies[0].Expires != math.MaxInt64 {
				t.Fatal("future bound was treated as absent")
			}
		})
	}
}

func TestIndependentClient3ConcurrentLateSendCarriesOneOutcome(t *testing.T) {
	b := i3Bundle(t, "late-send")
	tr := i3NewTransport(t, b)
	started := make(chan struct{})
	release := make(chan struct{})
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		close(started)
		<-release
		return nil, context.DeadlineExceeded
	}
	c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
	private := errors.New("synthetic-private-callback-diagnostic")
	senderDone := make(chan error, 1)
	callbackDone := make(chan struct{})
	outcome := make(chan error, 1)
	var escaped func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
	go func() {
		outcome <- c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
			escaped = send
			go func() {
				_, e := send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
				senderDone <- e
			}()
			<-started
			close(callbackDone)
			return private
		})
	}()
	<-callbackDone
	select {
	case <-outcome:
		t.Fatal("sequence returned before already-started sender finished")
	default:
	}
	close(release)
	e := <-outcome
	sent := <-senderDone
	var u *sdkErrs.MutationUncertain
	if e != sent || !errors.As(e, &u) || !errors.Is(e, context.DeadlineExceeded) || errors.Is(e, private) {
		t.Fatal("callback replaced or lost established send outcome")
	}
	if _, e = escaped(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true); e == nil {
		t.Fatal("late capability escaped")
	}
	if n, _, _, _ := tr.counts(); n != 1 {
		t.Fatal("financial request replayed")
	}
}
func TestIndependentClient3CallbackFailureControlAndPoisoning(t *testing.T) {
	for _, mode := range []string{"success", "transport-failure", "cancel-before-send"} {
		t.Run(mode, func(t *testing.T) {
			b := i3Bundle(t, "callback")
			tr := i3NewTransport(t, b)
			if mode == "transport-failure" {
				tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return nil, context.Canceled
				}
			}
			c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
			callbackPrivate := errors.New("synthetic-private-callback-error")
			var first error
			e := c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
				ctx := context.Background()
				if mode == "cancel-before-send" {
					child, cancel := context.WithCancel(ctx)
					cancel()
					ctx = child
				}
				_, first = send(ctx, Me2MeWorkflowPath, nil, nil, "/app/test", true)
				if mode != "success" {
					_, next := send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
					if next != first {
						t.Error("failed sequence did not preserve poison identity")
					}
				}
				return callbackPrivate
			})
			if e == nil || errors.Is(e, callbackPrivate) || strings.Contains(e.Error(), callbackPrivate.Error()) {
				t.Fatal("foreign callback diagnostics retained")
			}
			var u *sdkErrs.MutationUncertain
			wantPosts := 1
			switch mode {
			case "success":
				if errors.As(e, &u) {
					t.Fatal("successful send acquired fabricated uncertainty")
				}
			case "transport-failure":
				if e != first || !errors.As(e, &u) || !errors.Is(e, context.Canceled) {
					t.Fatal("after-send outcome lost")
				}
			default:
				wantPosts = 0
				if e != first || errors.As(e, &u) || !errors.Is(e, context.Canceled) {
					t.Fatal("pre-send cancellation mislabeled")
				}
			}
			if n, _, _, _ := tr.counts(); n != wantPosts {
				t.Fatal("unexpected independent transport attempts")
			}
		})
	}
}

type i3IdentityValue struct {
	*i3Transport
	metadata []byte
}

func (v i3IdentityValue) ClientTransportOwner() sdkTransport.Transport { return v.i3Transport }

type i3UnverifiableValue struct {
	*i3Transport
	metadata []byte
}
type i3RuntimeNoncomparable struct {
	*i3Transport
	metadata any
}

func TestIndependentClient3ClosingOwnerRejectionMatrix(t *testing.T) {
	for _, mode := range []string{"honest-alias", "unverifiable-fresh", "runtime-noncomparable", "error-and-alias"} {
		t.Run(mode, func(t *testing.T) {
			b := i3Bundle(t, "old-owner")
			fresh := i3Bundle(t, "new-owner")
			path := i3Profile(t, b)
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			old, next := i3NewTransport(t, b), i3NewTransport(t, fresh)
			old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return i3Response(401, `{}`), nil
			}
			var initial sdkTransport.Transport = old
			var replacement sdkTransport.Transport = i3IdentityValue{old, []byte{2}}
			var factoryError error
			switch mode {
			case "unverifiable-fresh":
				replacement = i3UnverifiableValue{next, []byte{2}}
			case "runtime-noncomparable":
				initial = i3RuntimeNoncomparable{old, []byte{1}}
				replacement = initial
			case "error-and-alias":
				factoryError = errors.New("synthetic-private-factory-error")
			}
			c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { t.Error("unexpected PIN callback"); return "", nil }, ClientOptions{Transport: initial, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return replacement, factoryError
			}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
				return fresh, nil
			}})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			var te *sdkErrs.TransportError
			if !errors.As(e, &te) || te.Code != "reused_transport" {
				t.Fatal("ambiguous closing owner accepted")
			}
			after, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("rejected candidate persisted profile")
			}
			if _, _, closes, _ := old.counts(); closes != 0 {
				t.Fatal("committed owner closed by rejection")
			}
			if _, _, closes, _ := next.counts(); closes != 0 {
				t.Fatal("unowned fresh ambiguous candidate closed")
			}
			if e = c.Close(); e != nil {
				t.Fatal(e)
			}
			if _, _, closes, _ := old.counts(); closes != 1 {
				t.Fatal("old owner close not exactly once")
			}
			if _, _, closes, _ := next.counts(); closes != 0 {
				t.Fatal("rejected owner acquired at final cleanup")
			}
		})
	}
}
func TestIndependentClient3AuthGateConcurrentClaimsAndCleanup(t *testing.T) {
	b := i3Bundle(t, "auth-gate")
	old, auth := i3NewTransport(t, b), i3NewTransport(t, b)
	auth.closeHook = func(n int) error {
		if n == 1 {
			return errors.New("synthetic close failure")
		}
		return nil
	}
	c := i3Client(t, b, old, ClientOptions{})
	guarded := clientGuardAuthOptions(sdkAuth.AuthOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return i3IdentityValue{auth, []byte{1}}, errors.New("synthetic factory failure")
	}}, c.claimAuthTransport)
	if got, e := guarded.TransportFactory(b, sdkTransport.TransportOptions{}); got != nil || e == nil {
		t.Fatal("explicit borrowed owner acquired by auth")
	}
	if _, _, n, _ := old.counts(); n != 0 {
		t.Fatal("auth closed borrowed owner")
	}
	_, e := guarded.TransportFactory(b, sdkTransport.TransportOptions{})
	var pending *ClientCleanupError
	if !errors.As(e, &pending) {
		t.Fatal("fresh failing auth factory lost retryable cleanup")
	}
	if e = pending.Close(); e != nil {
		t.Fatal(e)
	}
	if e = pending.Close(); e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	results := make(chan error, 12)
	fresh := i3NewTransport(t, b)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, e := c.claimAuthTransport(i3IdentityValue{fresh, []byte{byte(i)}})
			results <- e
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	accepted := 0
	for e := range results {
		if e == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("auth acquired multiple aliases")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	if _, _, n, _ := auth.counts(); n != 2 {
		t.Fatal("shared auth/client owner duplicated successful cleanup")
	}
	if _, _, n, _ := fresh.counts(); n != 1 {
		t.Fatal("concurrent owner cleanup duplicated")
	}
}

const i3PortfolioJSON = `{"success":true,"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":3101,"name":"synthetic-current","number":"unique-i3","balance":{"amount":"10.50","currencyCode":"RUB"}}]},"accounts":{"data":[]},"cardsInWallet":{"data":[{"id":3404,"name":"synthetic-card","number":"0004","isCTA":true,"cardAccount":"unique-i3","availableTotalLimit":{"amount":"2.50","currencyCode":"RUB"}}]}}}}}}`

func TestIndependentClient3CachedPortfolioRenewalAndCopyClose(t *testing.T) {
	b, fresh := i3Bundle(t, "bundle-old"), i3Bundle(t, "bundle-new")
	path := i3Profile(t, b)
	old, next := i3NewTransport(t, b), i3NewTransport(t, fresh)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return i3Response(401, `{}`), nil
	}
	next.post = func(ctx context.Context, target string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		saved, e := sdkSession.LoadSessionBundle(path)
		if e != nil {
			t.Error(e)
		} else {
			creds, e := sdkSession.CredentialsFromBundle(saved)
			if e != nil || creds.UFSSession != "i3-session-bundle-new" {
				t.Error("retry happened before renewed profile persisted")
			}
		}
		if !strings.HasSuffix(target, ProductsPath) || p["withData"] != true {
			t.Error("portfolio request deviated")
		}
		return i3Response(200, i3PortfolioJSON), nil
	}
	var renewals atomic.Int32
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { t.Error("unexpected PIN use"); return "", nil }, ClientOptions{Transport: i3IdentityValue{old, []byte{1}}, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return i3IdentityValue{next, []byte{2}}, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		renewals.Add(1)
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	getters := []any{c.Products(), c.Operations(), c.Accounts(), c.Cards(), c.Transfers(), c.Analytics(), c.Session()}
	cloned := *c
	first, e := c.Portfolio(context.Background(), false)
	if e != nil {
		t.Fatal(e)
	}
	later, e := cloned.Portfolio(context.Background(), true)
	if e != nil {
		t.Fatal(e)
	}
	if first == later || first.Cards()[0] == later.Cards()[0] || first.Transfers() != c.Transfers() || later.Transfers() != c.Transfers() {
		t.Fatal("portfolio rebuilt cached issuer or financial snapshots")
	}
	if first.Cards()[0].Account() != first.Accounts()[0] || first.Cards()[0].Balance().Amount.String() != "2.50" {
		t.Fatal("typed balance or relations lost")
	}
	now := []any{cloned.Products(), cloned.Operations(), cloned.Accounts(), cloned.Cards(), cloned.Transfers(), cloned.Analytics(), cloned.Session()}
	if !reflect.DeepEqual(getters, now) || renewals.Load() != 1 {
		t.Fatal("cached resource lifetime changed")
	}
	if e = cloned.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = first.Cards()[0].Operations(context.Background()); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("retained financial entity resurrected closed client")
	}
	if _, _, n, closedPosts := next.counts(); n != 1 || closedPosts != 0 {
		t.Fatal("copy close duplicated owner or attempted closed send")
	}
}
func TestIndependentClient3CloseRacingReplacementCannotPublish(t *testing.T) {
	b, fresh := i3Bundle(t, "race-old"), i3Bundle(t, "race-new")
	path := i3Profile(t, b)
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	old, next := i3NewTransport(t, b), i3NewTransport(t, fresh)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return i3Response(401, `{}`), nil
	}
	factoryEntered, release := make(chan struct{}), make(chan struct{})
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		close(factoryEntered)
		<-release
		return next, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	readDone := make(chan error, 1)
	go func() { _, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); readDone <- e }()
	<-factoryEntered
	closeDone := make(chan error, 1)
	go func() { closeDone <- c.Close() }()
	<-c.core().root.Done()
	close(release)
	if e = <-readDone; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("close-raced read succeeded")
	}
	if e = <-closeDone; e != nil {
		t.Fatal(e)
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("close-raced replacement published profile")
	}
	for _, tr := range []*i3Transport{old, next} {
		if _, _, n, deadPosts := tr.counts(); n != 1 || deadPosts != 0 {
			t.Fatal("close race lost or duplicated discarded transport")
		}
	}
}
func TestIndependentClient3WarmupPersistsBeforeDebounce(t *testing.T) {
	b := i3Bundle(t, "warm")
	tr := i3NewTransport(t, b)
	dir := testPrivateDir(t)
	path := filepath.Join(dir, "not-created", "profile.json")
	clock := time.Now()
	c := i3Client(t, b, tr, ClientOptions{SessionPath: path, Monotonic: func() time.Time { return clock }})
	if e := c.WarmUp(context.Background(), false); e == nil {
		t.Fatal("failed persistence reported warmup success")
	}
	if e := os.Mkdir(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if e := c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if n, gets, _, _ := tr.counts(); n != 2 || gets != 0 {
		t.Fatal("failed capture established debounce or hidden discovery")
	}
	if _, e := sdkSession.LoadSessionBundle(path); e != nil {
		t.Fatal(e)
	}
}

// Public-client outcome diagnostic: raw response integrity failure after a send
// is not the same as an explicit server business rejection.
func TestIndependentClient3MalformedFinancialResponsesRemainUncertain(t *testing.T) {
	for _, mode := range []string{"missing-response", "invalid-json", "integrity-duplicate", "server-502"} {
		t.Run(mode, func(t *testing.T) {
			b := i3Bundle(t, "raw-uncertain")
			tr := i3NewTransport(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				switch mode {
				case "missing-response":
					return nil, nil
				case "invalid-json":
					return i3Response(200, `{"success":true`), nil
				case "integrity-duplicate":
					return i3Response(200, `{"success":true,"success":true}`), nil
				default:
					return i3Response(502, `{}`), nil
				}
			}
			c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
			_, e := c.Mutate(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
			var u *sdkErrs.MutationUncertain
			if !errors.As(e, &u) {
				t.Error("after-send unknown financial response lacks MutationUncertain")
			}
			if n, _, _, _ := tr.counts(); n != 1 {
				t.Fatal("financial send replayed")
			}
		})
	}
}

// Synthetic interface-composition witness. It does not execute PIN/SRP or
// credential posts and does not claim a full successful auth-flow run.
func TestIndependentClient3InitialAuthOwnershipSurvivesBusinessHandoff(t *testing.T) {
	b := i3Bundle(t, "initial-auth-handoff")
	shared := i3NewTransport(t, b)
	factory := func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return shared, nil
	}
	o, e := clientNormalizeOptions(ClientOptions{TransportFactory: factory})
	if e != nil {
		t.Fatal(e)
	}
	// Match the repaired constructor's client-only composition. The exact old
	// disconnected-ledger witness remains archived under original/probes.
	initial := clientNewInitialOwners(o.Transport)
	authOptions := clientGuardAuthOptions(o.AuthOptions, initial.claim)
	auth, e := authOptions.TransportFactory(b, sdkTransport.TransportOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if e = auth.Close(); e != nil {
		t.Fatal(e)
	}
	// This is the repaired caller-side adoption after the local auth scope has
	// returned a ready bundle; no public successful PIN branch is executed.
	c, e := clientNewSberClient(b, o, initial)
	if e == nil {
		defer c.Close()
		_, sendError := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
		if !errors.Is(sendError, sdkErrs.ErrClosed) {
			t.Fatal("independent closed-transport control confounded")
		}
		t.Error("initial business handoff accepted an already-closed auth owner")
	}
	if _, _, n, _ := shared.counts(); n < 1 {
		t.Fatal("auth-close control did not execute")
	}
}

const i3StartJSON = `{"success":true,"body":{"pid":"i3-workflow","result":"SUCCESS","flow":"me2meCreate","state":"transferRequisites","output":{"references":{"fromResource":{"items":[{"value":"transactionAccount:3101","properties":{"type":"payAccount","name":"synthetic-current","currency":"RUB"}}]},"toResource":{"items":[{"value":"card:3404","properties":{"type":"card","name":"synthetic-card","currency":"RUB"}}]}}}}}`

func TestIndependentClient3BoundWorkflowPreservesCancellationIdentity(t *testing.T) {
	for _, stage := range []string{"success-control", "start", "prepare", "confirm-entry", "confirm-return", "confirm-final"} {
		for _, kind := range []string{"canceled", "deadline"} {
			t.Run(stage+"/"+kind, func(t *testing.T) {
				b := i3Bundle(t, "bound-cancellation")
				tr := i3NewTransport(t, b)
				var financial atomic.Int32
				cause := context.Canceled
				if kind == "deadline" {
					cause = context.DeadlineExceeded
				}
				tr.post = func(_ context.Context, target string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					u, e := url.Parse(target)
					if e != nil {
						t.Fatal(e)
					}
					if u.Path == ProductsPath {
						return i3Response(200, i3PortfolioJSON), nil
					}
					financial.Add(1)
					name := u.Query().Get("name")
					targetStage := map[string]string{"me2meMain_v2": "start", "next": "prepare", "summaryNext": "confirm-entry", "on-enter": "confirm-return", "on-return": "confirm-final"}[name]
					if targetStage == stage {
						return nil, cause
					}
					switch name {
					case "me2meMain_v2":
						return i3Response(200, i3StartJSON), nil
					case "next":
						return i3Response(200, `{"success":true,"body":{"pid":"i3-workflow","result":"SUCCESS","flow":"me2meCreate","state":"summary"}}`), nil
					case "summaryNext":
						return i3Response(200, `{"success":true,"body":{"pid":"i3-workflow","result":"EXTERNAL_ENTER","url":"/bh-confirmation/v3/workflow2"}}`), nil
					case "on-enter":
						return i3Response(200, `{"success":true,"body":{"pid":"i3-workflow","result":"EXTERNAL_RETURN","url":"/me2me/v1/workflow"}}`), nil
					case "on-return":
						return i3Response(200, `{"success":true,"body":{"pid":"i3-workflow","result":"SUCCESS","flow":"me2meInfo","state":"showInfo","output":{"document":{"srcDocumentId":"i3-doc"},"screens":[{"header":[{"properties":{"level":"done"}}]}]}}}`), nil
					default:
						t.Error("unexpected financial transition")
						return nil, errors.New("unexpected synthetic transition")
					}
				}
				c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
				p, e := c.Portfolio(context.Background(), false)
				if e != nil {
					t.Fatal(e)
				}
				d, e := c.Transfers().Start(context.Background())
				var prepared PreparedTransfer
				if e == nil {
					amount, pe := ParseDecimal("10.50")
					if pe != nil {
						t.Fatal(pe)
					}
					prepared, e = p.Transfers().Prepare(context.Background(), d, "transactionAccount:3101", "card:3404", amount)
				}
				if e == nil {
					_, e = p.Transfers().Confirm(context.Background(), prepared)
				}
				if stage == "success-control" {
					if e != nil {
						t.Fatal("successful synthetic workflow control failed", e)
					}
					if financial.Load() != 5 {
						t.Fatal("successful control not five captured transitions")
					}
					return
				}
				var uncertain *sdkErrs.MutationUncertain
				if !errors.As(e, &uncertain) {
					t.Fatal("bound workflow lost uncertainty")
				}
				if !errors.Is(e, cause) {
					t.Error("cached resource wrapper dropped already-sent cancellation identity")
				}
				before := financial.Load()
				later, e := c.Portfolio(context.Background(), true)
				if e != nil {
					t.Fatal(e)
				}
				if later.Transfers() != p.Transfers() {
					t.Fatal("later snapshot rebuilt workflow owner")
				}
				switch stage {
				case "start":
					_, e = later.Transfers().Start(context.Background())
				case "prepare":
					amount, _ := ParseDecimal("10.50")
					_, e = later.Transfers().Prepare(context.Background(), d, "transactionAccount:3101", "card:3404", amount)
				default:
					_, e = later.Transfers().Confirm(context.Background(), prepared)
				}
				if e == nil || financial.Load() != before {
					t.Fatal("cached alias replayed canceled workflow")
				}
			})
		}
	}
}
