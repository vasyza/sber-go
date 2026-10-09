package auth

import (
	"context"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	sdkSession "github.com/vasyza/sber-sdk/internal/session"
)

// uapiChannel uses the native client identity. It does not claim a rendered
// browser, copy protection cookies, or run JavaScript.
func (f *authFlow) uapiChannel(store *bool) map[string]any {
	d := map[string]any{"rsa_data": map[string]any{"deviceprint": *f.bundle.Deviceprint, "js_events": "", "manvsmachinedetection": "", "dom_elements": "", "htmlinjection": ""}, "browser": "Go", "os": runtime.GOOS}
	if store != nil {
		d["set_cookie"] = *store
	}
	if f.options.IsPWA {
		d["is_pwa"] = true
	}
	return map[string]any{"type": "web_sbol", "data": d}
}
func encodeAuthCaptcha(value string) string {
	parts := make([]string, 0, len(value))
	for _, r := range strings.ToUpper(value) {
		parts = append(parts, strconv.Itoa(int(r)))
	}
	return strings.Join(parts, "_")
}
func uapiIdentifier(kind, token string) map[string]any {
	return map[string]any{"type": kind, "data": map[string]any{"value": token}}
}
func (f *authFlow) postUAPI(ctx context.Context, path string, body map[string]any) (map[string]any, error) {
	c := *f.config
	target, e := sdkSession.AuthEndpoint(c.BaseURL(), path)
	if e != nil {
		return nil, e
	}
	opts, e := f.pinHeaders(c)
	if e != nil {
		return nil, e
	}
	r, e := f.transport.Post(ctx, target, body, opts)
	if ce := f.check(ctx); ce != nil {
		return nil, authRequestError(ce)
	}
	if e != nil {
		return nil, authRequestError(e)
	}
	if e = f.updateCSRF(r); e != nil {
		return nil, e
	}
	if r.StatusCode == 204 && path == "/uapi/v2/getOperation" {
		return map[string]any{}, nil
	}
	p, e := authJSONObject(r)
	if e != nil {
		return nil, e
	}
	remote, _ := p["error"].(map[string]any)
	if r.StatusCode < 200 || r.StatusCode >= 300 || remote != nil {
		return p, f.pinResponseError(p, r, c.BaseURL())
	}
	return p, nil
}
func (f *authFlow) finishUAPI(ctx context.Context, p map[string]any) (*sdkSession.SessionBundle, error) {
	d, ok := p["response_data"].(map[string]any)
	if !ok {
		return nil, authFailure("missing_redirect", nil)
	}
	host, ok := d["host"].(string)
	if !ok || !authInput(host, 8192) {
		return nil, authFailure("missing_redirect", nil)
	}
	safe, e := sdkSession.SafeOnlineURL(host, sdkSession.AppOrigin+"/")
	if e != nil {
		return nil, e
	}
	u, e := url.Parse(safe)
	if e != nil {
		return nil, authFailure("missing_redirect", nil)
	}
	if raw, present := d["authcode"]; present {
		code, ok := raw.(string)
		if !ok || !authInput(code, 8192) {
			return nil, authFailure("invalid_auth_token", nil)
		}
		q := u.Query()
		if q.Has("AuthToken") {
			return nil, authFailure("invalid_auth_token", nil)
		}
		q.Set("AuthToken", code)
		u.RawQuery = q.Encode()
	}
	b, e := f.finishRedirect(ctx, *f.config, u.String())
	if e != nil {
		return nil, e
	}
	return &b, nil
}
func authenticationDigits(value string) (string, bool) {
	var out strings.Builder
	for _, c := range value {
		switch {
		case c >= '0' && c <= '9':
			out.WriteRune(c)
		case c == ' ' || c == '-' || c == '(' || c == ')':
		case c == '+' && out.Len() == 0:
		default:
			return "", false
		}
	}
	return out.String(), true
}
