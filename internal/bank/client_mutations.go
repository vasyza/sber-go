package bank

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func clientMutationPath(path string) bool {
	switch path {
	case "/ufs-productdetail/rest/v1/changeProductName", "/me2me/v1/workflow", "/bh-confirmation/v3/workflow2":
		return true
	}
	return false
}
func (c *SberClient) validateMutation(path, page string) error {
	if !c.core().options.AllowMutations {
		return &sdkErrs.TransportError{Code: "mutation_disabled"}
	}
	if !clientMutationPath(path) {
		return &sdkErrs.TransportError{Code: "endpoint_not_allowed"}
	}
	if !utf8.ValidString(page) || !strings.HasPrefix(page, "/") || sdkSession.HasControls(page) {
		return &sdkErrs.TransportError{Code: "unsafe_page_id"}
	}
	return nil
}

// Mutate makes exactly one business POST attempt. It never triggers PIN renewal,
// network retries, redirects, or replay. After-send transport/cancel/save failures
// carry MutationUncertain: the caller must verify the outcome, not retry blindly.
func (c *SberClient) Mutate(ctx context.Context, path string, payload map[string]any, query map[string]string, pageID string, workflow bool) (map[string]any, error) {
	if e := c.validateMutation(path, pageID); e != nil {
		return nil, e
	}
	ctx, done, e := c.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	data, err := c.mutateOnce(ctx, path, payload, query, pageID, workflow)
	return data, clientDiagnostic(err)
}
func (c *SberClient) mutateOnce(ctx context.Context, path string, payload map[string]any, query map[string]string, pageID string, workflow bool) (map[string]any, error) {
	if e := c.validateMutation(path, pageID); e != nil {
		return nil, e
	}
	if e := c.check(ctx); e != nil {
		return nil, e
	}
	deviceprint := c.core().bundle.AntifraudDeviceprint
	if deviceprint == nil {
		return nil, &sdkErrs.MissingSession{Message: "antifraud device identity required"}
	}
	headers := sdkTransport.HeaderOverrides{"RSA-Antifraud-Device-Print": sdkSession.CloneString(deviceprint), "RSA-Antifraud-Page-Id": sdkTransport.PtrString(pageID)}
	if workflow {
		headers["X-Workflow-Options"] = sdkTransport.PtrString("3.0")
	}
	base, e := c.resolveAPIBase(ctx)
	if e != nil {
		return nil, e
	}
	target := base + path
	if len(query) != 0 {
		q := url.Values{}
		for k, v := range query {
			q.Set(k, v)
		}
		target += "?" + q.Encode()
	}
	if e = c.check(ctx); e != nil {
		return nil, e
	}
	if payload == nil {
		payload = map[string]any{}
	}
	r, e := c.core().transport.Post(ctx, target, payload, sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: headers})
	if e != nil {
		return nil, errors.Join(&sdkErrs.MutationUncertain{Message: "mutation send failed; outcome unknown"}, clientSafeError(e))
	}
	if e = c.check(ctx); e != nil {
		return nil, errors.Join(&sdkErrs.MutationUncertain{Message: "mutation canceled after send"}, e)
	}
	data, e := clientDecodeResponseOutcome(r, path, true)
	if e != nil {
		// Only our validated definite rejection proves a known result. Every
		// other response/availability failure occurred after the one send.
		if _, rejected := e.(*sdkErrs.APIRejected); rejected {
			return nil, e
		}
		return nil, errors.Join(&sdkErrs.MutationUncertain{Message: "mutation response unavailable; outcome unknown"}, clientSafeError(e))
	}
	if e = c.saveRotatedSession(ctx); e != nil {
		return nil, errors.Join(&sdkErrs.MutationUncertain{Message: "mutation accepted; session persistence uncertain"}, clientSafeError(e))
	}
	return data, nil
}
