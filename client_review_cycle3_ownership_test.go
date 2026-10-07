package sber

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

// Opt-in wrappers expose a stable terminal closing handle, not raw fields or
// cookie state. This structural method compiles even before the client gate.
type clientCycle3ValueOwner struct {
	*clientFakeTransport
	metadata []byte
}

func (v clientCycle3ValueOwner) ClientTransportOwner() Transport { return v.clientFakeTransport }

type clientCycle3InterfaceValue struct {
	*clientFakeTransport
	metadata any
}

type clientCycle3NilOwner struct {
	*clientFakeTransport
	metadata []byte
}

func (v clientCycle3NilOwner) ClientTransportOwner() Transport { return (*clientFakeTransport)(nil) }

func TestClientCycle3ClosingIdentityRejectsAliasesBeforePublication(t *testing.T) {
	for _, kind := range []string{"identity-value", "value-to-pointer", "pointer-to-value", "runtime-noncomparable", "unverifiable-fresh", "nil-owner", "alias-with-error"} {
		t.Run(kind, func(t *testing.T) {
			b, fresh := clientFixture(t, "cycle3-owner-old"), clientFixture(t, "cycle3-owner-new")
			path := clientPrivatePath(t)
			if err := b.Save(path); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			old, next := clientFake(t, b), clientFake(t, fresh)
			old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
				return clientResponse(401, `{}`), nil
			}
			var initial, candidate Transport
			initial = clientCycle3ValueOwner{old, []byte{1}}
			candidate = clientCycle3ValueOwner{old, []byte{2}}
			var factoryError error
			switch kind {
			case "value-to-pointer":
				candidate = old
			case "pointer-to-value":
				initial = old
			case "runtime-noncomparable":
				initial = clientCycle3InterfaceValue{old, []byte{1}}
				candidate = initial
			case "unverifiable-fresh":
				initial = old
				candidate = reviewValueTransport{clientFakeTransport: next, nonComparable: []byte{2}}
			case "nil-owner":
				initial = old
				candidate = clientCycle3NilOwner{next, []byte{1}}
			case "alias-with-error":
				factoryError = errors.New("cycle3-private-factory-canary")
			}
			c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic", nil }, ClientOptions{Transport: initial, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return candidate, factoryError }, Renewal: func(context.Context, SessionBundle, PINProvider, AuthOptions) (SessionBundle, error) {
				return fresh, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			// A comparable struct type may hold a non-comparable interface
			// value. Equality must not panic while evaluating ownership.
			var outcome error
			func() {
				defer func() {
					if recover() != nil {
						t.Error("ownership comparison panicked")
					}
				}()
				_, outcome = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			}()
			var transport *TransportError
			if !errors.As(outcome, &transport) || transport.Code != "reused_transport" {
				t.Error("alias/unverifiable closing owner was accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Error("unsafe candidate persisted profile")
			}
			exported, err := c.ExportSession()
			if err != nil || exported.APIBase != b.APIBase {
				t.Error("unsafe candidate published metadata")
			}
			if _, closes, _ := old.counts(); closes != 0 {
				t.Error("borrowed committed owner closed")
			}
			if _, closes, _ := next.counts(); closes != 0 {
				t.Error("unverifiable candidate was claimed/closed")
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			if _, closes, _ := old.counts(); closes != 1 {
				t.Error("committed owner double closed")
			}
		})
	}
}

func TestClientCycle3FreshValueClosingOwnersKeepRenewalAndCloseRetry(t *testing.T) {
	b, fresh := clientFixture(t, "cycle3-fresh-old"), clientFixture(t, "cycle3-fresh-new")
	path := clientPrivatePath(t)
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	old, next := clientFake(t, b), clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	attempts := 0
	old.close = func() error {
		attempts++
		if attempts == 1 {
			return errors.New("synthetic retryable close")
		}
		return nil
	}
	next.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		saved, err := LoadSessionBundle(path)
		if err != nil || saved.APIBase != fresh.APIBase {
			t.Error("read retry preceded persistence")
		}
		return clientResponse(200, `{"success":true}`), nil
	}
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic", nil }, ClientOptions{Transport: clientCycle3ValueOwner{old, []byte{1}}, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) {
		return clientCycle3ValueOwner{next, []byte{2}}, nil
	}, Renewal: func(context.Context, SessionBundle, PINProvider, AuthOptions) (SessionBundle, error) {
		return fresh, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	issuer, products := c.Transfers(), c.Products()
	if _, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || c.Transfers() != issuer || c.Products() != products {
		t.Fatal("fresh owning value changed cached binding or retirement")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatal("failed owner retirement not retried by Close")
	}
	if n, closes, _ := next.counts(); n != 1 || closes != 1 {
		t.Fatal("fresh value lifetime changed")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatal("successful Close replayed")
	}
}
