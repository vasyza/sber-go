package sber

import (
	"context"
	"errors"
	"testing"
)

func TestClientPINFactoryCancellationCannotReturnALiveClient(t *testing.T) {
	b := clientFixture(t, "cancel-factory")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	tr := clientFake(t, b)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, e := NewSberClientFromPINProfile(ctx, path, func(context.Context) (string, error) { t.Error("ready profile logged in"); return "", nil }, ClientOptions{TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { cancel(); return tr, nil }})
	if c != nil {
		defer c.Close()
	}
	if !errors.Is(e, context.Canceled) {
		t.Fatal("canceled constructor returned success")
	}
	calls, closes, _ := tr.counts()
	if calls != 0 || closes != 1 {
		t.Fatal("canceled constructor leaked transport")
	}
}
func TestClientMutationSequenceRetainsCancellationAndUncertainty(t *testing.T) {
	b := clientFixture(t, "scope-cancel")
	b.AntifraudDeviceprint = ptrString("version%3Dfixture")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return nil, context.Canceled
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	e = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		_, e := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", false)
		return e
	})
	var uncertain *MutationUncertain
	if !errors.Is(e, context.Canceled) || !errors.As(e, &uncertain) {
		t.Fatal("scope collapsed after-send uncertainty")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e = c.MutationSequence(ctx, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		cancel()
		return nil
	})
	if !errors.Is(e, context.Canceled) {
		t.Fatal("canceled scope silently succeeded")
	}
}
