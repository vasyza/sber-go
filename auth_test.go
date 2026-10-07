package sber

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// Synthetic scripts port pin.py and tests/test_{pin,primary}_auth.py; they
// cannot make a network request. Flow-only fake proofs are NOT crypto KATs.
type authStep struct {
	method, target string
	response       *Response
	err            error
	inspect        func(map[string]any, map[string]string, RequestOptions)
	run            func(context.Context)
}
type authScript struct {
	t      *testing.T
	mu     sync.Mutex
	jar    *CookieJar
	steps  []authStep
	calls  int
	closes int
}

func newAuthScript(t *testing.T, steps ...authStep) *authScript {
	t.Helper()
	j, err := NewCookieJar(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &authScript{t: t, jar: j, steps: steps}
}
func (s *authScript) request(ctx context.Context, method, target string, body map[string]any, form map[string]string, o RequestOptions) (*Response, error) {
	s.mu.Lock()
	if s.calls >= len(s.steps) {
		s.mu.Unlock()
		s.t.Error("unexpected request; credentials must not replay")
		return nil, errors.New("unexpected synthetic request")
	}
	step := s.steps[s.calls]
	s.calls++
	s.mu.Unlock()
	if method != step.method || target != step.target {
		s.t.Errorf("wrong synthetic request method/path (step %d)", s.calls)
	}
	if step.inspect != nil {
		step.inspect(body, form, o)
	}
	if step.run != nil {
		step.run(ctx)
	}
	return step.response, step.err
}
func (s *authScript) Get(ctx context.Context, u string, o RequestOptions) (*Response, error) {
	return s.request(ctx, "GET", u, nil, nil, o)
}
func (s *authScript) Post(ctx context.Context, u string, b map[string]any, o RequestOptions) (*Response, error) {
	return s.request(ctx, "POST", u, b, nil, o)
}
func (s *authScript) PostForm(ctx context.Context, u string, b map[string]string, o RequestOptions) (*Response, error) {
	return s.request(ctx, "FORM", u, nil, b, o)
}
func (s *authScript) CookieJar() *CookieJar { return s.jar }
func (s *authScript) Close() error          { s.mu.Lock(); defer s.mu.Unlock(); s.closes++; return nil }
func authHTML(primary bool) string {
	n := strings.Repeat("f", 512)
	if primary {
		return `<script>window.config = {baseApiUrl:"CSAFront",processId:"process-fixture",authTypeByCookie:"start",srpConfig:{enabled:true,N:"` + n + `",g:"2"},pinConfig:{enabled:true,hasPin:false},validation:{pin:{length:5}},isSeamlessWeb:true,isUfsRedirectMethodPostEnabled:true};</script>`
	}
	return `<script>window.config = {baseApiUrl:"CSAFront",processId:"process-fixture",authTypeByCookie:"pin",pinConfig:{enabled:true,hasPin:true,length:5,srp_N:"` + n + `",srp_g:"2"}};</script>`
}
func authPage(text string) *Response {
	return &Response{StatusCode: 200, Headers: http.Header{}, Content: []byte(text)}
}
func authJSON(status int, p map[string]any) *Response {
	b, _ := json.Marshal(p)
	return &Response{StatusCode: status, Headers: http.Header{}, Content: b}
}
func authBundle(t *testing.T) SessionBundle {
	t.Helper()
	d := "version=fixture&screen=synthetic"
	b, err := NewSessionBundle(SessionBundle{APIBase: AppOrigin, WebBase: AppOrigin, Deviceprint: &d})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func authErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *PinAuthError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("expected auth code %s; got %v", code, err)
	}
}
func authHeader(o RequestOptions, name string) string {
	p := o.Headers[name]
	if p == nil {
		return ""
	}
	return *p
}
func setAuthCookies(t *testing.T, j *CookieJar) {
	t.Helper()
	u, _ := url.Parse(AppOrigin + "/")
	if err := j.ApplySetCookie(u, []string{"UFS-SESSION=synthetic-session; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly", "UFS-TOKEN=synthetic-token; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthBootstrapStrictDefault(t *testing.T) {
	for _, tc := range []struct{ html, code string }{{authHTML(false), ""}, {"<html>malformed anonymous page</html>", "invalid_frontend_config"}, {`<script src="/TSPD/a.js"></script>Enable JavaScript`, "browser_check_required"}} {
		t.Run(tc.code, func(t *testing.T) {
			s := newAuthScript(t, authStep{method: "GET", target: PublicBootstrapURL, response: authPage(tc.html), inspect: func(_ map[string]any, _ map[string]string, o RequestOptions) {
				if authHeader(o, "Sec-Fetch-Site") != "none" || o.Headers["Content-Type"] != nil || o.Headers["Origin"] != nil {
					t.Error("not document navigation")
				}
			}})
			a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
			if err != nil {
				t.Fatal(err)
			}
			c, err := a.LoadConfig(context.Background())
			if tc.code != "" {
				authErrorCode(t, err, tc.code)
			} else if err != nil || c.ProcessID() != "process-fixture" || c.PINLength() != 5 {
				t.Fatal("strict config missing")
			}
			if _, err := a.ExportSession(); err != nil {
				t.Fatal(err)
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := a.LoadConfig(context.Background()); !errors.Is(err, ErrClosed) {
				t.Fatal("closed auth resurrected")
			}
			if err := a.Close(); err != nil || s.closes != 1 {
				t.Fatal("close not idempotent")
			}
		})
	}
}
