package sber

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestClientCycle3SequenceSenderFailureDominatesPrivateCallback(t *testing.T) {
	b := clientFixture(t, "cycle3-outcome")
	b.AntifraudDeviceprint = ptrString("synthetic-antifraud")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
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
	var uncertain *MutationUncertain
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
	b.AntifraudDeviceprint = ptrString("synthetic-antifraud")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
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
			var uncertain *MutationUncertain
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
