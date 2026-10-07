package bank

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
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
