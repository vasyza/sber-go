package bank

import (
	"bytes"
	"context"
	"errors"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientCycle4AfterSendOutcomesDistinguishDefiniteRejection(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		uncertain  bool
	}{
		{"truncated", `{"success":true`, 200, true},
		{"duplicate", `{"success":true,"success":true}`, 200, true},
		{"trailing", `{"success":true} {}`, 200, true},
		{"surrogate", `{"success":true,"id":"\ud800"}`, 200, true},
		{"null-document", `null`, 200, true},
		{"missing-success", `{}`, 200, true},
		{"null-success", `{"success":null}`, 200, true},
		{"numeric-success", `{"success":1}`, 200, true},
		{"unknown-success", `{"success":"unknown"}`, 200, true},
		{"gateway", `{"success":false}`, 502, true},
		{"expired", `{}`, 401, true},
		{"redirect", `{}`, 307, true},
		{"boolean-rejection", `{"success":false}`, 200, false},
		{"string-rejection", `{"success":"false","error":{"code":"5"}}`, 200, false},
	}
	for _, tc := range cases {
		for _, route := range []string{"direct", "sequence"} {
			t.Run(tc.name+"/"+route, func(t *testing.T) {
				b := i3Bundle(t, "outcome")
				tr := i3NewTransport(t, b)
				response := i3Response(tc.status, tc.body)
				before := append([]byte(nil), response.Content...)
				tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return response, nil
				}
				c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
				var first, got error
				if route == "direct" {
					_, got = c.Mutate(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
				} else {
					got = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
						_, first = send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
						_, next := send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
						if next != first {
							t.Error("recorded outcome replaced or financial POST replayed")
						}
						return errors.New("unrelated synthetic callback")
					})
					if got != first {
						t.Error("sequence replaced its safe recorded outcome")
					}
				}
				var uncertain *sdkErrs.MutationUncertain
				var rejected *sdkErrs.APIRejected
				if errors.As(got, &uncertain) != tc.uncertain {
					t.Error("after-send outcome classification wrong")
				}
				if !tc.uncertain && !errors.As(got, &rejected) {
					t.Error("validated definite rejection lost")
				}
				if tc.uncertain && errors.As(got, &rejected) {
					t.Error("unvalidated response fabricated definite rejection")
				}
				if n, _, _, _ := tr.counts(); n != 1 {
					t.Error("financial POST attempted more or less than once")
				}
				if !bytes.Equal(before, response.Content) {
					t.Error("original response bytes modified")
				}
			})
		}
	}
}
func TestClientCycle4PreSendValidationDoesNotInventUncertainty(t *testing.T) {
	for _, mode := range []string{"disabled", "bad-path", "bad-page", "missing-identity", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			b := i3Bundle(t, "presend")
			if mode == "missing-identity" {
				b.AntifraudDeviceprint = nil
			}
			tr := i3NewTransport(t, b)
			c := i3Client(t, b, tr, ClientOptions{AllowMutations: mode != "disabled"})
			path, page := Me2MeWorkflowPath, "/app/test"
			ctx := context.Background()
			if mode == "bad-path" {
				path = "/not-allowed"
			}
			if mode == "bad-page" {
				page = "invalid"
			}
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, got := c.Mutate(ctx, path, nil, nil, page, true)
			var uncertain *sdkErrs.MutationUncertain
			if got == nil || errors.As(got, &uncertain) {
				t.Error("validation lost or pre-send failure mislabeled")
			}
			if n, _, _, _ := tr.counts(); n != 0 {
				t.Error("pre-send validation attempted POST")
			}
		})
	}
}
