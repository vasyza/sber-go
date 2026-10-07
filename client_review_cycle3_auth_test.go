package sber

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
)

func TestClientCycle3DefaultAuthRejectsBorrowedClosingOwners(t *testing.T) {
	for _, readiness := range []string{"ready", "unready"} {
		for _, route := range []string{"direct", "factory", "factory-error", "value-owner", "runtime-noncomparable", "inherited-business-factory"} {
			t.Run(readiness+"/"+route, func(t *testing.T) {
				b := clientFixture(t, "cycle3-auth-reserved")
				if readiness == "unready" {
					b.Cookies = nil
				}
				path := clientPrivatePath(t)
				if err := b.Save(path); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				old := clientFake(t, b)
				old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
					return clientResponse(401, `{}`), nil
				}
				old.get = func(context.Context, string, RequestOptions) (*Response, error) {
					return clientResponse(403, `synthetic bootstrap rejection`), nil
				}
				opts := ClientOptions{Transport: old}
				var candidate Transport = old
				if route == "value-owner" {
					candidate = clientCycle3ValueOwner{old, []byte{1}}
				}
				if route == "runtime-noncomparable" {
					candidate = clientCycle3InterfaceValue{old, []byte{1}}
				}
				if route == "direct" {
					opts.AuthOptions.Transport = candidate
				} else if route == "inherited-business-factory" {
					opts.TransportFactory = func(SessionBundle, TransportOptions) (Transport, error) { return candidate, nil }
				} else {
					opts.AuthOptions.TransportFactory = func(SessionBundle, TransportOptions) (Transport, error) {
						if route == "factory-error" {
							return candidate, errors.New("synthetic-private-factory-canary")
						}
						return candidate, nil
					}
				}
				c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, opts)
				if readiness == "ready" {
					if err != nil {
						t.Fatal(err)
					}
					defer c.Close()
					_, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
				} else if c != nil {
					defer c.Close()
					t.Error("unready auth ownership accepted")
				}
				var transport *TransportError
				if !errors.As(err, &transport) || transport.Code != "reused_transport" {
					t.Error("borrowed auth closing owner was not rejected")
				}
				after, readErr := os.ReadFile(path)
				if readErr != nil || !bytes.Equal(before, after) {
					t.Error("borrowed auth published a profile")
				}
				if _, closes, _ := old.counts(); closes != 0 {
					t.Error("borrowed owner closed on auth failure")
				}
				posts, gets := 0, 0
				for _, call := range old.snapshotCalls() {
					if call.method == "POST" {
						posts++
					} else {
						gets++
					}
				}
				expectedPosts := 0
				if readiness == "ready" {
					expectedPosts = 1
				}
				if posts != expectedPosts || gets != 0 {
					t.Error("rejected auth owner reached bootstrap or credential/business replay")
				}
				if c != nil {
					if err = c.Close(); err != nil {
						t.Fatal(err)
					}
					if _, closes, _ := old.counts(); closes != 1 {
						t.Error("borrowed business owner double closed")
					}
				}
			})
		}
	}
}

func TestClientCycle3FreshAuthFactoryErrorsKeepRetryableCleanup(t *testing.T) {
	for _, route := range []string{"factory-error", "nil-jar"} {
		t.Run(route, func(t *testing.T) {
			b := clientFixture(t, "cycle3-fresh-auth")
			path := clientPrivatePath(t)
			if err := b.Save(path); err != nil {
				t.Fatal(err)
			}
			old, candidate := clientFake(t, b), clientFake(t, b)
			old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
				return clientResponse(401, `{}`), nil
			}
			if route == "nil-jar" {
				candidate.jar = nil
			}
			var attempts atomic.Int32
			candidate.close = func() error {
				if attempts.Add(1) == 1 {
					return errors.New("synthetic close failure")
				}
				return nil
			}
			c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: AuthOptions{TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) {
				if route == "factory-error" {
					return candidate, errors.New("synthetic factory failure")
				}
				return candidate, nil
			}}})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			var cleanup *ClientCleanupError
			if !errors.As(err, &cleanup) || attempts.Load() != 1 {
				t.Fatal("fresh failed auth construction leaked retryable closing ownership")
			}
			if err = cleanup.Close(); err != nil {
				t.Fatal(err)
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			if attempts.Load() != 2 {
				t.Fatal("auth cleanup retried or double closed")
			}
			if n, _, _ := candidate.counts(); n != 0 {
				t.Fatal("invalid auth candidate performed I/O")
			}
		})
	}
}

func TestClientCycle3CustomRenewalRetainsExplicitOptions(t *testing.T) {
	b := clientFixture(t, "cycle3-custom-auth")
	path := clientPrivatePath(t)
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	old, next, unused := clientFake(t, b), clientFake(t, b), clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	called := false
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic", nil }, ClientOptions{Transport: old, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return next, nil }, AuthOptions: AuthOptions{Transport: unused}, Renewal: func(_ context.Context, b SessionBundle, _ PINProvider, ao AuthOptions) (SessionBundle, error) {
		called = true
		if ao.Transport != unused {
			t.Error("custom renewal injection replaced")
		}
		return b, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); err != nil || !called {
		t.Fatal("custom renewal disabled", err)
	}
	if n, closes, _ := unused.counts(); n != 0 || closes != 0 {
		t.Fatal("client claimed unused custom auth transport")
	}
}
