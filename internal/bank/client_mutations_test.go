package bank

import (
	"context"
	"errors"
	"net/url"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

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
