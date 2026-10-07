package bank

import (
	"context"
	"errors"
	"sync"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

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
