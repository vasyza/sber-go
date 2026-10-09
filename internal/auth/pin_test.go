package auth

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

func TestAuthPrimaryCaptchaTokenAndFormChoices(t *testing.T) {
	token := "synthetic-captcha-token"
	d := "version=fixture&screen=synthetic"
	s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(true))}, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"token": token, "error": map[string]any{"captcha": true, "code": 400}})}, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(400, map[string]any{"error": map[string]any{"captcha": "true", "field": "audioCaptchaCode"}}), inspect: func(_ map[string]any, b map[string]string, o sdkTransport.RequestOptions) {
		if b["token"] != token || b["audioCaptchaCode"] != "ABCD" || b["storeLogin"] != "false" || b["publicKeyCredentialAvailable"] != "true" {
			t.Error("captcha token/form choices lost")
		}
	}})
	a, e := NewPrimaryAuth(AuthOptions{Transport: s, Deviceprint: &d})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	_, e = a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	var c *sdkErrs.PinCaptchaRequired
	if !errors.As(e, &c) || c.AudioURL == nil || !strings.Contains(*c.AudioURL, "/CSAFront/audiocaptcha?noc=") {
		t.Fatal("200 captcha not recognized")
	}
	no := false
	answer := "ABCD"
	_, e = a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{StoreLogin: &no, PublicKeyCredentialAvailable: true, Captcha: CaptchaAnswer{AudioCode: &answer}})
	if !errors.As(e, &c) || c.Code != "invalid_captcha" {
		t.Fatal("audio captcha error source mismatch")
	}
}
func TestAuthPrimaryResendLimitDoesNotDiscardUsableCode(t *testing.T) {
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"state": "NEED_CONFIRM", "timeout": 120, "token": "synthetic-two"})
	s.steps = append(s.steps, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "NEED_CONFIRM", "smsRequestAttemptsExhausted": "true", "token": "synthetic-three"})}, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "ATTEMPTS_EXHAUSTED"})})
	a := newPrimaryForScript(t, s)
	_, _ = a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	_, e := a.RetryOTP(context.Background())
	authErrorCode(t, e, "attempts_limit_reached")
	if !a.primaryOTPPending || a.primaryToken != "synthetic-three" {
		t.Fatal("resend exhaustion discarded active code")
	}
	_, e = a.ConfirmOTP(context.Background(), "000000")
	authErrorCode(t, e, "attempts_limit_reached")
	if a.primaryOTPPending {
		t.Fatal("terminal confirmation did not clear pending")
	}
}
func TestAuthPrimaryMalformedChallengeAndProof(t *testing.T) {
	for _, mode := range []string{"token", "challenge", "proof", "optional-proof"} {
		t.Run(mode, func(t *testing.T) {
			s := newAuthScript(t)
			state := map[string]any{"state": "NEED_CONFIRM", "token": "synthetic-two"}
			s.steps = primaryNativeSteps(t, state)
			if mode == "token" || mode == "challenge" {
				old := s.steps[1].inspect
				s.steps[1].inspect = func(b map[string]any, f map[string]string, o sdkTransport.RequestOptions) {
					old(b, f, o)
					if mode == "token" {
						s.steps[1].response.Content = authJSON(200, map[string]any{"token": "bad\ntoken", "srpInfo": map[string]any{}}).Content
					} else {
						s.steps[1].response.Content = authJSON(200, map[string]any{"token": "synthetic-one", "srpInfo": map[string]any{"srp_B": "0", "srp_s": "cafe"}}).Content
					}
				}
				s.steps = s.steps[:2]
			}
			if mode == "proof" {
				old := s.steps[2].inspect
				s.steps[2].inspect = func(b map[string]any, f map[string]string, o sdkTransport.RequestOptions) {
					old(b, f, o)
					s.steps[2].response.Content = authJSON(200, map[string]any{"srpInfo": map[string]any{"srp_R": "1"}}).Content
				}
			}
			if mode == "optional-proof" {
				s.steps = append(s.steps, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "NOT_REQUIRED", "srpInfo": map[string]any{"srp_R": "1"}, "pinInfo": map[string]any{"webPinSkip": true}})})
			}
			a := newPrimaryForScript(t, s)
			_, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
			expected := map[string]string{"token": "invalid_auth_token", "challenge": "invalid_srp_challenge", "proof": "invalid_server_proof", "optional-proof": "invalid_server_proof"}[mode]
			if mode == "optional-proof" {
				_, e = a.ConfirmOTP(context.Background(), "000000")
			}
			authErrorCode(t, e, expected)
			if strings.Contains(fmt.Sprintf("%#v", e), "fixture-password") {
				t.Fatal("secret retained in error")
			}
		})
	}
}
func TestAuthTrustedFinishNavigationBounds(t *testing.T) {
	for _, mode := range []string{"external", "loop", "post-to-get", "missing-session", "conflicting-host", "unsafe-effective"} {
		t.Run(mode, func(t *testing.T) {
			s := newAuthScript(t)
			a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
			defer a.Close()
			c := sdkSession.NewFrontendConfig("", "", 0, "", "", false, true)
			redirect := "/finish"
			expected := ""
			switch mode {
			case "external":
				redirect = "https://example.invalid/finish"
				expected = "unsafe_redirect"
			case "loop":
				for i := 0; i < 10; i++ {
					s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/finish", response: &sdkTransport.Response{StatusCode: 307, Headers: http.Header{"Location": []string{"/finish"}}}, inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
						if b != nil {
							t.Error("navigation replay included credentials")
						}
					}})
				}
				expected = "redirect_loop"
			case "post-to-get":
				s.steps = []authStep{{method: "POST", target: sdkSession.AppOrigin + "/finish", response: &sdkTransport.Response{StatusCode: 302, Headers: http.Header{"Location": []string{"/main"}}}}, {method: "GET", target: sdkSession.AppOrigin + "/main", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), run: func(context.Context) { setAuthCookies(t, s.jar) }}}
			case "missing-session":
				s.steps = []authStep{{method: "POST", target: sdkSession.AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`)}}
				expected = "missing_ufs_session"
			case "conflicting-host":
				s.steps = []authStep{{method: "POST", target: sdkSession.AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru","ufs.block.root.url":"https://web3.online.sberbank.ru"}`)}, {method: "GET", target: sdkSession.AppOrigin + "/app/main", response: authPage("invalid shell")}}
				expected = "missing_ufs_host"
			case "unsafe-effective":
				r := authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`)
				r.Headers.Set("X-Response-Url", "https://example.invalid/main")
				s.steps = []authStep{{method: "POST", target: sdkSession.AppOrigin + "/finish", response: r}}
				expected = "unsafe_redirect"
			}
			ctx, done, e := a.enter(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			_, e = a.finishRedirect(ctx, c, redirect)
			done()
			if expected == "" {
				if e != nil {
					t.Fatal(e)
				}
			} else {
				authErrorCode(t, e, expected)
				if a.authenticated {
					t.Fatal("failed finish claimed ready")
				}
			}
		})
	}
}
func TestAuthJSONAndInputBoundaries(t *testing.T) {
	for _, raw := range []string{"null", "[]", "bad JSON"} {
		s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(false))}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/begin", response: authPage(raw)})
		a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
		_, e := a.Login(context.Background(), "13579", CaptchaAnswer{})
		authErrorCode(t, e, "invalid_json")
		_ = a.Close()
	}
	for _, pin := range []string{"1234", "１２３４５", "1357x"} {
		s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(false))})
		a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
		_, e := a.Login(context.Background(), pin, CaptchaAnswer{})
		authErrorCode(t, e, "invalid_pin")
		_ = a.Close()
	}
	_ = json.Valid
}

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
func pinNativeSteps(t *testing.T, proofSuffix string) ([]authStep, *sdkTransport.Response, *sdkTransport.Response) {
	t.Helper()
	begin := authJSON(200, map[string]any{})
	logon := authJSON(200, map[string]any{})
	var expected string
	steps := []authStep{{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(false))}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/begin", response: begin, inspect: func(body map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if body["deviceprint"] != "version=fixture&screen=synthetic" || authHeader(o, "Process-Id") != "process-fixture" || authHeader(o, "Rq-Uid") == "" || o.Headers["X-Requested-With"] != nil {
			t.Error("PIN request context missing")
		}
		B, s, M, R := authServerChallenge(body["srp_A"].(string), "13579")
		expected = M
		begin.Content = authJSON(200, map[string]any{"srp_B": B, "srp_s": s}).Content
		begin.Headers.Set("X-Csrf-Token", "synthetic-csrf")
		logon.Content = authJSON(200, map[string]any{"srp_R": R + proofSuffix, "redirect": "/finish"}).Content
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/logon", response: logon, inspect: func(body map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if body["srp_M"] != expected || authHeader(o, "X-CSRF-Token") != "synthetic-csrf" {
			t.Error("native proof/rotated CSRF mismatch")
		}
	}}}
	return steps, begin, logon
}
func TestAuthPINNativeProofTrustedDiscovery(t *testing.T) {
	steps, _, _ := pinNativeSteps(t, "")
	s := newAuthScript(t)
	s.steps = append(steps, authStep{method: "GET", target: sdkSession.AppOrigin + "/finish", response: &sdkTransport.Response{StatusCode: 302, Headers: http.Header{"Location": []string{"/app/main"}}}, run: func(context.Context) { setAuthCookies(t, s.jar) }}, authStep{method: "GET", target: sdkSession.AppOrigin + "/app/main", response: authPage(`startup({"ufsHost":"https://web2.online.sberbank.ru"})`)}, authStep{method: "GET", target: "https://web2.online.sberbank.ru/main", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`)})
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
	if _, err = sdkSession.CredentialsFromBundle(b); err != nil {
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
	s.steps = append(steps, authStep{method: "GET", target: sdkSession.AppOrigin + "/finish", response: authPage("anonymous"), run: func(context.Context) { setAuthCookies(t, s.jar) }}, authStep{method: "GET", target: sdkSession.AppOrigin + "/app/main", response: authPage("anonymous")})
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
	steps[1].inspect = func(b map[string]any, f map[string]string, o sdkTransport.RequestOptions) {
		old(b, f, o)
		if authHeader(o, "X-XSRF-TOKEN") != "xsrf fixture" {
			t.Error("readable XSRF decoding wrong")
		}
	}
	steps = steps[:2]
	steps[1].err = &sdkErrs.TransportError{Code: "synthetic-stop"}
	s := newAuthScript(t, steps...)
	u, _ := url.Parse(sdkTransport.PublicBootstrapURL)
	_ = s.jar.ApplySetCookie(u, []string{"XSRF-TOKEN=xsrf%20fixture; Path=/CSAFront; Secure"})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	defer a.Close()
	_, _ = a.Login(context.Background(), "13579", CaptchaAnswer{})
}
