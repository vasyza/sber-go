package sber

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
)

func (f *authFlow) withContext(body map[string]any) map[string]any {
	body["deviceprint"] = *f.bundle.Deviceprint
	if f.options.IsPWA {
		body["isPwa"] = true
	}
	return body
}
func (f *authFlow) updateCSRF(r *Response) error {
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
func authResponseHeader(r *Response, name string) string {
	if r != nil {
		for k, v := range r.Headers {
			if strings.EqualFold(k, name) && len(v) > 0 {
				return v[0]
			}
		}
	}
	return ""
}
func (f *authFlow) pinHeaders(c FrontendConfig) (RequestOptions, error) {
	target, err := AuthEndpoint(c.BaseURL(), "/api/v1/pin/begin")
	if err != nil {
		return RequestOptions{}, err
	}
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return RequestOptions{}, authFailure("entropy_unavailable", nil)
	}
	v := hex.EncodeToString(raw)
	uid := v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:]
	h := HeaderOverrides{"Accept": ptrString("application/json, text/plain, */*"), "Content-Type": ptrString("application/json"), "Origin": ptrString(AppOrigin), "Referer": ptrString(PublicBootstrapURL), "Sec-Fetch-Dest": ptrString("empty"), "Sec-Fetch-Mode": ptrString("cors"), "Sec-Fetch-Site": ptrString(authFetchSite(AppOrigin, target)), "X-Requested-With": nil, "Process-Id": ptrString(c.ProcessID()), "Rq-Uid": ptrString(uid), "X-TS-AJAX-Request": ptrString("true")}
	if f.csrf != "" {
		h["X-CSRF-Token"] = ptrString(f.csrf)
	}
	if authFetchSite(AppOrigin, target) == "same-origin" {
		u, _ := url.Parse(target)
		var xsrf string
		for _, c := range f.transport.CookieJar().RecordsForURL(u) {
			if c.Name == "XSRF-TOKEN" && !c.HTTPOnly {
				xsrf = c.Value
			}
		}
		if xsrf != "" {
			if decoded, err := url.PathUnescape(xsrf); err == nil {
				h["X-XSRF-TOKEN"] = ptrString(decoded)
			}
		}
	}
	return RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: h}, nil
}
func authJSONObject(r *Response) (map[string]any, error) {
	var p map[string]any
	if r == nil || json.Unmarshal(r.Content, &p) != nil || p == nil {
		return nil, authFailure("invalid_json", r)
	}
	return p, nil
}
func (f *authFlow) postJSON(ctx context.Context, c FrontendConfig, path string, body map[string]any) (map[string]any, error) {
	target, err := AuthEndpoint(c.BaseURL(), path)
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
func authSeamlessHeaders(target string, redirect, seamless bool) RequestOptions {
	h := HeaderOverrides{"Accept": ptrString("application/json, text/plain, */*"), "Content-Type": nil, "Origin": ptrString(AppOrigin), "Referer": ptrString(AppOrigin + "/"), "Sec-Fetch-Dest": ptrString("empty"), "Sec-Fetch-Mode": ptrString("cors"), "Sec-Fetch-Site": ptrString(authFetchSite(AppOrigin, target)), "X-Seamless-Web": nil, "X-Requested-With": nil}
	if redirect {
		h["Accept"] = ptrString("*/*")
		h["Content-Type"] = ptrString("application/json; charset=utf-8")
		if seamless {
			h["X-Seamless-Web"] = ptrString("true")
		}
	}
	return RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: h}
}
func (f *authFlow) getDocument(ctx context.Context, target, referer, code string) (*Response, error) {
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
func (f *authFlow) finishRedirect(ctx context.Context, c FrontendConfig, redirect string) (SessionBundle, error) {
	current, err := SafeOnlineURL(redirect, AppOrigin+"/")
	if err != nil {
		return SessionBundle{}, authRequestError(err)
	}
	post := c.RedirectPost()
	var r *Response
	finished := false
	for i := 0; i < MaxAuthRedirects; i++ {
		var e error
		if post {
			r, e = f.transport.Post(ctx, current, nil, authSeamlessHeaders(current, true, c.SeamlessWeb()))
		} else {
			r, e = f.transport.Get(ctx, current, authDocumentHeaders(PublicBootstrapURL, current))
		}
		if err = f.check(ctx); err != nil {
			return SessionBundle{}, authRequestError(err)
		}
		if e != nil {
			return SessionBundle{}, authRequestError(e)
		}
		if r == nil {
			return SessionBundle{}, authFailure("redirect_failed", nil)
		}
		if IsAuthRedirectStatus(r.StatusCode) {
			current, post, err = ResolveAuthRedirect(current, r.StatusCode, authResponseHeader(r, "Location"), post)
			if err != nil {
				return SessionBundle{}, authRequestError(err)
			}
			continue
		}
		if r.StatusCode != 200 {
			return SessionBundle{}, authFailure("redirect_failed", r)
		}
		finished = true
		break
	}
	if !finished {
		return SessionBundle{}, authFailure("redirect_loop", nil)
	}
	effective := current
	if observed := authResponseHeader(r, "X-Response-URL"); observed != "" {
		effective, err = SafeOnlineURL(observed, current)
		if err != nil {
			return SessionBundle{}, authRequestError(err)
		}
	}
	api, found := APIBaseFromMainHTML(r.Text())
	web := ""
	if found {
		web = authOrigin(effective)
	} else {
		if current != AppOrigin+"/app/main" {
			current = AppOrigin + "/app/main"
			r, err = f.getDocument(ctx, current, PublicBootstrapURL, "redirect_failed")
			if err != nil {
				return SessionBundle{}, authRequestError(err)
			}
		}
		var ok bool
		web, ok = UFSHostFromAppShell(r.Text())
		if !ok {
			return SessionBundle{}, authFailure("missing_ufs_host", r)
		}
		r, err = f.getDocument(ctx, web+"/main", current, "ufs_bootstrap_failed")
		if err != nil {
			return SessionBundle{}, authRequestError(err)
		}
		api, ok = APIBaseFromMainHTML(r.Text())
		if !ok {
			return SessionBundle{}, authFailure("missing_ufs_api_host", r)
		}
	}
	if c.SeamlessWeb() {
		ready := web + "/api/front/ready"
		r, e := f.transport.Get(ctx, ready, authSeamlessHeaders(ready, false, true))
		if err = f.check(ctx); err != nil {
			return SessionBundle{}, authRequestError(err)
		}
		if e != nil {
			return SessionBundle{}, authRequestError(e)
		}
		if r == nil || r.StatusCode != 200 {
			return SessionBundle{}, authFailure("ufs_ready_failed", r)
		}
	}
	snapshot := f.bundle
	snapshot.APIBase = api
	snapshot.WebBase = web
	snapshot, err = snapshot.WithCookieJar(f.transport.CookieJar())
	if err != nil {
		return SessionBundle{}, authRequestError(err)
	}
	if _, err = CredentialsFromBundle(snapshot); err != nil {
		return SessionBundle{}, authFailure("missing_ufs_session", nil)
	}
	if _, err = snapshot.ToSeed(true); err != nil {
		return SessionBundle{}, authFailure("missing_ufs_session", nil)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return SessionBundle{}, ErrClosed
	}
	if ctx.Err() != nil {
		return SessionBundle{}, transportFailure(ctx.Err())
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
