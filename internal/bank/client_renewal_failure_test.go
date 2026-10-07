package bank

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientRefreshFactoryReuseCannotRetireCommittedTransport(t *testing.T) {
	b := clientFixture(t, "retained")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	fresh := clientFixture(t, "replacement")
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return old, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var transport *sdkErrs.TransportError
	if !errors.As(e, &transport) || transport.Code != "reused_transport" {
		t.Fatal("factory reuse committed")
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("invalid replacement profile published")
	}
	exported, e := c.ExportSession()
	if e != nil || exported.APIBase != b.APIBase {
		t.Fatal("committed state changed on failed refresh")
	}
	calls, closes, _ := old.counts()
	if calls != 1 || closes != 0 {
		t.Fatal("old generation torn down after failed refresh")
	}
}

func TestClientFailedRefreshLeavesOldStateAndCleanupWaitsForClose(t *testing.T) {
	for _, mode := range []string{"provider", "renewal", "invalid-bundle", "factory", "factory-with-owned", "nil-jar", "save"} {
		t.Run(mode, func(t *testing.T) {
			b := clientFixture(t, "failure-old")
			path := clientPrivatePath(t)
			if e := b.Save(path); e != nil {
				t.Fatal(e)
			}
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			old := clientFake(t, b)
			old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(401, `{}`), nil
			}
			fresh := clientFixture(t, "failure-new")
			candidate := clientFake(t, fresh)
			var closeAttempts atomic.Int32
			candidate.close = func() error {
				if closeAttempts.Add(1) == 1 {
					return errors.New("synthetic close failure")
				}
				return nil
			}
			c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) {
				if mode == "provider" {
					return "", errors.New("synthetic provider failure")
				}
				return "13579", nil
			}, ClientOptions{Transport: old,
				Renewal: func(ctx context.Context, _ sdkSession.SessionBundle, p sdkAuth.PINProvider, _ sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
					if _, e := p(ctx); e != nil {
						return sdkSession.SessionBundle{}, e
					}
					switch mode {
					case "renewal":
						return sdkSession.SessionBundle{}, &sdkErrs.PinAuthError{Code: "fixture_failure"}
					case "invalid-bundle":
						fresh.APIBase = "https://external.invalid"
					case "save":
						if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
							t.Fatal(e)
						}
					}
					return fresh, nil
				},
				TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					switch mode {
					case "factory":
						return nil, errors.New("synthetic factory failure")
					case "factory-with-owned":
						return candidate, errors.New("synthetic factory failure")
					case "nil-jar":
						candidate.jar = nil
					}
					return candidate, nil
				},
			})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e == nil {
				t.Fatal("failed renewal accepted")
			}
			after, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("failed refresh wrote profile")
			}
			exported, e := c.ExportSession()
			if e != nil || exported.APIBase != b.APIBase {
				t.Fatal("failed refresh changed state")
			}
			_, oldCloses, _ := old.counts()
			if oldCloses != 0 {
				t.Fatal("old transport closed before commit")
			}
			expected := 0
			if mode == "factory-with-owned" || mode == "nil-jar" || mode == "save" {
				expected = 1
			}
			if int(closeAttempts.Load()) != expected {
				t.Fatal("unexpected candidate cleanup")
			}
			if e = c.Close(); e != nil {
				t.Fatal(e)
			}
			if int(closeAttempts.Load()) != expected*2 {
				t.Fatal("failed cleanup not retried exclusively by Close")
			}
		})
	}
}
