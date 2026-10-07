package sber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAuthPrimaryCaptchaTokenAndFormChoices(t *testing.T) {
	token := "synthetic-captcha-token"
	d := "version=fixture&screen=synthetic"
	s := newAuthScript(t, authStep{method: "GET", target: PublicBootstrapURL, response: authPage(authHTML(true))}, authStep{method: "FORM", target: AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"token": token, "error": map[string]any{"captcha": true, "code": 400}})}, authStep{method: "FORM", target: AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(400, map[string]any{"error": map[string]any{"captcha": "true", "field": "audioCaptchaCode"}}), inspect: func(_ map[string]any, b map[string]string, o RequestOptions) {
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
	var c *PinCaptchaRequired
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
	s.steps = append(s.steps, authStep{method: "FORM", target: AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "NEED_CONFIRM", "smsRequestAttemptsExhausted": "true", "token": "synthetic-three"})}, authStep{method: "FORM", target: AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "ATTEMPTS_EXHAUSTED"})})
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
				s.steps[1].inspect = func(b map[string]any, f map[string]string, o RequestOptions) {
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
				s.steps[2].inspect = func(b map[string]any, f map[string]string, o RequestOptions) {
					old(b, f, o)
					s.steps[2].response.Content = authJSON(200, map[string]any{"srpInfo": map[string]any{"srp_R": "1"}}).Content
				}
			}
			if mode == "optional-proof" {
				s.steps = append(s.steps, authStep{method: "FORM", target: AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "NOT_REQUIRED", "srpInfo": map[string]any{"srp_R": "1"}, "pinInfo": map[string]any{"webPinSkip": true}})})
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
			c := NewFrontendConfig("", "", 0, "", "", false, true)
			redirect := "/finish"
			expected := ""
			switch mode {
			case "external":
				redirect = "https://example.invalid/finish"
				expected = "unsafe_redirect"
			case "loop":
				for i := 0; i < 10; i++ {
					s.steps = append(s.steps, authStep{method: "POST", target: AppOrigin + "/finish", response: &Response{StatusCode: 307, Headers: http.Header{"Location": []string{"/finish"}}}, inspect: func(b map[string]any, _ map[string]string, o RequestOptions) {
						if b != nil {
							t.Error("navigation replay included credentials")
						}
					}})
				}
				expected = "redirect_loop"
			case "post-to-get":
				s.steps = []authStep{{method: "POST", target: AppOrigin + "/finish", response: &Response{StatusCode: 302, Headers: http.Header{"Location": []string{"/main"}}}}, {method: "GET", target: AppOrigin + "/main", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), run: func(context.Context) { setAuthCookies(t, s.jar) }}}
			case "missing-session":
				s.steps = []authStep{{method: "POST", target: AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`)}}
				expected = "missing_ufs_session"
			case "conflicting-host":
				s.steps = []authStep{{method: "POST", target: AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru","ufs.block.root.url":"https://web3.online.sberbank.ru"}`)}, {method: "GET", target: AppOrigin + "/app/main", response: authPage("invalid shell")}}
				expected = "missing_ufs_host"
			case "unsafe-effective":
				r := authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`)
				r.Headers.Set("X-Response-URL", "https://example.invalid/main")
				s.steps = []authStep{{method: "POST", target: AppOrigin + "/finish", response: r}}
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
		s := newAuthScript(t, authStep{method: "GET", target: PublicBootstrapURL, response: authPage(authHTML(false))}, authStep{method: "POST", target: AppOrigin + "/CSAFront/api/v1/pin/begin", response: authPage(raw)})
		a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
		_, e := a.Login(context.Background(), "13579", CaptchaAnswer{})
		authErrorCode(t, e, "invalid_json")
		_ = a.Close()
	}
	for _, pin := range []string{"1234", "１２３４５", "1357x"} {
		s := newAuthScript(t, authStep{method: "GET", target: PublicBootstrapURL, response: authPage(authHTML(false))})
		a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
		_, e := a.Login(context.Background(), pin, CaptchaAnswer{})
		authErrorCode(t, e, "invalid_pin")
		_ = a.Close()
	}
	_ = json.Valid
}
func TestAuthBrowserInvalidStateDoesNotAdopt(t *testing.T) {
	for _, mode := range []string{"url", "ua", "config", "failure", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s := newAuthScript(t)
			created := 0
			provider := BrowserBootstrapFunc(func(ctx context.Context, b SessionBundle, target string) (BrowserBootstrapResult, error) {
				r := BrowserBootstrapResult{HTML: authHTML(false), URL: PublicBootstrapURL, Browser: BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "synthetic"}}}}
				switch mode {
				case "url":
					r.URL = AppOrigin + "/app/main"
				case "ua":
					r.Browser = BrowserProfile{}
				case "config":
					r.HTML = "invalid config"
				case "failure":
					return r, errors.New("private-support-id secret")
				case "timeout":
					<-ctx.Done()
					return r, ctx.Err()
				}
				return r, nil
			})
			a, e := NewPINAuth(authBundle(t), AuthOptions{Transport: s, BrowserFirst: true, BrowserBootstrap: provider, BrowserBootstrapTimeout: 20 * time.Millisecond, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { created++; return newAuthScript(t), nil }})
			if e != nil {
				t.Fatal(e)
			}
			_, e = a.LoadConfig(context.Background())
			if e == nil || created != 0 || a.config != nil || strings.Contains(fmt.Sprintf("%+v", e), "private-support-id") {
				t.Fatal("invalid browser state adopted/leaked")
			}
			_ = a.Close()
		})
	}
}
