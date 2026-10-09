package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	"github.com/vasyza/sber-sdk/internal/rsaoaep"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	"github.com/vasyza/sber-sdk/internal/srp"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

// PrimaryAuth performs primary SRP, owner OTP and optional PIN enrollment.
// A nil session with nil error means enrollment is permitted, not authentication.
type PrimaryAuth struct{ *authFlow }

// StoreLogin defaults to true when nil; callers may explicitly choose false.
type PrimaryLoginOptions struct {
	StoreLogin                   *bool
	PublicKeyCredentialAvailable bool
	Captcha                      CaptchaAnswer
}

func (PrimaryLoginOptions) String() string               { return "PrimaryLoginOptions(<redacted>)" }
func (o PrimaryLoginOptions) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, o.String()) }
func (PrimaryLoginOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

// NewPrimaryAuth requires an explicit device identity and starts with no cookies.
func NewPrimaryAuth(o AuthOptions) (*PrimaryAuth, error) {
	return NewPrimaryAuthFromBundle(sdkSession.SessionBundle{APIBase: sdkSession.AppOrigin, WebBase: sdkSession.AppOrigin, Browser: o.Browser, Deviceprint: o.Deviceprint, AntifraudDeviceprint: o.AntifraudDeviceprint}, o)
}
func NewPrimaryAuthFromBundle(b sdkSession.SessionBundle, o AuthOptions) (*PrimaryAuth, error) {
	f, e := newAuthFlow(b, o, true)
	if e != nil {
		return nil, e
	}
	return &PrimaryAuth{f}, nil
}
func (a *PrimaryAuth) Login(ctx context.Context, login, password string, o PrimaryLoginOptions) (*sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if a.authenticated {
		b, e := a.bundle.Clone()
		return &b, e
	}
	if a.primaryOTPPending {
		return nil, authFailure("otp_already_pending", nil)
	}
	if !authInput(login, 256) || !authInput(password, 1024) {
		return nil, authFailure("invalid_credential", nil)
	}
	if e = o.Captcha.validate(); e != nil {
		return nil, e
	}
	c, e := a.loadConfig(ctx)
	if e != nil {
		return nil, e
	}
	client, e := srp.New(c.NHex(), c.GHex())
	if e != nil {
		return nil, authFailure("invalid_srp_challenge", nil)
	}
	store := true
	if o.StoreLogin != nil {
		store = *o.StoreLogin
	}
	body := a.primaryContext()
	body["operation"] = "button.begin"
	body["login"] = login
	body["pageInputType"] = "INDEX"
	body["storeLogin"] = strconv.FormatBool(store)
	body["srp_A"] = client.PublicHex()
	body["publicKeyCredentialAvailable"] = strconv.FormatBool(o.PublicKeyCredentialAvailable)
	if a.primaryToken != "" {
		body["token"] = a.primaryToken
	}
	if o.Captcha.Code != nil {
		body["captchaCode"] = *o.Captcha.Code
	}
	if o.Captcha.AudioCode != nil {
		body["audioCaptchaCode"] = *o.Captcha.AudioCode
	}
	begin, e := a.postPrimary(ctx, c, body)
	if e != nil {
		return nil, e
	}
	info, ok := begin["srpInfo"].(map[string]any)
	if !ok {
		return nil, authFailure("invalid_primary_auth_state", nil)
	}
	token, e := authToken(begin, true)
	if e != nil {
		return nil, e
	}
	B, _ := info["srp_B"].(string)
	salt, _ := info["srp_s"].(string)
	M, e := client.Process(password, salt, B)
	password = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		return nil, authFailure("invalid_srp_challenge", nil)
	}
	body = a.primaryContext()
	body["org.apache.struts.taglib.html.TOKEN"] = token
	body["operation"] = "button.next"
	body["login"] = login
	body["pageInputType"] = "INDEX"
	body["storeLogin"] = strconv.FormatBool(store)
	body["srp_M"] = M
	body["token"] = token
	proof, e := a.postPrimary(ctx, c, body)
	login = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		return nil, e
	}
	if e = verifyAuthPrimary(client, proof, false); e != nil {
		return nil, e
	}
	a.primarySRP = client
	return a.consumePrimary(ctx, c, proof)
}
func (a *PrimaryAuth) ConfirmOTP(ctx context.Context, code string) (*sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if !a.primaryOTPPending || a.config == nil || a.primaryToken == "" || a.primarySRP == nil {
		return nil, authFailure("otp_not_pending", nil)
	}
	if !authInput(code, 32) {
		return nil, authFailure("invalid_otp_code", nil)
	}
	body := a.primaryContext()
	body["org.apache.struts.taglib.html.TOKEN"] = a.primaryToken
	body["operation"] = "button.next"
	body["confirmPassword"] = code
	body["pageInputType"] = "INDEX"
	body["token"] = a.primaryToken
	p, e := a.postPrimary(ctx, *a.config, body)
	if e != nil {
		return nil, e
	}
	if e = verifyAuthPrimary(a.primarySRP, p, true); e != nil {
		return nil, e
	}
	b, e := a.consumePrimary(ctx, *a.config, p)
	if e == nil {
		a.primaryOTPPending = false
	}
	return b, e
}
func (a *PrimaryAuth) RetryOTP(ctx context.Context) (*sdkErrs.PinOTPRequired, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if !a.primaryOTPPending || a.config == nil || a.primaryToken == "" || a.primarySRP == nil {
		return nil, authFailure("otp_not_pending", nil)
	}
	body := a.primaryContext()
	body["org.apache.struts.taglib.html.TOKEN"] = a.primaryToken
	body["operation"] = "button.getNewPass"
	body["pageInputType"] = "INDEX"
	body["token"] = a.primaryToken
	p, e := a.postPrimary(ctx, *a.config, body)
	if e != nil {
		return nil, e
	}
	if p["state"] == "ATTEMPTS_EXHAUSTED" || authFormTrue(p["smsRequestAttemptsExhausted"]) {
		return nil, authFailure("attempts_limit_reached", nil)
	}
	return &sdkErrs.PinOTPRequired{Lifetime: authNonnegative(p["timeout"], true)}, nil
}
func (a *PrimaryAuth) CreatePIN(ctx context.Context, pin string) (sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return sdkSession.SessionBundle{}, e
	}
	defer done()
	if a.authenticated {
		return a.bundle.Clone()
	}
	if a.config == nil || a.pinPublicKey == "" {
		return sdkSession.SessionBundle{}, authFailure("pin_create_not_ready", nil)
	}
	if e = validateAuthPIN(pin, a.config.PINLength()); e != nil {
		return sdkSession.SessionBundle{}, e
	}
	ciphertext, e := rsaoaep.Encrypt(a.pinPublicKey, pin)
	pin = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		return sdkSession.SessionBundle{}, authFailure("invalid_pin_public_key", nil)
	}
	_, e = a.postJSON(ctx, *a.config, "/api/v1/pin/create", a.withContext(map[string]any{"pin": ciphertext}))
	if e != nil {
		return sdkSession.SessionBundle{}, e
	}
	a.pinPublicKey = ""
	return a.finishPrimary(ctx, *a.config)
}
func (a *PrimaryAuth) primaryContext() map[string]string {
	return map[string]string{"deviceprint": *a.bundle.Deviceprint, "jsEvents": "", "domElements": ""}
}
func authToken(p map[string]any, required bool) (string, error) {
	v, exists := p["token"]
	if !exists || v == nil {
		if required {
			return "", authFailure("missing_auth_token", nil)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok || !authInput(s, 8192) {
		return "", authFailure("invalid_auth_token", nil)
	}
	return s, nil
}
func verifyAuthPrimary(client *srp.Client, p map[string]any, optional bool) error {
	info, ok := p["srpInfo"].(map[string]any)
	if optional {
		if !ok {
			return nil
		}
		if _, exists := info["srp_R"]; !exists {
			return nil
		}
	}
	if !ok {
		return authFailure("invalid_primary_auth_state", nil)
	}
	R, ok := info["srp_R"].(string)
	valid := false
	if ok {
		valid, _ = client.Verify(R)
	}
	if !valid {
		return authFailure("invalid_server_proof", nil)
	}
	return nil
}
func authFormTrue(v any) bool { return v == true || v == "true" }
func (a *PrimaryAuth) postPrimary(ctx context.Context, c sdkSession.FrontendConfig, body map[string]string) (map[string]any, error) {
	target, e := sdkSession.AuthEndpoint(c.BaseURL(), "/authMainJson.do")
	if e != nil {
		return nil, e
	}
	h := sdkTransport.HeaderOverrides{"Accept": sdkTransport.PtrString("application/json, text/plain, */*"), "Content-Type": sdkTransport.PtrString("application/x-www-form-urlencoded"), "Origin": sdkTransport.PtrString(sdkSession.AppOrigin), "Referer": sdkTransport.PtrString(sdkTransport.PublicBootstrapURL), "Page-Id": sdkTransport.PtrString(""), "Process-Id": sdkTransport.PtrString(c.ProcessID()), "Sec-Fetch-Dest": sdkTransport.PtrString("empty"), "Sec-Fetch-Mode": sdkTransport.PtrString("cors"), "Sec-Fetch-Site": sdkTransport.PtrString(authFetchSite(sdkSession.AppOrigin, target)), "X-TS-AJAX-Request": sdkTransport.PtrString("true"), "X-Requested-With": nil}
	r, e := a.transport.PostForm(ctx, target, body, sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: h})
	if err := a.check(ctx); err != nil {
		return nil, err
	}
	if e != nil {
		return nil, authRequestError(e)
	}
	if e = a.updateCSRF(r); e != nil {
		return nil, e
	}
	p, e := authJSONObject(r)
	if e != nil {
		return nil, e
	}
	token, e := authToken(p, false)
	if e != nil {
		return nil, e
	}
	if token != "" {
		a.primaryToken = token
	}
	remote, _ := p["error"].(map[string]any)
	if remote != nil && authFormTrue(remote["captcha"]) {
		code := "need_captcha"
		if remote["field"] == "captchaCode" || remote["field"] == "audioCaptchaCode" {
			code = "invalid_captcha"
		}
		assets := map[string]any{"imageUrl": "captcha.png", "audioUrl": "audiocaptcha"}
		return nil, &sdkErrs.PinCaptchaRequired{PinAuthError: *authFailure(code, r), ImageURL: authCaptchaLink(assets, "imageUrl", c.BaseURL()), AudioURL: authCaptchaLink(assets, "audioUrl", c.BaseURL())}
	}
	state, _ := p["state"].(string)
	known := state == "NEED_CONFIRM" || state == "NOT_REQUIRED" || state == "WRONG_PASS" || state == "ATTEMPTS_EXHAUSTED"
	code := "primary_auth_failed"
	if v := remote["code"]; v != nil {
		code = fmt.Sprint(v)
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 || !known && remote != nil && remote["code"] != nil && code != "200" {
		return nil, authFailure(code, r)
	}
	return p, nil
}
func (a *PrimaryAuth) consumePrimary(ctx context.Context, c sdkSession.FrontendConfig, p map[string]any) (*sdkSession.SessionBundle, error) {
	switch p["state"] {
	case "NEED_CONFIRM":
		if a.primaryToken == "" {
			return nil, authFailure("invalid_otp_challenge", nil)
		}
		a.primaryOTPPending = true
		return nil, &sdkErrs.PinOTPRequired{Lifetime: authNonnegative(p["timeout"], true)}
	case "WRONG_PASS":
		return nil, authFailure("wrong_code", nil)
	case "ATTEMPTS_EXHAUSTED":
		a.primaryOTPPending = false
		return nil, authFailure("attempts_limit_reached", nil)
	}
	if info, ok := p["pinInfo"].(map[string]any); ok {
		if authFormTrue(info["webPinSkip"]) {
			b, e := a.finishPrimary(ctx, c)
			if e != nil {
				return nil, e
			}
			return &b, nil
		}
		if key, ok := info["publicKey"].(string); ok && key != "" {
			a.pinPublicKey = key
			return nil, nil
		}
	}
	if redirect, ok := p["redirect"].(string); ok && redirect != "" {
		b, e := a.finishRedirect(ctx, c, redirect)
		if e != nil {
			return nil, e
		}
		return &b, nil
	}
	if p["webAuthnInfo"] != nil {
		return nil, authFailure("webauthn_required", nil)
	}
	return nil, authFailure("invalid_primary_auth_state", nil)
}
func (a *PrimaryAuth) finishPrimary(ctx context.Context, c sdkSession.FrontendConfig) (sdkSession.SessionBundle, error) {
	p, e := a.postJSON(ctx, c, "/api/v1/auth", a.withContext(map[string]any{}))
	if e != nil {
		return sdkSession.SessionBundle{}, e
	}
	redirect, ok := p["redirect"].(string)
	if !ok || redirect == "" {
		return sdkSession.SessionBundle{}, authFailure("missing_redirect", nil)
	}
	return a.finishRedirect(ctx, c, redirect)
}
