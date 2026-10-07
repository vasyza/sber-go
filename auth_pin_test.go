package sber

import (
	"context"
	"crypto/sha512"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Server math is independently ported from test_pin_auth.py::_server_challenge.
// It checks native M1 and supplies a genuine native M2; no fake proof injection.
func authServerChallenge(public, secret string) (B, salt, m1, m2 string) {
	n, _ := new(big.Int).SetString(strings.Repeat("f", 512), 16)
	g := big.NewInt(2)
	a, _ := new(big.Int).SetString(public, 16)
	s := big.NewInt(0xcafe)
	b := big.NewInt(0x1234567)
	ib := func(v *big.Int) []byte {
		x := v.Bytes()
		if len(x) == 0 {
			return []byte{0}
		}
		return x
	}
	pad := func(v *big.Int) []byte { x := make([]byte, 256); v.FillBytes(x); return x }
	h := func(p ...[]byte) []byte {
		d := sha512.New()
		for _, v := range p {
			d.Write(v)
		}
		return d.Sum(nil)
	}
	hi := func(p ...[]byte) *big.Int { return new(big.Int).SetBytes(h(p...)) }
	k := hi(pad(n), pad(g))
	x := hi(ib(s), []byte(secret))
	v := new(big.Int).Exp(g, x, n)
	sv := new(big.Int).Mul(k, v)
	sv.Add(sv, new(big.Int).Exp(g, b, n))
	sv.Mod(sv, n)
	u := hi(pad(a), pad(sv))
	shared := new(big.Int).Mul(a, new(big.Int).Exp(v, u, n))
	shared.Mod(shared, n)
	shared.Exp(shared, b, n)
	key := h(pad(shared))
	hn, hg := h(ib(n)), h(ib(g))
	for i := range hn {
		hn[i] ^= hg[i]
	}
	M := hi(hn, ib(s), pad(a), pad(sv), key)
	R := hi(pad(a), ib(M), key)
	return sv.Text(16), s.Text(16), M.Text(16), R.Text(16)
}
func pinNativeSteps(t *testing.T, proofSuffix string) ([]authStep, *Response, *Response) {
	t.Helper()
	begin := authJSON(200, map[string]any{})
	logon := authJSON(200, map[string]any{})
	var expected string
	steps := []authStep{{method: "GET", target: PublicBootstrapURL, response: authPage(authHTML(false))}, {method: "POST", target: AppOrigin + "/CSAFront/api/v1/pin/begin", response: begin, inspect: func(body map[string]any, _ map[string]string, o RequestOptions) {
		if body["deviceprint"] != "version=fixture&screen=synthetic" || authHeader(o, "Process-Id") != "process-fixture" || authHeader(o, "Rq-Uid") == "" || o.Headers["X-Requested-With"] != nil {
			t.Error("PIN request context missing")
		}
		B, s, M, R := authServerChallenge(body["srp_A"].(string), "13579")
		expected = M
		begin.Content = authJSON(200, map[string]any{"srp_B": B, "srp_s": s}).Content
		begin.Headers.Set("X-Csrf-Token", "synthetic-csrf")
		logon.Content = authJSON(200, map[string]any{"srp_R": R + proofSuffix, "redirect": "/finish"}).Content
	}}, {method: "POST", target: AppOrigin + "/CSAFront/api/v1/pin/logon", response: logon, inspect: func(body map[string]any, _ map[string]string, o RequestOptions) {
		if body["srp_M"] != expected || authHeader(o, "X-CSRF-Token") != "synthetic-csrf" {
			t.Error("native proof/rotated CSRF mismatch")
		}
	}}}
	return steps, begin, logon
}
func TestAuthPINNativeProofTrustedDiscovery(t *testing.T) {
	steps, _, _ := pinNativeSteps(t, "")
	s := newAuthScript(t)
	s.steps = append(steps, authStep{method: "GET", target: AppOrigin + "/finish", response: &Response{StatusCode: 302, Headers: http.Header{"Location": []string{"/app/main"}}}, run: func(context.Context) { setAuthCookies(t, s.jar) }}, authStep{method: "GET", target: AppOrigin + "/app/main", response: authPage(`startup({"ufsHost":"https://web2.online.sberbank.ru"})`)}, authStep{method: "GET", target: "https://web2.online.sberbank.ru/main", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`)})
	a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := a.Login(context.Background(), "13579", CaptchaAnswer{})
	if err != nil {
		t.Fatal(err)
	}
	if b.APIBase != "https://web-node2.online.sberbank.ru" || b.WebBase != "https://web2.online.sberbank.ru" {
		t.Fatal("did not use trusted runtime discovery")
	}
	if _, err = CredentialsFromBundle(b); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Login(context.Background(), "not reused", CaptchaAnswer{}); err != nil || s.calls != 6 {
		t.Fatal("authenticated login replayed credentials")
	}
}
func TestAuthPINRejectsBadProofBeforeNavigation(t *testing.T) {
	steps, _, _ := pinNativeSteps(t, "1")
	s := newAuthScript(t, steps...)
	a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	_, err = a.Login(context.Background(), "13579", CaptchaAnswer{})
	authErrorCode(t, err, "invalid_server_proof")
	if s.calls != 3 {
		t.Fatal("navigated without proof")
	}
}
func TestAuthPINMissingRuntimeCannotClaimReady(t *testing.T) {
	steps, _, _ := pinNativeSteps(t, "")
	s := newAuthScript(t)
	s.steps = append(steps, authStep{method: "GET", target: AppOrigin + "/finish", response: authPage("anonymous"), run: func(context.Context) { setAuthCookies(t, s.jar) }}, authStep{method: "GET", target: AppOrigin + "/app/main", response: authPage("anonymous")})
	a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	_, err = a.Login(context.Background(), "13579", CaptchaAnswer{})
	authErrorCode(t, err, "missing_ufs_host")
}
func TestAuthPINReadableXSRFOnly(t *testing.T) {
	steps, _, _ := pinNativeSteps(t, "")
	old := steps[1].inspect
	steps[1].inspect = func(b map[string]any, f map[string]string, o RequestOptions) {
		old(b, f, o)
		if authHeader(o, "X-XSRF-TOKEN") != "xsrf fixture" {
			t.Error("readable XSRF decoding wrong")
		}
	}
	steps = steps[:2]
	steps[1].err = &TransportError{Code: "synthetic-stop"}
	s := newAuthScript(t, steps...)
	u, _ := url.Parse(PublicBootstrapURL)
	_ = s.jar.ApplySetCookie(u, []string{"XSRF-TOKEN=xsrf%20fixture; Path=/CSAFront; Secure"})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	defer a.Close()
	_, _ = a.Login(context.Background(), "13579", CaptchaAnswer{})
}
