package sber

import (
	"context"
	"errors"
	"testing"
)

func TestClientTypedNilTransportFailsClosedWithoutPanic(t *testing.T) {
	b := clientFixture(t, "nil-transport")
	var absent *clientFakeTransport
	// Factories and injected interface values can carry typed nils. None may be
	// considered a usable transport or trigger a fallback connection.
	for _, o := range []ClientOptions{{Transport: absent}, {TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return absent, nil }}} {
		if c, e := NewSberClient(b, o); e == nil || c != nil {
			t.Fatal("typed nil accepted")
		}
	}
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return absent, nil }, Renewal: func(context.Context, SessionBundle, PINProvider, AuthOptions) (SessionBundle, error) { return b, nil }})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	var missing *MissingSession
	if !errors.As(e, &missing) {
		t.Fatal("typed nil renewal accepted")
	}
	calls, closes, _ := old.counts()
	if calls != 1 || closes != 0 {
		t.Fatal("typed nil replaced committed generation")
	}
}
