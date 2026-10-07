package bank

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientDefaultRenewalUsesAuthenticationConnectionPolicy(t *testing.T) {
	for _, tt := range []struct {
		name        string
		auth        bool
		connections int64
	}{
		{"business", false, 3},
		{"authentication", true, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var connections, requests atomic.Int64
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				_, _ = io.WriteString(w, "synthetic")
			}))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
			}
			server.StartTLS()
			defer server.Close()
			ca := filepath.Join(filepath.Dir(clientPrivatePath(t)), "local-ca.pem")
			if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			options, err := clientNormalizeOptions(ClientOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: ca, Timeout: 3 * time.Second}})
			if err != nil {
				t.Fatal(err)
			}
			var transport sdkTransport.Transport
			bundle := clientFixture(t, "connection-policy")
			if tt.auth {
				transport, err = options.AuthOptions.TransportFactory(bundle, options.AuthOptions.TransportOptions)
			} else {
				transport, err = options.TransportFactory(bundle, options.TransportOptions)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer transport.Close()
			ctx := context.Background()
			if _, err := transport.Get(ctx, server.URL, sdkTransport.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := transport.PostForm(ctx, server.URL, map[string]string{"synthetic": "proof"}, sdkTransport.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if _, err := transport.Post(ctx, server.URL, nil, sdkTransport.RequestOptions{}); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 3 || connections.Load() != tt.connections {
				t.Fatalf("requests=%d connections=%d; want 3 requests over %d connections", requests.Load(), connections.Load(), tt.connections)
			}
		})
	}
}

func TestClientTypedNilTransportFailsClosedWithoutPanic(t *testing.T) {
	b := clientFixture(t, "nil-transport")
	var absent *clientFakeTransport
	// Factories and injected interface values can carry typed nils. None may be
	// considered a usable transport or trigger a fallback connection.
	for _, o := range []ClientOptions{{Transport: absent}, {TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return absent, nil
	}}} {
		if c, e := NewSberClient(b, o); e == nil || c != nil {
			t.Fatal("typed nil accepted")
		}
	}
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return absent, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
		return b, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var missing *sdkErrs.MissingSession
	if !errors.As(e, &missing) {
		t.Fatal("typed nil renewal accepted")
	}
	calls, closes, _ := old.counts()
	if calls != 1 || closes != 0 {
		t.Fatal("typed nil replaced committed generation")
	}
}

func TestClientCycle3OptionsRetainExistingExportedFieldLayout(t *testing.T) {
	typ := reflect.TypeOf(ClientOptions{})
	expected := []string{"Transport", "TransportFactory", "TransportOptions", "AllowMutations", "SessionPath", "Monotonic", "Renewal", "AuthOptions", "BrowserBootstrap", "BrowserBootstrapTimeout"}
	if typ.NumField() != len(expected) {
		t.Fatal("ClientOptions gained a field, breaking prior positional literals")
	}
	for i, name := range expected {
		field := typ.Field(i)
		if field.Name != name || field.PkgPath != "" {
			t.Fatal("ClientOptions public layout changed")
		}
	}
}

func TestClientCycle3OpaqueCopiesShareCloseAndCachedWorkflowLifetime(t *testing.T) {
	b := clientFixture(t, "cycle3-copy-lifetime")
	tr := clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	copy := reflect.ValueOf(c).Elem().Interface().(SberClient)
	if copy.Products() != c.Products() || copy.Transfers() != c.Transfers() {
		t.Fatal("handle copy rebuilt Resources/workflow issuer")
	}
	if err = copy.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = c.ExportSession(); !errors.Is(err, sdkErrs.ErrClosed) {
		t.Fatal("copy close did not close the original lifetime")
	}
	if _, err = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); !errors.Is(err, sdkErrs.ErrClosed) {
		t.Fatal("copy close resurrected original")
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if n, closes, _ := tr.counts(); n != 0 || closes != 1 {
		t.Fatal("copy close duplicated owner cleanup")
	}
}

// Opt-in wrappers expose a stable terminal closing handle, not raw fields or
// cookie state. This structural method compiles even before the client gate.
type clientCycle3ValueOwner struct {
	*clientFakeTransport
	metadata []byte
}

func (v clientCycle3ValueOwner) ClientTransportOwner() sdkTransport.Transport {
	return v.clientFakeTransport
}

type clientCycle3InterfaceValue struct {
	*clientFakeTransport
	metadata any
}

type clientCycle3NilOwner struct {
	*clientFakeTransport
	metadata []byte
}

func (v clientCycle3NilOwner) ClientTransportOwner() sdkTransport.Transport {
	return (*clientFakeTransport)(nil)
}

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
			old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(401, `{}`), nil
			}
			var initial, candidate sdkTransport.Transport
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
			c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic", nil }, ClientOptions{Transport: initial, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return candidate, factoryError
			}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
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
			var transport *sdkErrs.TransportError
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
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
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
	next.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		saved, err := sdkSession.LoadSessionBundle(path)
		if err != nil || saved.APIBase != fresh.APIBase {
			t.Error("read retry preceded persistence")
		}
		return clientResponse(200, `{"success":true}`), nil
	}
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "synthetic", nil }, ClientOptions{Transport: clientCycle3ValueOwner{old, []byte{1}}, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return clientCycle3ValueOwner{next, []byte{2}}, nil
	}, Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
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
