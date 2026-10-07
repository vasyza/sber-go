package bank

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientWarmUpIsExplicitDebouncedAndForceable(t *testing.T) {
	b := clientFixture(t, "warm")
	tr := clientFake(t, b)
	now := time.Unix(1000, 0)
	tr.post = func(_ context.Context, u string, p map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		if u != b.WebBase+"/api/warmUpSession" || len(p) != 0 {
			t.Error("wrong warmup request")
		}
		return clientResponse(204, "not JSON; allowed"), nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, Monotonic: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := c.WarmUp(context.Background(), false); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	calls, _, _ := tr.counts()
	if calls != 1 {
		t.Fatalf("not debounced: %d", calls)
	}
	now = now.Add(59 * time.Second)
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	calls, _, _ = tr.counts()
	if calls != 1 {
		t.Fatal("premature warmup")
	}
	now = now.Add(time.Second)
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	calls, _, _ = tr.counts()
	if calls != 4 {
		t.Fatalf("force ignored: %d", calls)
	}
}
func TestClientWarmUpGateAndPersistenceMustSucceedBeforeDebounce(t *testing.T) {
	for _, tc := range []struct {
		name, kind, ctype string
		status            int
	}{
		{"empty-200", "ok", "application/json", 200}, {"204", "ok", "", 204}, {"html-200", "expired", "text/html", 200}, {"html-204", "expired", "text/html", 204}, {"redirect", "expired", "", 302}, {"503-html", "api", "text/html", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := clientFixture(t, "warm-gate")
			tr := clientFake(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				r := clientResponse(tc.status, "not JSON")
				r.Headers = http.Header{"Content-Type": []string{tc.ctype}}
				return r, nil
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			e = c.WarmUp(context.Background(), false)
			var api *sdkErrs.APIError
			var expired *sdkErrs.AuthenticationExpired
			if tc.kind == "ok" && e != nil || tc.kind == "api" && !errors.As(e, &api) || tc.kind == "expired" && !errors.As(e, &expired) {
				t.Fatalf("wrong warmup gate: %v", e)
			}
			_ = c.WarmUp(context.Background(), false)
			calls, _, _ := tr.counts()
			want := 1
			if tc.kind != "ok" {
				want = 2
			}
			if calls != want {
				t.Fatal("failed warmup debounced")
			}
		})
	}
	b := clientFixture(t, "warm-persist")
	tr := clientFake(t, b)
	path := clientPrivatePath(t)
	if e := os.Chmod(filepath.Dir(path), 0755); e != nil {
		t.Fatal(e)
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr, SessionPath: path})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.WarmUp(context.Background(), false); e == nil {
		t.Fatal("save error hidden")
	}
	if e = c.WarmUp(context.Background(), false); e == nil {
		t.Fatal("failed save marked warm")
	}
	calls, _, _ := tr.counts()
	if calls != 2 {
		t.Fatal("save failure skipped warmup")
	}
	if e := os.Chmod(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e = c.WarmUp(context.Background(), false); e != nil {
		t.Fatal(e)
	}
	if _, e = sdkSession.LoadSessionBundle(path); e != nil {
		t.Fatal("warmup did not save")
	}
}
