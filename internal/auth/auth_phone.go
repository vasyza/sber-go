package auth

import (
	"context"
	"errors"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	"github.com/vasyza/sber-go/internal/rsaoaep"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	"github.com/vasyza/sber-go/internal/srp"
)

// PhoneAuth authenticates a phone number with the online banking password.
// Both SRP proofs are checked before the client can continue to SMS or a session.
type phoneState struct {
	token string
	store bool
}
type PhoneAuth struct {
	*authFlow
	phone **phoneState
}

func (a *PhoneAuth) state() *phoneState { return *a.phone }

func NewPhoneAuth(o AuthOptions) (*PhoneAuth, error) {
	f, e := newAuthFlow(sdkSession.SessionBundle{APIBase: sdkSession.AppOrigin, WebBase: sdkSession.AppOrigin, Browser: o.Browser, Deviceprint: o.Deviceprint, AntifraudDeviceprint: o.AntifraudDeviceprint}, o, true)
	if e != nil {
		return nil, e
	}
	state := &phoneState{}
	return &PhoneAuth{authFlow: f, phone: &state}, nil
}
func (a *PhoneAuth) Login(ctx context.Context, phone, password string, o PrimaryLoginOptions) (*sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if a.authenticated {
		b, e := a.bundle.Clone()
		return &b, e
	}
	if a.otpPending {
		return nil, authFailure("otp_already_pending", nil)
	}
	phone, ok := authenticationDigits(phone)
	if !ok || len(phone) != 11 || phone[0] != '7' {
		return nil, authFailure("invalid_phone", nil)
	}
	if !authInput(password, 1024) {
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
	a.state().store = true
	if o.StoreLogin != nil {
		a.state().store = *o.StoreLogin
	}
	channel := a.uapiChannel(&a.state().store)
	data, ok := channel["data"].(map[string]any)
	if !ok {
		return nil, authFailure("invalid_auth_channel", nil)
	}
	if o.Captcha.Code != nil {
		data["captcha_code"] = encodeAuthCaptcha(*o.Captcha.Code)
	}
	if o.Captcha.AudioCode != nil {
		data["audio_captcha_code"] = encodeAuthCaptcha(*o.Captcha.AudioCode)
	}
	p, e := a.postUAPI(ctx, "/uapi/v2/authenticate", map[string]any{"identifier": uapiIdentifier("phone_login", phone), "authenticator": map[string]any{"type": "web_sbol_srp_pass", "data": map[string]any{"srp_a": client.PublicHex()}}, "flow": "authcode", "channel": channel})
	phone = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		if phonePasswordTransition(e) {
			return a.passwordTransition(ctx, p, "", password, channel)
		}
		return nil, e
	}
	token, ok := p["ouid"].(string)
	if !ok || !authInput(token, 8192) {
		return nil, authFailure("invalid_auth_token", nil)
	}
	authenticators, ok := p["authenticator"].([]any)
	if !ok || len(authenticators) != 1 {
		return nil, authFailure("invalid_srp_challenge", nil)
	}
	challenge, ok := authenticators[0].(map[string]any)
	if !ok || challenge["type"] != "web_sbol_srp_pass" {
		return nil, authFailure("invalid_srp_challenge", nil)
	}
	cd, _ := challenge["data"].(map[string]any)
	B, _ := cd["srp_b"].(string)
	salt, _ := cd["srp_s"].(string)
	M, e := client.Process(password, salt, B)
	if e != nil {
		return nil, authFailure("invalid_srp_challenge", nil)
	}
	p, e = a.postUAPI(ctx, "/uapi/v2/verify", map[string]any{"identifier": uapiIdentifier("ouid", token), "authenticator": map[string]any{"type": "web_sbol_srp_pass", "data": map[string]any{"srp_m": M}}, "channel": a.uapiChannel(nil)})
	if e != nil {
		if phonePasswordTransition(e) {
			return a.passwordTransition(ctx, p, token, password, channel)
		}
		return nil, e
	}
	password = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	R, _ := p["srp_r"].(string)
	valid, _ := client.Verify(R)
	if !valid {
		return nil, authFailure("invalid_server_proof", nil)
	}
	return a.acceptPassword(ctx, p, token)
}

func phonePasswordTransition(err error) bool {
	var bank *sdkErrs.PinAuthError
	return errors.As(err, &bank) && bank.Code == "password_bypass"
}

// This is an explicit bank protocol transition, never a retry after an
// uncertain response or a replacement for checking a supplied SRP proof.
func (a *PhoneAuth) passwordTransition(ctx context.Context, p map[string]any, token, password string, channel map[string]any) (*sdkSession.SessionBundle, error) {
	if raw, present := p["ouid"]; present {
		var ok bool
		token, ok = raw.(string)
		if !ok || !authInput(token, 8192) {
			return nil, authFailure("invalid_auth_token", nil)
		}
	}
	if !authInput(token, 8192) {
		return nil, authFailure("invalid_auth_token", nil)
	}
	typeName, value := "pass", password
	if raw, present := p["passwordEncryptionKey"]; present {
		key, ok := raw.(string)
		if !ok || !authInput(key, 16384) {
			return nil, authFailure("invalid_password_key", nil)
		}
		encrypted, err := rsaoaep.Encrypt(key, password)
		if err != nil {
			return nil, authFailure("invalid_password_key", nil)
		}
		typeName, value = "encodedPassword", encrypted
	}
	password = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	p, err := a.postUAPI(ctx, "/uapi/v2/authenticate", map[string]any{"identifier": uapiIdentifier("ouid", token), "authenticator": map[string]any{"type": typeName, "data": map[string]any{"value": value}}, "flow": "authcode", "channel": channel})
	value = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if err != nil {
		return nil, err
	}
	return a.acceptPassword(ctx, p, token)
}

func (a *PhoneAuth) acceptPassword(ctx context.Context, p map[string]any, token string) (*sdkSession.SessionBundle, error) {
	if p["response_data"] != nil {
		return a.finishUAPI(ctx, p)
	}
	list, ok := p["authenticator"].([]any)
	if !ok || len(list) != 1 {
		return nil, authFailure("invalid_otp_challenge", nil)
	}
	ch, ok := list[0].(map[string]any)
	if !ok || ch["type"] != "sms_otp" {
		return nil, authFailure("invalid_otp_challenge", nil)
	}
	if raw, present := p["ouid"]; present {
		token, ok = raw.(string)
		if !ok || !authInput(token, 8192) {
			return nil, authFailure("invalid_auth_token", nil)
		}
	}
	a.state().token = token
	a.otpPending = true
	return nil, &sdkErrs.PinOTPRequired{Lifetime: authNonnegative(ch["lifetime"], true), RemainingAttempts: authNonnegative(ch["attempts_remaining"], false)}
}
func (a *PhoneAuth) ConfirmOTP(ctx context.Context, code string) (*sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if !a.otpPending || a.state().token == "" || a.config == nil {
		return nil, authFailure("otp_not_pending", nil)
	}
	if !authInput(code, 32) {
		return nil, authFailure("invalid_otp_code", nil)
	}
	p, e := a.postUAPI(ctx, "/uapi/v2/verify", map[string]any{"identifier": uapiIdentifier("ouid", a.state().token), "authenticator": map[string]any{"type": "sms_otp", "data": map[string]any{"value": code}}, "channel": a.uapiChannel(&a.state().store)})
	code = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		return nil, e
	}
	b, e := a.finishUAPI(ctx, p)
	if e == nil {
		a.otpPending = false
		a.state().token = ""
	}
	return b, e
}
