package bank

import (
	"context"
	"errors"
	"net/url"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// An injected transport classification is not a validated API response. The
// client's after-send uncertainty dominates the nested rejection classification
// at the cached workflow guard, or a second snapshot could resend START.
func TestClientCycle4AfterSendClassifiedRejectionCannotResetStartGuard(t *testing.T) {
	b := i3Bundle(t, "rejection-guard")
	tr := i3NewTransport(t, b)
	financial := 0
	tr.post = func(_ context.Context, target string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		u, e := url.Parse(target)
		if e != nil {
			t.Fatal(e)
		}
		if u.Path == ProductsPath {
			return i3Response(200, i3PortfolioJSON), nil
		}
		financial++
		return nil, &sdkErrs.APIRejected{Message: "synthetic private transport diagnostic"}
	}
	c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
	p, e := c.Portfolio(context.Background(), false)
	if e != nil {
		t.Fatal(e)
	}
	_, e = p.Transfers().Start(context.Background())
	var uncertain *sdkErrs.MutationUncertain
	if !errors.As(e, &uncertain) {
		t.Fatal("unknown after-send transport failure lost uncertainty")
	}
	later, e := c.Portfolio(context.Background(), true)
	if e != nil {
		t.Fatal(e)
	}
	if later.Transfers() != p.Transfers() {
		t.Fatal("snapshot replaced issuer")
	}
	_, e = later.Transfers().Start(context.Background())
	if e == nil || financial != 1 {
		t.Error("nested transport rejection bypassed uncertain START guard")
	}
}
