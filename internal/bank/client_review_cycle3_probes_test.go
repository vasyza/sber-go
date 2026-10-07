package bank

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// Independent review probes: only public client calls, existing synthetic
// fixture builders, and in-process transports. No credential POST or network.
// Failing assertions are the required safety contract, not deliberate mutants.
func TestReviewProbeClientIndirectDiagnosticsStayRedacted(t *testing.T) {
	b := clientFixture(t, "review-diagnostic-cookie-canary")
	b.Browser.Headers = []sdkSession.BrowserHeader{{Name: "user-agent", Value: "review-diagnostic-browser-canary"}}
	tr := clientFake(t, b)
	opts := ClientOptions{Transport: tr, SessionPath: "review-diagnostic-path-canary"}
	c, err := NewSberClient(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Reflect indirection models log adapters which receive the concrete
	// exported value. No client method is invoked on the copy, and no unsafe
	// reflection or private field access is used.
	indirect := reflect.ValueOf(c).Elem().Interface()
	cases := []struct {
		name  string
		value any
		verb  string
	}{
		{"pointer-fmt-v", c, "%v"}, {"pointer-fmt-sharp", c, "%#v"},
		{"pointer-unsupported-d", c, "%d"}, {"options-indirect", reflect.ValueOf(&opts).Elem().Interface(), "%#v"},
		{"client-indirect-v", indirect, "%v"}, {"client-indirect-plus", indirect, "%+v"},
		{"client-indirect-sharp", indirect, "%#v"}, {"client-indirect-unsupported-d", indirect, "%d"},
		{"client-indirect-slog", indirect, "slog"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out string
			if tc.verb == "slog" {
				var buf bytes.Buffer
				slog.New(slog.NewTextHandler(&buf, nil)).Info("synthetic diagnostic", slog.Any("client", tc.value))
				out = buf.String()
			} else {
				out = fmt.Sprintf(tc.verb, tc.value)
			}
			cookieLeak := strings.Contains(out, "session-review-diagnostic-cookie-canary") || strings.Contains(out, "token-review-diagnostic-cookie-canary")
			headerLeak := strings.Contains(out, "review-diagnostic-browser-canary")
			pathLeak := strings.Contains(out, "review-diagnostic-path-canary")
			t.Logf("cookie_leak=%t browser_leak=%t path_leak=%t", cookieLeak, headerLeak, pathLeak)
			if cookieLeak || headerLeak || pathLeak {
				t.Fatal("client-owned secret state leaked through ordinary indirect diagnostic")
			}
		})
	}
}

func TestReviewProbeSequenceKeepsUncertaintyWhenCallbackErrors(t *testing.T) {
	for _, kind := range []string{"foreign-error", "callback-canceled", "callback-sdk-error"} {
		t.Run(kind, func(t *testing.T) {
			b := clientFixture(t, "review-sequence")
			b.AntifraudDeviceprint = sdkTransport.PtrString("review-antifraud-canary")
			tr := clientFake(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return nil, context.Canceled
			}
			c, err := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			err = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
				_, first := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
				var u *sdkErrs.MutationUncertain
				if !errors.As(first, &u) {
					t.Fatal("send did not establish uncertainty")
				}
				_, second := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
				if !errors.As(second, &u) {
					t.Fatal("poisoned sender forgot failure")
				}
				switch kind {
				case "callback-canceled":
					return context.Canceled
				case "callback-sdk-error":
					return &sdkErrs.MissingSession{Message: "review-callback-private-canary"}
				default:
					return errors.New("review-callback-private-canary")
				}
			})
			calls, _, _ := tr.counts()
			var uncertain *sdkErrs.MutationUncertain
			keepUncertain := errors.As(err, &uncertain)
			keepCancel := errors.Is(err, context.Canceled)
			t.Logf("business_posts=%d mutation_uncertain=%t send_cancel_identity=%t", calls, keepUncertain, keepCancel)
			if calls != 1 {
				t.Fatal("financial POST replayed")
			}
			if !keepUncertain || !keepCancel {
				t.Fatal("sequence discarded established after-send uncertainty/cancellation for unrelated callback error")
			}
		})
	}
}

// All methods are real Transport methods promoted from the existing fixture.
// This is a supported Go interface implementation whose dynamic type contains
// a slice, so it is non-comparable although its underlying owner is the same.
type reviewValueTransport struct {
	*clientFakeTransport
	nonComparable  []byte
	blockedRetries *atomic.Int32
}

func (v reviewValueTransport) Post(ctx context.Context, target string, payload map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	_, _, closed := v.clientFakeTransport.counts()
	if closed > 0 {
		v.blockedRetries.Add(1)
		return nil, sdkErrs.ErrClosed
	}
	return v.clientFakeTransport.Post(ctx, target, payload, o)
}

func TestReviewProbeReusedNonComparableTransportFailsClosed(t *testing.T) {
	b := clientFixture(t, "review-value-old")
	fresh := clientFixture(t, "review-value-new")
	path := clientPrivatePath(t)
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	base := clientFake(t, b)
	base.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var blocked atomic.Int32
	same := reviewValueTransport{clientFakeTransport: base, nonComparable: []byte{1}, blockedRetries: &blocked}
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{
		Transport: same,
		Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
			return fresh, nil
		},
		TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			return same, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, readErr := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := c.ExportSession()
	if err != nil {
		t.Fatal(err)
	}
	_, closesBefore, _ := base.counts()
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	_, closesAfter, _ := base.counts()
	var transportErr *sdkErrs.TransportError
	rejectedReuse := errors.As(readErr, &transportErr) && transportErr.Code == "reused_transport"
	profileChanged := !bytes.Equal(before, after)
	metadataChanged := exported.APIBase != b.APIBase
	t.Logf("non_comparable=%t reused_rejected=%t profile_changed=%t metadata_changed=%t blocked_retry=%d retired_closes=%d total_closes=%d", !reflect.TypeOf(same).Comparable(), rejectedReuse, profileChanged, metadataChanged, blocked.Load(), closesBefore, closesAfter)
	if !rejectedReuse || profileChanged || metadataChanged || closesBefore != 0 || closesAfter != 1 {
		t.Fatal("same non-comparable transport was committed and closed as both current and retired generation")
	}
}

func TestReviewProbeDefaultPINAuthCannotCloseCommittedTransport(t *testing.T) {
	for _, kind := range []string{"explicit-auth-transport", "auth-factory-reuse"} {
		t.Run(kind, func(t *testing.T) {
			b := clientFixture(t, "review-auth-owner")
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
			authOpts := sdkAuth.AuthOptions{}
			if kind == "explicit-auth-transport" {
				authOpts.Transport = old
			} else {
				authOpts.TransportFactory = func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					return old, nil
				}
			}
			c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: authOpts})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, readErr := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			_, closesBefore, _ := old.counts()
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			calls, closesAfter, _ := old.counts()
			postCount, getCount := 0, 0
			for _, call := range old.snapshotCalls() {
				if call.method == "POST" {
					postCount++
				} else {
					getCount++
				}
			}
			t.Logf("error_returned=%t old_closed_before_commit=%d old_total_closes=%d calls=%d business_posts=%d bootstrap_gets=%d profile_changed=%t", readErr != nil, closesBefore, closesAfter, calls, postCount, getCount, !bytes.Equal(before, after))
			if postCount != 1 || getCount > 1 {
				t.Fatal("unexpected synthetic requests")
			}
			if closesBefore != 0 || closesAfter != 1 {
				t.Fatal("failed default PIN renewal closed the still-committed business owner through the auth options")
			}
		})
	}
}

func TestReviewProbeOutOfRangeExpiredHARCookieCannotBecomeSessionCookie(t *testing.T) {
	for _, expiry := range []string{"-1", "-9223372036854775809", "-1e30"} {
		t.Run(strings.ReplaceAll(expiry, "-", "neg"), func(t *testing.T) {
			raw := `{"log":{"entries":[{"request":{"method":"GET","url":"https://web2.online.sberbank.ru/main"},"response":{"cookies":[{"name":"SID","value":"review-expired-cookie-canary","domain":"online.sberbank.ru","path":"/","secure":true,"expires":` + expiry + `}] }},{"request":{"method":"POST","url":"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}}]}}`
			path := clientPrivatePath(t)
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			builds := 0
			c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				builds++
				return clientFake(t, b), nil
			}})
			accepted := c != nil && err == nil
			nilExpiry := false
			if c != nil {
				defer c.Close()
				b, exportErr := c.ExportSession()
				if exportErr != nil {
					t.Fatal(exportErr)
				}
				nilExpiry = len(b.Cookies) == 1 && b.Cookies[0].Expires == nil
			}
			t.Logf("accepted=%t factory_builds=%d expired_cookie_has_nil_expiry=%t", accepted, builds, nilExpiry)
			if accepted || builds != 0 {
				t.Fatal("valid numeric expired HAR timestamp was silently converted to a live session cookie")
			}
		})
	}
}
