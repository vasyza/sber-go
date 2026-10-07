package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestClientMutationPersistenceFailureIsUncertainAndNeverReplayed(t *testing.T) {
	b := clientFixture(t, "write-save")
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	path := clientPrivatePath(t)
	tr := clientFake(t, b)
	if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true, SessionPath: path})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.Mutate(context.Background(), "/ufs-productdetail/rest/v1/changeProductName", nil, nil, "/app/card", false)
	var uncertain *sdkErrs.MutationUncertain
	var insecure *sdkErrs.InsecureSessionFile
	if !errors.As(e, &uncertain) || !errors.As(e, &insecure) {
		t.Fatal("accepted-write persistence uncertainty hidden")
	}
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatal("write replayed after persistence failure")
	}
}
func TestClientMutationCancellationDistinguishesBeforeAndAfterSend(t *testing.T) {
	for _, where := range []string{"before", "discovery", "after-send"} {
		t.Run(where, func(t *testing.T) {
			b := clientFixture(t, "cancel-write")
			b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
			if where == "discovery" {
				b.APIBase = sdkSession.AppOrigin
				b.WebBase = sdkSession.AppOrigin
			}
			tr := clientFake(t, b)
			started := make(chan struct{})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wait := func(ctx context.Context) (*sdkTransport.Response, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			if where == "discovery" {
				tr.get = func(ctx context.Context, _ string, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return wait(ctx)
				}
			} else {
				tr.post = func(ctx context.Context, _ string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return wait(ctx)
				}
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			done := make(chan error, 1)
			if where == "before" {
				cancel()
			}
			go func() { _, e := c.Mutate(ctx, "/me2me/v1/workflow", nil, nil, "/app/test", true); done <- e }()
			if where != "before" {
				select {
				case <-started:
				case e := <-done:
					t.Fatalf("request phase not entered: %v", e)
				}
				cancel()
			}
			e = <-done
			var uncertain *sdkErrs.MutationUncertain
			if !errors.Is(e, context.Canceled) {
				t.Fatal("cancel identity hidden")
			}
			if errors.As(e, &uncertain) != (where == "after-send") {
				t.Fatal("wrong send uncertainty classification")
			}
			calls := tr.snapshotCalls()
			if where == "before" && len(calls) != 0 || where != "before" && len(calls) != 1 {
				t.Fatal("cancel repeated request")
			}
			if where == "discovery" && calls[0].method != "GET" {
				t.Fatal("business mutation sent during canceled discovery")
			}
		})
	}
}

// This is deliberately private and structurally duplicates only the required
// coordinator signature, not the resource worker's BusinessRequester type.
type clientRequestContract interface {
	PostRead(context.Context, string, map[string]any) (map[string]any, error)
	Mutate(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
	MutationSequence(context.Context, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error
	ExportSession() (sdkSession.SessionBundle, error)
	ExportCredentials() (sdkSession.SberCredentials, error)
	WarmUp(context.Context, bool) error
	Close() error
}

var _ clientRequestContract = (*SberClient)(nil)

func TestClientMutationsAreExplicitOneShotWithObservedIdentity(t *testing.T) {
	b := clientFixture(t, "mutation")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Mutate(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/transfers", true); e == nil {
		t.Fatal("default mutation enabled")
	}
	c.Close()
	calls, _, _ := tr.counts()
	if calls != 0 {
		t.Fatal("policy not fail-fast")
	}
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	tr = clientFake(t, b)
	c, e = NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for _, path := range []string{"/ufs-productdetail/rest/v1/changeProductName", "/me2me/v1/workflow", "/bh-confirmation/v3/workflow2"} {
		if _, e = c.Mutate(context.Background(), path, map[string]any{"fixture": true}, map[string]string{"name": "next step", "opaque": "a&b"}, "/app/transfers", true); e != nil {
			t.Fatal(e)
		}
	}
	for _, call := range tr.snapshotCalls() {
		u, e := url.Parse(call.target)
		if e != nil || u.Query().Get("name") != "next step" || u.Query().Get("opaque") != "a&b" {
			t.Fatal("query not encoded")
		}
		h := call.options.Headers
		if h["RSA-Antifraud-Device-Print"] == nil || *h["RSA-Antifraud-Device-Print"] != *b.AntifraudDeviceprint || *h["RSA-Antifraud-Page-Id"] != "/app/transfers" || *h["X-Workflow-Options"] != "3.0" {
			t.Fatal("observed headers lost")
		}
	}
	for _, path := range []string{"/api/warmUpSession", "/uoh-bh/v1/operations/list", "/not-observed", "/me2me/v1/workflow?name=next"} {
		if _, e = c.Mutate(context.Background(), path, nil, nil, "/app/test", false); e == nil {
			t.Fatal("mutation path bypass")
		}
	}
	for _, page := range []string{"", "absolute", "/app/\r\nCookie:secret", "/app/\x00", "/app/\x7f"} {
		if _, e = c.Mutate(context.Background(), "/me2me/v1/workflow", nil, nil, page, false); e == nil {
			t.Fatal("unsafe page identity")
		}
	}
	calls, _, _ = tr.counts()
	if calls != 3 {
		t.Fatal("invalid mutation sent")
	}
}
func TestClientMutationWithoutAntifraudFailsBeforeEvenDiscovery(t *testing.T) {
	b, _ := (sdkSession.SberCredentials{UFSSession: "session-fixture", UFSToken: "token-fixture"}).ToBundle(sdkSession.CredentialsBundleOptions{})
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.Mutate(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", false)
	var missing *sdkErrs.MissingSession
	if !errors.As(e, &missing) {
		t.Fatal("identity absence hidden")
	}
	calls, _, _ := tr.counts()
	if calls != 0 {
		t.Fatal("network before missing identity rejection")
	}
}
func TestClientMutationNeverReauthenticatesOrReplaysAnyFailure(t *testing.T) {
	for _, mode := range []string{"401", "403", "redirect", "rejected", "server", "bad-json", "network", "canceled", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			b := clientFixture(t, "once")
			b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
			path := clientPrivatePath(t)
			if e := b.Save(path); e != nil {
				t.Fatal(e)
			}
			tr := clientFake(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				switch mode {
				case "401":
					return clientResponse(401, `{}`), nil
				case "403":
					return clientResponse(403, `{}`), nil
				case "redirect":
					return clientResponse(307, `{}`), nil
				case "rejected":
					return clientResponse(200, `{"success":false}`), nil
				case "server":
					return clientResponse(502, `{}`), nil
				case "bad-json":
					return clientResponse(200, `{"success":true,"x":"\ud800"}`), nil
				case "network":
					return nil, errors.New("connection after send")
				case "canceled":
					return nil, context.Canceled
				case "deadline":
					return nil, context.DeadlineExceeded
				}
				panic("unreachable")
			}
			c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { t.Error("mutation called PIN provider"); return "", nil }, ClientOptions{Transport: tr, AllowMutations: true, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
				t.Error("mutation reauthenticated")
				return sdkSession.SessionBundle{}, errors.New("forbidden")
			}})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, e = c.Mutate(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
			if e == nil {
				t.Fatal("mutation failure hidden")
			}
			calls, _, _ := tr.counts()
			if calls != 1 {
				t.Fatal("mutation repeated")
			}
			if mode == "network" || mode == "canceled" || mode == "deadline" {
				var uncertain *sdkErrs.MutationUncertain
				if !errors.As(e, &uncertain) {
					t.Fatal("send uncertainty not surfaced")
				}
			}
			if mode == "canceled" && !errors.Is(e, context.Canceled) || mode == "deadline" && !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("cancellation identity lost")
			}
		})
	}
}

func TestClientReadDecodesOriginalBytesAndEnforcesSourceGate(t *testing.T) {
	cases := []struct {
		name, body, ctype, kind string
		status                  int
	}{
		{"boolean", `{"success":true,"n":9007199254740993}`, "application/json", "ok", 200},
		{"string", `{"success":"true"}`, "application/json", "ok", 200},
		{"numeric", `{"success":1}`, "application/json", "rejected", 200},
		{"false", `{"success":false}`, "application/json", "rejected", 200},
		{"missing", `{}`, "application/json", "rejected", 200},
		{"nested", `{"success":true,"value":{"x":1,"x":2}}`, "application/json", "api", 200},
		{"escaped-key", `{"success":false,"\u0073uccess":true}`, "application/json", "api", 200},
		{"surrogate", `{"success":true,"id":"\ud800"}`, "application/json", "api", 200},
		{"surrogate-low", `{"success":true,"id":"\udfff"}`, "application/json", "api", 200},
		{"invalid-utf8", string([]byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'}), "application/json", "api", 200},
		{"trailing", `{"success":true} {}`, "application/json", "api", 200},
		{"list", `[]`, "application/json", "api", 200},
		{"null", `null`, "application/json", "api", 200},
		{"malformed", `{`, "application/json", "api", 200},
		{"html", `<html>secret</html>`, "TEXT/HTML; charset=utf-8", "expired", 200},
		{"bad-gateway-html", `<html>secret</html>`, "text/html", "api", 502},
		{"no-content-read", ``, "application/json", "api", 204},
		{"301", ``, "text/html", "expired", 301}, {"302", ``, "", "expired", 302},
		{"303", ``, "", "expired", 303}, {"307", ``, "", "expired", 307}, {"308", ``, "", "expired", 308},
		{"401", ``, "", "expired", 401}, {"403", ``, "", "expired", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := clientFixture(t, "read")
			tr := clientFake(t, b)
			r := clientResponse(tc.status, tc.body)
			r.Headers = http.Header{"cOnTeNt-TyPe": []string{tc.ctype}}
			original := append([]byte(nil), r.Content...)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return r, nil
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			got, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", map[string]any{"paginationOffset": 0})
			var api *sdkErrs.APIError
			var rejected *sdkErrs.APIRejected
			var expired *sdkErrs.AuthenticationExpired
			switch tc.kind {
			case "ok":
				if e != nil || got["success"] == nil {
					t.Fatalf("expected success: %v", e)
				}
				if tc.name == "boolean" && got["n"] != json.Number("9007199254740993") {
					t.Fatal("number rounded")
				}
			case "api":
				if !errors.As(e, &api) {
					t.Fatalf("expected API error: %v", e)
				}
			case "rejected":
				if !errors.As(e, &rejected) {
					t.Fatalf("expected rejected: %v", e)
				}
			case "expired":
				if !errors.As(e, &expired) {
					t.Fatalf("expected expired: %v", e)
				}
			}
			if !bytes.Equal(r.Content, original) {
				t.Fatal("response rewritten")
			}
			calls := tr.snapshotCalls()
			if len(calls) != 1 || calls[0].method != "POST" || calls[0].target != b.APIBase+"/uoh-bh/v1/operations/list" || calls[0].options.AcceptEncoding != "gzip, deflate, br, zstd" {
				t.Fatal("request changed or repeated")
			}
		})
	}
	b := clientFixture(t, "allowlist")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	paths := []string{"/main-screen/rest/v2/m1/web/section/meta", "/uoh-bh/v1/operations/list", "/uoh-bh/v1/operation/details", "/ufs-carddetail/rest/card/v1/cardInfo", "/pfpv_alf_mb/v1.00/alf/amounts"}
	for _, p := range paths {
		if _, e := c.PostRead(context.Background(), p, map[string]any{}); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []string{"/api/warmUpSession", "/me2me/v1/workflow", "/ufs-productdetail/rest/v1/changeProductName", "/bh-confirmation/v3/workflow2", "/uoh-bh/v1/operations/list?injected=x", "https://other.invalid/", ""} {
		if _, e := c.PostRead(context.Background(), p, nil); e == nil {
			t.Fatal("noncanonical path accepted")
		}
	}
	calls, _, _ := tr.counts()
	if calls != len(paths) {
		t.Fatal("allowlist side effect")
	}
}

func TestClientCycle3SequenceSenderFailureDominatesPrivateCallback(t *testing.T) {
	b := clientFixture(t, "cycle3-outcome")
	b.AntifraudDeviceprint = sdkTransport.PtrString("synthetic-antifraud")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return nil, context.Canceled
	}
	c, err := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	private := errors.New("cycle3-private-callback-canary")
	var first error
	err = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		_, first = send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
		return private
	})
	var uncertain *sdkErrs.MutationUncertain
	if !errors.As(err, &uncertain) || !errors.Is(err, context.Canceled) || err != first {
		t.Fatal("recorded terminal send result was replaced")
	}
	if errors.Is(err, private) {
		t.Fatal("foreign callback error retained in terminal result")
	}
	for _, format := range []string{"%v", "%+v", "%#v", "%d", "%w"} {
		if strings.Contains(fmt.Sprintf(format, err), "cycle3-private-callback-canary") {
			t.Fatal("private callback leaked")
		}
	}
	if n, _, _ := tr.counts(); n != 1 {
		t.Fatal("mutation replayed")
	}
}

func TestClientCycle3SequencePanicInvalidatesFailedCapability(t *testing.T) {
	b := clientFixture(t, "cycle3-panic")
	b.AntifraudDeviceprint = sdkTransport.PtrString("synthetic-antifraud")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return nil, context.Canceled
	}
	c, err := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var escaped func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("callback panic suppressed")
			}
		}()
		_ = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
			escaped = send
			_, err := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
			var uncertain *sdkErrs.MutationUncertain
			if !errors.As(err, &uncertain) || !errors.Is(err, context.Canceled) {
				t.Fatal("send classification changed")
			}
			panic("synthetic callback panic")
		})
	}()
	if _, err := escaped(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true); err == nil {
		t.Fatal("panic capability escaped")
	}
	if n, _, _ := tr.counts(); n != 1 {
		t.Fatal("panic replayed mutation")
	}
	// The operation gate must be released even during panic unwinding.
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestClientMutationSequenceOwnsCapabilityAndExcludesReads(t *testing.T) {
	b := clientFixture(t, "sequence")
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	paused := make(chan struct{})
	release := make(chan struct{})
	sequenceDone := make(chan error, 1)
	var escaped func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
	go func() {
		sequenceDone <- c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
			escaped = send
			for i := 0; i < 3; i++ {
				if _, e := send(context.Background(), "/me2me/v1/workflow", map[string]any{"step": i}, nil, "/app/transfers", true); e != nil {
					return e
				}
				if i == 0 {
					close(paused)
					<-release
				}
			}
			return nil
		})
	}()
	select {
	case <-paused:
	case e := <-sequenceDone:
		t.Fatalf("sequence did not execute: %v", e)
	}
	ctx, queued := clientQueue()
	readDone := make(chan error, 1)
	go func() { _, e := c.PostRead(ctx, "/uoh-bh/v1/operations/list", nil); readDone <- e }()
	<-queued
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatal("read interleaved before sequence ended")
	}
	close(release)
	if e = <-sequenceDone; e != nil {
		t.Fatal(e)
	}
	if e = <-readDone; e != nil {
		t.Fatal(e)
	}
	calls = 0
	for i, call := range tr.snapshotCalls() {
		if i < 3 && call.target != b.APIBase+"/me2me/v1/workflow" || i == 3 && call.target != b.APIBase+"/uoh-bh/v1/operations/list" {
			t.Fatal("interleaved sequence")
		}
		calls++
	}
	if calls != 4 {
		t.Fatal("sequence request repeated")
	}
	if _, e = escaped(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", false); e == nil {
		t.Fatal("sender escaped callback lifetime")
	}
	after, _, _ := tr.counts()
	if after != calls {
		t.Fatal("escaped sender performed write")
	}
	// All scope-owned sends validate exactly the same policy and page identity.
	if e = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		_, e := send(context.Background(), "/me2me/v1/workflow", nil, nil, "invalid", false)
		return e
	}); e == nil {
		t.Fatal("in-sequence validation bypass")
	}
}
func TestClientMutationSequenceAbortCannotBeSwallowedOrReplay(t *testing.T) {
	b := clientFixture(t, "abort")
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return nil, errors.New("raw synthetic uncertain transport")
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	e = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		_, _ = send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/transfers", true)
		_, _ = send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/transfers", true)
		return nil
	})
	var uncertain *sdkErrs.MutationUncertain
	if !errors.As(e, &uncertain) {
		t.Fatal("sequence swallowed send uncertainty")
	}
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatal("sequence replayed failed write")
	}
}
func TestClientMutationSequenceSerializesConcurrentSendersAndHonorsOuterCancellation(t *testing.T) {
	b := clientFixture(t, "parallel-scope")
	b.AntifraudDeviceprint = sdkTransport.PtrString("version%3Dfixture")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	e = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, e := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/transfers", true); e != nil {
					t.Error(e)
				}
			}()
		}
		wg.Wait()
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	e = c.MutationSequence(ctx, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		called = true
		return nil
	})
	if called || !errors.Is(e, context.Canceled) {
		t.Fatal("canceled sequence started")
	}
	calls, _, _ := tr.counts()
	if calls != 8 {
		t.Fatal("extra writes")
	}
}
