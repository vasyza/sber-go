package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func (f *authFlow) withContext(body map[string]any) map[string]any {
	body["deviceprint"] = *f.bundle.Deviceprint
	if f.options.IsPWA {
		body["isPwa"] = true
	}
	return body
}
func (f *authFlow) updateCSRF(r *sdkTransport.Response) error {
	if r == nil {
		return authFailure("invalid_json", r)
	}
	v := authResponseHeader(r, "X-CSRF-Token")
	if v != "" {
		if !authInput(v, 8192) {
			return authFailure("invalid_csrf", r)
		}
		f.csrf = v
	}
	return nil
}
func authResponseHeader(r *sdkTransport.Response, name string) string {
	if r != nil {
		for k, v := range r.Headers {
			if strings.EqualFold(k, name) && len(v) > 0 {
				return v[0]
			}
		}
	}
	return ""
}
func (f *authFlow) pinHeaders(c sdkSession.FrontendConfig) (sdkTransport.RequestOptions, error) {
	target, err := sdkSession.AuthEndpoint(c.BaseURL(), "/api/v1/pin/begin")
	if err != nil {
		return sdkTransport.RequestOptions{}, err
	}
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return sdkTransport.RequestOptions{}, authFailure("entropy_unavailable", nil)
	}
	v := hex.EncodeToString(raw)
	uid := v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:]
	h := sdkTransport.HeaderOverrides{"Accept": sdkTransport.PtrString("application/json, text/plain, */*"), "Content-Type": sdkTransport.PtrString("application/json"), "Origin": sdkTransport.PtrString(sdkSession.AppOrigin), "Referer": sdkTransport.PtrString(sdkTransport.PublicBootstrapURL), "Sec-Fetch-Dest": sdkTransport.PtrString("empty"), "Sec-Fetch-Mode": sdkTransport.PtrString("cors"), "Sec-Fetch-Site": sdkTransport.PtrString(authFetchSite(sdkSession.AppOrigin, target)), "X-Requested-With": nil, "Process-Id": sdkTransport.PtrString(c.ProcessID()), "Rq-Uid": sdkTransport.PtrString(uid), "X-TS-AJAX-Request": sdkTransport.PtrString("true")}
	if f.csrf != "" {
		h["X-CSRF-Token"] = sdkTransport.PtrString(f.csrf)
	}
	if authFetchSite(sdkSession.AppOrigin, target) == "same-origin" {
		u, _ := url.Parse(target)
		var xsrf string
		for _, c := range f.transport.CookieJar().RecordsForURL(u) {
			if c.Name == "XSRF-TOKEN" && !c.HTTPOnly {
				xsrf = c.Value
			}
		}
		if xsrf != "" {
			if decoded, err := url.PathUnescape(xsrf); err == nil {
				h["X-XSRF-TOKEN"] = sdkTransport.PtrString(decoded)
			}
		}
	}
	return sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: h}, nil
}
func authJSONObject(r *sdkTransport.Response) (map[string]any, error) {
	var p map[string]any
	if r == nil || json.Unmarshal(r.Content, &p) != nil || p == nil {
		return nil, authFailure("invalid_json", r)
	}
	return p, nil
}
func (f *authFlow) postJSON(ctx context.Context, c sdkSession.FrontendConfig, path string, body map[string]any) (map[string]any, error) {
	target, err := sdkSession.AuthEndpoint(c.BaseURL(), path)
	if err != nil {
		return nil, authRequestError(err)
	}
	o, err := f.pinHeaders(c)
	if err != nil {
		return nil, authRequestError(err)
	}
	r, e := f.transport.Post(ctx, target, body, o)
	if err = f.check(ctx); err != nil {
		return nil, authRequestError(err)
	}
	if e != nil {
		return nil, authRequestError(e)
	}
	if err = f.updateCSRF(r); err != nil {
		return nil, authRequestError(err)
	}
	p, err := authJSONObject(r)
	if err != nil {
		return nil, authRequestError(err)
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		code := ""
		if e, ok := p["error"].(map[string]any); ok {
			code, _ = e["code"].(string)
		}
		if code == "invalid_request" || (path == "/api/v1/pin/begin" && code == "service_unavailable") {
			f.resetProcess()
		}
		return nil, f.pinResponseError(p, r, c.BaseURL())
	}
	return p, nil
}
func authSeamlessHeaders(target string, redirect, seamless bool) sdkTransport.RequestOptions {
	h := sdkTransport.HeaderOverrides{"Accept": sdkTransport.PtrString("application/json, text/plain, */*"), "Content-Type": nil, "Origin": sdkTransport.PtrString(sdkSession.AppOrigin), "Referer": sdkTransport.PtrString(sdkSession.AppOrigin + "/"), "Sec-Fetch-Dest": sdkTransport.PtrString("empty"), "Sec-Fetch-Mode": sdkTransport.PtrString("cors"), "Sec-Fetch-Site": sdkTransport.PtrString(authFetchSite(sdkSession.AppOrigin, target)), "X-Seamless-Web": nil, "X-Requested-With": nil}
	if redirect {
		h["Accept"] = sdkTransport.PtrString("*/*")
		h["Content-Type"] = sdkTransport.PtrString("application/json; charset=utf-8")
		if seamless {
			h["X-Seamless-Web"] = sdkTransport.PtrString("true")
		}
	}
	return sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: h}
}
func (f *authFlow) getDocument(ctx context.Context, target, referer, code string) (*sdkTransport.Response, error) {
	r, e := f.transport.Get(ctx, target, authDocumentHeaders(referer, target))
	if err := f.check(ctx); err != nil {
		return nil, authRequestError(err)
	}
	if e != nil {
		return nil, authRequestError(e)
	}
	if r == nil || r.StatusCode != 200 {
		return nil, authFailure(code, r)
	}
	return r, nil
}
func (f *authFlow) finishRedirect(ctx context.Context, c sdkSession.FrontendConfig, redirect string) (sdkSession.SessionBundle, error) {
	current, err := sdkSession.SafeOnlineURL(redirect, sdkSession.AppOrigin+"/")
	if err != nil {
		return sdkSession.SessionBundle{}, authRequestError(err)
	}
	post := c.RedirectPost()
	var r *sdkTransport.Response
	finished := false
	for i := 0; i < sdkSession.MaxAuthRedirects; i++ {
		var e error
		if post {
			r, e = f.transport.Post(ctx, current, nil, authSeamlessHeaders(current, true, c.SeamlessWeb()))
		} else {
			r, e = f.transport.Get(ctx, current, authDocumentHeaders(sdkTransport.PublicBootstrapURL, current))
		}
		if err = f.check(ctx); err != nil {
			return sdkSession.SessionBundle{}, authRequestError(err)
		}
		if e != nil {
			return sdkSession.SessionBundle{}, authRequestError(e)
		}
		if r == nil {
			return sdkSession.SessionBundle{}, authFailure("redirect_failed", nil)
		}
		if sdkSession.IsAuthRedirectStatus(r.StatusCode) {
			current, post, err = sdkSession.ResolveAuthRedirect(current, r.StatusCode, authResponseHeader(r, "Location"), post)
			if err != nil {
				return sdkSession.SessionBundle{}, authRequestError(err)
			}
			continue
		}
		if r.StatusCode != 200 {
			return sdkSession.SessionBundle{}, authFailure("redirect_failed", r)
		}
		finished = true
		break
	}
	if !finished {
		return sdkSession.SessionBundle{}, authFailure("redirect_loop", nil)
	}
	effective := current
	if observed := authResponseHeader(r, "X-Response-URL"); observed != "" {
		effective, err = sdkSession.SafeOnlineURL(observed, current)
		if err != nil {
			return sdkSession.SessionBundle{}, authRequestError(err)
		}
	}
	api, found := sdkSession.APIBaseFromMainHTML(r.Text())
	web := ""
	if found {
		web = authOrigin(effective)
	} else {
		if current != sdkSession.AppOrigin+"/app/main" {
			current = sdkSession.AppOrigin + "/app/main"
			r, err = f.getDocument(ctx, current, sdkTransport.PublicBootstrapURL, "redirect_failed")
			if err != nil {
				return sdkSession.SessionBundle{}, authRequestError(err)
			}
		}
		var ok bool
		web, ok = sdkSession.UFSHostFromAppShell(r.Text())
		if !ok {
			return sdkSession.SessionBundle{}, authFailure("missing_ufs_host", r)
		}
		r, err = f.getDocument(ctx, web+"/main", current, "ufs_bootstrap_failed")
		if err != nil {
			return sdkSession.SessionBundle{}, authRequestError(err)
		}
		api, ok = sdkSession.APIBaseFromMainHTML(r.Text())
		if !ok {
			return sdkSession.SessionBundle{}, authFailure("missing_ufs_api_host", r)
		}
	}
	if c.SeamlessWeb() {
		ready := web + "/api/front/ready"
		r, e := f.transport.Get(ctx, ready, authSeamlessHeaders(ready, false, true))
		if err = f.check(ctx); err != nil {
			return sdkSession.SessionBundle{}, authRequestError(err)
		}
		if e != nil {
			return sdkSession.SessionBundle{}, authRequestError(e)
		}
		if r == nil || r.StatusCode != 200 {
			return sdkSession.SessionBundle{}, authFailure("ufs_ready_failed", r)
		}
	}
	snapshot := f.bundle
	snapshot.APIBase = api
	snapshot.WebBase = web
	snapshot, err = snapshot.WithCookieJar(f.transport.CookieJar())
	if err != nil {
		return sdkSession.SessionBundle{}, authRequestError(err)
	}
	if _, err = sdkSession.CredentialsFromBundle(snapshot); err != nil {
		return sdkSession.SessionBundle{}, authFailure("missing_ufs_session", nil)
	}
	if _, err = snapshot.ToSeed(true); err != nil {
		return sdkSession.SessionBundle{}, authFailure("missing_ufs_session", nil)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return sdkSession.SessionBundle{}, sdkErrs.ErrClosed
	}
	if ctx.Err() != nil {
		return sdkSession.SessionBundle{}, sdkTransport.TransportFailure(ctx.Err())
	}
	f.bundle = snapshot
	f.srp = nil
	f.authenticated = true
	f.otpPending = false
	f.primaryOTPPending = false
	f.primarySRP = nil
	f.primaryToken = ""
	f.pinPublicKey = ""
	f.csrf = ""
	return snapshot.Clone()
}
