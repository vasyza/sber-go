package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// Actual client-decoded rejection metadata, not a standalone foundation test.
// Explicit errors.As fields remain allowed; ordinary diagnostics must not read them.
func TestFreshClient4DecodedRejectionDiagnosticPrivacy(t *testing.T) {
	marker := strings.Join([]string{"fresh", "response", "metadata", "canary"}, "-")
	body, err := json.Marshal(map[string]any{"success": false, "error": map[string]any{"code": marker, "text": marker, "title": marker, "uuid": marker, "system": marker}})
	if err != nil {
		t.Fatal("synthetic response construction failed")
	}
	for _, route := range []string{"read", "raw-mutation", "bound-start", "sequence"} {
		t.Run(route, func(t *testing.T) {
			b := i3Bundle(t, "response-private")
			tr := i3NewTransport(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return i3Response(200, string(body)), nil
			}
			c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
			var got error
			switch route {
			case "read":
				_, got = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			case "raw-mutation":
				_, got = c.Mutate(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/fresh", true)
			case "sequence":
				got = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
					_, err := send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/fresh", true)
					return err
				})
			default:
				_, got = c.Transfers().Start(context.Background())
			}
			var rejected *sdkErrs.APIRejected
			if !errors.As(got, &rejected) || rejected.Code != marker || rejected.Text != marker {
				t.Fatal("explicit bounded rejection metadata control failed")
			}
			bad := "%w"
			control := struct{ Code string }{marker}
			if !strings.Contains(fmt.Sprintf(bad, control), marker) {
				t.Fatal("raw diagnostic negative control ineffective")
			}
			for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%p", "%w", "%#w", "%w %w"} {
				var buf bytes.Buffer
				log.New(&buf, "", 0).Printf(format, got)
				for _, text := range []string{fmt.Sprintf(format, got), fmt.Errorf(format, got).Error(), buf.String()} {
					if strings.Contains(text, marker) {
						t.Errorf("ordinary returned-outcome diagnostic exposes remote private metadata (format %s)", format)
					}
				}
			}
			data, e := json.Marshal(got)
			if e != nil || strings.Contains(string(data), marker) {
				t.Fatal("ordinary JSON exposes remote metadata")
			}
			if n, _, _, _ := tr.counts(); n != 1 {
				t.Fatal("diagnostic witness repeated request")
			}
		})
	}
}
