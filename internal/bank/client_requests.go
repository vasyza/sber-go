package bank

import (
	"bytes"
	"context"
	"strings"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

const clientWarmUpPath = "/api/warmUpSession"

func clientReadPath(p string) bool {
	switch p {
	case "/main-screen/rest/v2/m1/web/section/meta", "/uoh-bh/v1/operations/list", "/uoh-bh/v1/operation/details", "/ufs-carddetail/rest/card/v1/cardInfo", "/pfpv_alf_mb/v1.00/alf/amounts":
		return true
	}
	return false
}

// Business decoding calls the shared strict DecodeJSON on original Content;
// Response.Text/JSON are deliberately not used (they are lossy/permissive).
func clientDecodeResponse(r *sdkTransport.Response, path string) (map[string]any, error) {
	return clientDecodeResponseOutcome(r, path, false)
}

// Reads retain the canonical source gate. After a mutation send, only an
// explicit validated false success flag establishes definite API rejection.
func clientDecodeResponseOutcome(r *sdkTransport.Response, path string, mutation bool) (map[string]any, error) {
	if r == nil {
		return nil, &sdkErrs.APIError{Message: "missing response"}
	}
	switch r.StatusCode {
	case 301, 302, 303, 307, 308, 401, 403:
		return nil, &sdkErrs.AuthenticationExpired{}
	}
	contentType := ""
	for n, values := range r.Headers {
		if strings.EqualFold(n, "content-type") {
			contentType += "," + strings.ToLower(strings.Join(values, ","))
		}
	}
	if path == clientWarmUpPath && (r.StatusCode == 200 || r.StatusCode == 204) {
		if strings.Contains(contentType, "text/html") {
			return nil, &sdkErrs.AuthenticationExpired{}
		}
		return map[string]any{}, nil
	}
	if r.StatusCode != 200 {
		return nil, &sdkErrs.APIError{StatusCode: r.StatusCode}
	}
	if strings.Contains(contentType, "text/html") {
		return nil, &sdkErrs.AuthenticationExpired{}
	}
	data, e := DecodeJSON(bytes.NewReader(r.Content))
	if e != nil || data == nil {
		return nil, &sdkErrs.APIError{StatusCode: r.StatusCode, Message: "invalid JSON object"}
	}
	success, definiteRejection := false, false
	switch x := data["success"].(type) {
	case bool:
		success, definiteRejection = x, !x
	case string:
		success, definiteRejection = x == "true", x == "false"
	}
	if !success {
		if mutation && !definiteRejection {
			return nil, &sdkErrs.APIError{StatusCode: r.StatusCode, Message: "unvalidated mutation outcome"}
		}
		details, _ := data["error"].(map[string]any)
		return nil, &sdkErrs.APIRejected{Message: "API rejected request", Code: clientBoundedErrorField(details, "code", 128), Title: clientBoundedErrorField(details, "title", 4096), Text: clientBoundedErrorField(details, "text", 4096), UUID: clientBoundedErrorField(details, "uuid", 256), System: clientBoundedErrorField(details, "system", 256)}
	}
	return data, nil
}
func clientBoundedErrorField(data map[string]any, key string, limit int) string {
	s, ok := data[key].(string)
	if !ok || s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > limit {
		return ""
	}
	for _, r := range s {
		if r < 0x20 && r != '	' && r != '\n' || r == 0x7f {
			return ""
		}
	}
	return s
}
func clientSafeError(e error) error {
	remaining := 64
	return clientCopyError(e, &remaining)
}
func (c *SberClient) check(ctx context.Context) error {
	c.core().mu.Lock()
	closed := c.core().closed
	c.core().mu.Unlock()
	if closed {
		return sdkErrs.ErrClosed
	}
	if ctx == nil {
		return &sdkErrs.TransportError{Code: "invalid_context"}
	}
	if ctx.Err() != nil {
		return sdkTransport.TransportFailure(ctx.Err())
	}
	return nil
}
func (c *SberClient) enter(ctx context.Context) (context.Context, func(), error) {
	if e := c.check(ctx); e != nil {
		return nil, nil, e
	}
	select {
	case c.core().gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, sdkTransport.TransportFailure(ctx.Err())
	case <-c.core().root.Done():
		return nil, nil, sdkErrs.ErrClosed
	}
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(c.core().root, cancel)
	done := func() { stop(); cancel(); <-c.core().gate }
	if e := c.check(child); e != nil {
		done()
		return nil, nil, e
	}
	return child, done, nil
}

// PostRead sends only an exact observed read endpoint. Business reads never
// implicitly warm up and no transport/network failure triggers a replay.
func (c *SberClient) PostRead(ctx context.Context, path string, payload map[string]any) (map[string]any, error) {
	if !clientReadPath(path) {
		return nil, &sdkErrs.TransportError{Code: "endpoint_not_allowed"}
	}
	var data map[string]any
	e := c.withReadRenewal(ctx, func(ctx context.Context) error {
		var err error
		data, err = c.postReadOnce(ctx, path, payload)
		return err
	})
	return data, clientDiagnostic(e)
}
func (c *SberClient) postReadOnce(ctx context.Context, path string, payload map[string]any) (map[string]any, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	base, e := c.resolveAPIBase(ctx)
	if e != nil {
		return nil, e
	}
	r, e := c.core().transport.Post(ctx, base+path, payload, sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd"})
	if e != nil {
		return nil, clientSafeError(e)
	}
	if e = c.check(ctx); e != nil {
		return nil, e
	}
	data, e := clientDecodeResponse(r, path)
	if e != nil {
		return nil, e
	}
	if e = c.saveRotatedSession(ctx); e != nil {
		return nil, e
	}
	return data, nil
}
