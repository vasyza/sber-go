package bank

import (
	"context"
	"errors"
	"net/http"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientAnyHTMLContentTypeAliasRejectsLoginResponse(t *testing.T) {
	b := clientFixture(t, "header-alias")
	tr := clientFake(t, b)
	tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		r := clientResponse(200, `{"success":true}`)
		r.Headers = http.Header{"Content-Type": []string{"application/json"}, "content-type": []string{"TEXT/HTML"}}
		return r, nil
	}
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	for i := 0; i < 32; i++ {
		_, e = c.PostRead(context.Background(), "/uoh-bh/v1/operation/details", nil)
		var expired *sdkErrs.AuthenticationExpired
		if !errors.As(e, &expired) {
			t.Fatal("ambiguous header let login response through")
		}
	}
}
