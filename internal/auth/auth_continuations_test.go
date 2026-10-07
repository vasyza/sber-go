package auth

import (
	"context"
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
	captcha.Headers.Set("X-CSRF-Token", "captcha-csrf")
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
