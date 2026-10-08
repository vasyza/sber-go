package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestAuthPINOTPContinuation(t *testing.T) {
	steps, _, logon := pinNativeSteps(t, "")
	old := steps[2].inspect
	steps[2].inspect = func(b map[string]any, f map[string]string, o sdkTransport.RequestOptions) {
		old(b, f, o)
		var p map[string]any
		_ = json.Unmarshal(logon.Content, &p)
		delete(p, "redirect")
		p["confirmInfo"] = map[string]any{"lifetime": 120, "remainingAttempts": 3}
		logon.Content = authJSON(200, p).Content
	}
	s := newAuthScript(t)
	s.steps = append(steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/otp/retry", response: authJSON(200, map[string]any{"confirmInfo": map[string]any{"lifetime": 90, "remainingAttempts": 2}}), inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if len(b) != 1 || b["isPwa"] != true {
			t.Error("retry included deviceprint or omitted isPwa")
		}
	}}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/otp/confirm", response: authJSON(400, map[string]any{"error": map[string]any{"code": "wrong_code"}, "confirmInfo": map[string]any{"remainingAttempts": 1}})}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/otp/confirm", response: authJSON(200, map[string]any{"redirect": "/finish"}), inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if b["confirmPassword"] != "000000" || b["isPwa"] != true {
			t.Error("confirm context wrong")
		}
	}}, authStep{method: "GET", target: sdkSession.AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), run: func(context.Context) { setAuthCookies(t, s.jar) }})
	a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s, IsPWA: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	_, err = a.Login(context.Background(), "13579", CaptchaAnswer{})
	var otp *sdkErrs.PinOTPRequired
	if !errors.As(err, &otp) || otp.Lifetime == nil || *otp.Lifetime != 120 {
		t.Fatal("missing OTP challenge")
	}
	_, err = a.Login(context.Background(), "13579", CaptchaAnswer{})
	authErrorCode(t, err, "otp_already_pending")
	challenge, err := a.RetryOTP(context.Background())
	if err != nil || challenge.Lifetime == nil || *challenge.Lifetime != 90 {
		t.Fatal("retry metadata wrong")
	}
	_, err = a.ConfirmOTP(context.Background(), "111111")
	authErrorCode(t, err, "wrong_code")
	var pe *sdkErrs.PinAuthError
	_ = errors.As(err, &pe)
	if pe.RemainingAttempts == nil || *pe.RemainingAttempts != 1 {
		t.Fatal("attempt metadata lost")
	}
	if _, err = a.ConfirmOTP(context.Background(), "000000"); err != nil {
		t.Fatal(err)
	}
	_, err = a.ConfirmOTP(context.Background(), "000000")
	authErrorCode(t, err, "otp_not_pending")
}
func TestAuthPINCaptchaRetryKeepsEphemeral(t *testing.T) {
	steps, _, _ := pinNativeSteps(t, "")
	firstA := ""
	captcha := authJSON(400, map[string]any{"error": map[string]any{"code": "need_captcha"}, "captcha": map[string]any{"imageUrl": "/captcha.png"}})
	captcha.Headers.Set("X-Csrf-Token", "captcha-csrf")
	retryInspect := steps[1].inspect
	steps[1].inspect = func(b map[string]any, f map[string]string, o sdkTransport.RequestOptions) {
		if b["srp_A"] != firstA || b["captchaCode"] != "ABCD" || authHeader(o, "X-CSRF-Token") != "captcha-csrf" {
			t.Error("captcha continuation lost process/ephemeral")
		}
		retryInspect(b, f, o)
	}
	s := newAuthScript(t, steps[0], authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/begin", response: captcha, inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		firstA = b["srp_A"].(string)
	}}, authStep{method: "GET", response: authPage("synthetic-image")}, steps[1], steps[2])
	s.steps = append(s.steps, authStep{method: "GET", target: sdkSession.AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), run: func(context.Context) { setAuthCookies(t, s.jar) }})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	defer a.Close()
	_, err := a.Login(context.Background(), "13579", CaptchaAnswer{})
	var c *sdkErrs.PinCaptchaRequired
	if !errors.As(err, &c) || c.ImageURL == nil || !strings.HasPrefix(*c.ImageURL, sdkSession.AppOrigin+"/CSAFront/captcha.png?noc=") {
		t.Fatal("missing safe captcha URL")
	}
	s.steps[2].target = *c.ImageURL
	image, err := a.FetchCaptcha(context.Background(), *c.ImageURL)
	if err != nil || string(image) != "synthetic-image" {
		t.Fatal("captcha fetch failed")
	}
	answer := "ABCD"
	if _, err = a.Login(context.Background(), "13579", CaptchaAnswer{Code: &answer}); err != nil {
		t.Fatal(err)
	}
}
func TestAuthPINSourceErrorMetadataAndProcessReset(t *testing.T) {
	for _, code := range []string{"wrong_pin", "invalid_request", "service_unavailable"} {
		t.Run(code, func(t *testing.T) {
			s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(false))}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/begin", response: authJSON(400, map[string]any{"error": map[string]any{"code": code}, "pinInfo": map[string]any{"remainingAttempts": 0, "resetCookies": true}})})
			a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
			defer a.Close()
			_, err := a.Login(context.Background(), "13579", CaptchaAnswer{})
			authErrorCode(t, err, code)
			var e *sdkErrs.PinAuthError
			_ = errors.As(err, &e)
			if !e.ResetCookies || e.RemainingAttempts == nil || *e.RemainingAttempts != 0 {
				t.Fatal("lost source error metadata")
			}
			if code != "wrong_pin" && a.config != nil {
				t.Fatal("invalid process not reset")
			}
		})
	}
}
func TestAuthCaptchaUnsafeAndBodyGates(t *testing.T) {
	s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(false))}, authStep{method: "GET", target: sdkSession.AppOrigin + "/CSAFront/captcha.png", response: authPage("")})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	defer a.Close()
	_, _ = a.LoadConfig(context.Background())
	for _, u := range []string{"https://example.invalid/CSAFront/a", "/outside/a", "../a"} {
		if _, err := a.FetchCaptcha(context.Background(), u); err == nil {
			t.Fatal("unsafe captcha request")
		}
	}
	_, err := a.FetchCaptcha(context.Background(), sdkSession.AppOrigin+"/CSAFront/captcha.png")
	authErrorCode(t, err, "invalid_captcha_body")
	_ = url.Values{}
}

func primaryNativeSteps(t *testing.T, proofState map[string]any) []authStep {
	begin := authJSON(200, map[string]any{})
	proof := authJSON(200, map[string]any{})
	var M string
	return []authStep{{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(true))}, {method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: begin, inspect: func(_ map[string]any, b map[string]string, o sdkTransport.RequestOptions) {
		if b["operation"] != "button.begin" || b["storeLogin"] != "true" || b["login"] != "fixture-login" || b["publicKeyCredentialAvailable"] != "false" || b["jsEvents"] != "" || b["domElements"] != "" || authHeader(o, "Content-Type") != "application/x-www-form-urlencoded" || authHeader(o, "Process-Id") != "process-fixture" {
			t.Error("primary form/context mismatch")
		}
		B, s, m, R := authServerChallenge(b["srp_A"], "fixture-password")
		M = m
		begin.Content = authJSON(200, map[string]any{"token": "synthetic-one", "srpInfo": map[string]any{"srp_B": B, "srp_s": s}}).Content
		proofState["srpInfo"] = map[string]any{"srp_R": R}
		proof.Content = authJSON(200, proofState).Content
	}}, {method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: proof, inspect: func(_ map[string]any, b map[string]string, o sdkTransport.RequestOptions) {
		if b["operation"] != "button.next" || b["token"] != "synthetic-one" || b["org.apache.struts.taglib.html.TOKEN"] != b["token"] || b["srp_M"] != M {
			t.Error("primary native proof mismatch")
		}
	}}}
}
func newPrimaryForScript(t *testing.T, s *authScript) *PrimaryAuth {
	t.Helper()
	d := "version=fixture&screen=synthetic"
	a, e := NewPrimaryAuth(AuthOptions{Transport: s, Deviceprint: &d})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}
func primaryFinishSteps(t *testing.T, s *authScript) []authStep {
	return []authStep{{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/auth", response: authJSON(200, map[string]any{"redirect": "https://web2.online.sberbank.ru/auth/web?ticket=synthetic"}), inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if len(b) != 1 || b["deviceprint"] == nil {
			t.Error("finish JSON context wrong")
		}
	}}, {method: "POST", target: "https://web2.online.sberbank.ru/auth/web?ticket=synthetic", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), run: func(context.Context) { setAuthCookies(t, s.jar) }, inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if b != nil || authHeader(o, "X-Seamless-Web") != "true" {
			t.Error("auth redirect must be empty-body navigation")
		}
	}}, {method: "GET", target: "https://web2.online.sberbank.ru/api/front/ready", response: authJSON(200, map[string]any{})}}
}
func TestAuthPrimaryNativeOTPEnrollRSA(t *testing.T) {
	private, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	pub := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&private.PublicKey))
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"state": "NEED_CONFIRM", "timeout": "120", "token": "synthetic-two"})
	s.steps = append(s.steps, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "WRONG_PASS", "token": "synthetic-two", "error": map[string]any{"code": 507}})}, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "NEED_CONFIRM", "timeout": 90, "token": "synthetic-three"}), inspect: func(_ map[string]any, b map[string]string, o sdkTransport.RequestOptions) {
		if b["operation"] != "button.getNewPass" || b["token"] != "synthetic-two" {
			t.Error("resend context lost")
		}
	}}, authStep{method: "FORM", target: sdkSession.AppOrigin + "/CSAFront/authMainJson.do", response: authJSON(200, map[string]any{"state": "NOT_REQUIRED", "pinInfo": map[string]any{"publicKey": pub}, "smsRequestAttemptsExhausted": "true"}), inspect: func(_ map[string]any, b map[string]string, o sdkTransport.RequestOptions) {
		if b["confirmPassword"] != "000000" || b["token"] != "synthetic-three" {
			t.Error("OTP token rotation lost")
		}
	}}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/create", response: authJSON(200, map[string]any{}), inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		cipher, e := base64.StdEncoding.DecodeString(b["pin"].(string))
		if e != nil {
			t.Fatal("invalid RSA wire")
		}
		plain, e := rsa.DecryptOAEP(sha1.New(), rand.Reader, private, cipher, nil)
		if e != nil || string(plain) != "13579" {
			t.Fatal("native OAEP not frontend-compatible")
		}
	}})
	s.steps = append(s.steps, primaryFinishSteps(t, s)...)
	a := newPrimaryForScript(t, s)
	b, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	var otp *sdkErrs.PinOTPRequired
	if b != nil || !errors.As(e, &otp) || otp.Lifetime == nil || *otp.Lifetime != 120 {
		t.Fatal("primary OTP missing")
	}
	_, e = a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	authErrorCode(t, e, "otp_already_pending")
	_, e = a.ConfirmOTP(context.Background(), "111111")
	authErrorCode(t, e, "wrong_code")
	r, e := a.RetryOTP(context.Background())
	if e != nil || r.Lifetime == nil || *r.Lifetime != 90 {
		t.Fatal("primary retry missing")
	}
	b, e = a.ConfirmOTP(context.Background(), "000000")
	if e != nil || b != nil {
		t.Fatal("pinInfo should request enrollment, not early-ready")
	}
	_, e = a.ConfirmOTP(context.Background(), "000000")
	authErrorCode(t, e, "otp_not_pending")
	session, e := a.CreatePIN(context.Background(), "13579")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sdkSession.CredentialsFromBundle(session); e != nil {
		t.Fatal(e)
	}
	before := s.calls
	if _, e = a.CreatePIN(context.Background(), "unretained"); e != nil || s.calls != before {
		t.Fatal("enrollment replayed")
	}
}
func TestAuthPrimarySkipEnrollment(t *testing.T) {
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"state": "NOT_REQUIRED", "pinInfo": map[string]any{"webPinSkip": "true"}})
	s.steps = append(s.steps, primaryFinishSteps(t, s)...)
	a := newPrimaryForScript(t, s)
	b, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	if e != nil || b == nil {
		t.Fatal("skip did not finish")
	}
}

func TestAuthSeamlessRedirectCarriesProcessIdentity(t *testing.T) {
	const processID = "synthetic-redirect-process"
	script := newAuthScript(t, authStep{
		method: "POST", target: sdkSession.AppOrigin + "/finish",
		response: authJSON(500, map[string]any{}),
		inspect: func(body map[string]any, _ map[string]string, options sdkTransport.RequestOptions) {
			if body != nil {
				t.Error("seamless redirect must keep an empty request body")
			}
			if authHeader(options, "Process-Id") != processID {
				t.Error("seamless redirect lost the originating authentication process")
			}
			if authHeader(options, "X-Seamless-Web") != "true" {
				t.Error("seamless redirect lost its request mode")
			}
		},
	})
	auth, err := NewPrimaryAuth(AuthOptions{Deviceprint: stringPointer("synthetic-device"), Transport: script})
	if err != nil {
		t.Fatal(err)
	}
	defer auth.Close()
	config := sdkSession.NewFrontendConfig("", processID, 0, "", "", true, true)
	_, err = auth.finishRedirect(context.Background(), config, "/finish")
	authErrorCode(t, err, "redirect_failed")
	if script.calls != 1 {
		t.Fatal("failed redirect was automatically retried")
	}
}

func stringPointer(value string) *string { return &value }
