package auth

import (
	"context"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	"github.com/vasyza/sber-go/internal/rsaoaep"
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

// CardAuth encrypts a PAN, confirms bank SMS, and optionally enrolls a web PIN.
// It never requests a card PIN, CVV, expiry date or payment permission.
type CardAuth struct {
	*PrimaryAuth
}

func NewCardAuth(o AuthOptions) (*CardAuth, error) {
	a, e := NewPrimaryAuth(o)
	if e != nil {
		return nil, e
	}
	return &CardAuth{PrimaryAuth: a}, nil
}
func (a *CardAuth) Login(ctx context.Context, number string, captcha CaptchaAnswer) (*sdkSession.SessionBundle, error) {
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
	pan, ok := authenticationDigits(number)
	number = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if !ok || !validPAN(pan) {
		return nil, authFailure("invalid_card", nil)
	}
	if e = captcha.validate(); e != nil {
		return nil, e
	}
	c, e := a.loadConfig(ctx)
	if e != nil {
		return nil, e
	}
	id, key := c.CardEncryptionKey()
	if !authInput(id, 256) || key == "" {
		return nil, authFailure("missing_card_key", nil)
	}
	encrypted, e := rsaoaep.Encrypt(key, pan)
	pan = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		return nil, authFailure("invalid_card_key", nil)
	}
	body := a.withContext(map[string]any{"keyId": id, "cardNumber": encrypted})
	if captcha.Code != nil {
		body["captchaCode"] = *captcha.Code
	}
	if captcha.AudioCode != nil {
		body["audioCaptchaCode"] = *captcha.AudioCode
	}
	p, e := a.postJSON(ctx, c, "/api/v1/cardlogin/identify", body)
	if e != nil {
		return nil, e
	}
	info, ok := p["confirmInfo"].(map[string]any)
	if !ok {
		return nil, authFailure("invalid_otp_challenge", nil)
	}
	a.otpPending = true
	return nil, &sdkErrs.PinOTPRequired{Lifetime: authNonnegative(info["lifetime"], true), RemainingAttempts: authNonnegative(info["remainingAttempts"], false)}
}
func (a *CardAuth) ConfirmOTP(ctx context.Context, code string) (*sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if !a.otpPending || a.config == nil {
		return nil, authFailure("otp_not_pending", nil)
	}
	if !authInput(code, 32) {
		return nil, authFailure("invalid_otp_code", nil)
	}
	p, e := a.postJSON(ctx, *a.config, "/api/v1/cardlogin/confirm", a.withContext(map[string]any{"confirmPassword": code}))
	code = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if e != nil {
		return nil, e
	}
	if p["createPinAvailable"] == true {
		key, ok := p["publicKey"].(string)
		if !ok || !authInput(key, 16384) {
			return nil, authFailure("invalid_pin_public_key", nil)
		}
		a.pinPublicKey = key
		a.otpPending = false
		return nil, nil
	}
	if redirect, ok := p["redirect"].(string); ok && redirect != "" {
		b, e := a.finishRedirect(ctx, *a.config, redirect)
		if e == nil {
			a.otpPending = false
		}
		return &b, e
	}
	// The bank requires explicit account registration or credential recovery.
	// Never reset existing login/password values as a side effect of signing in.
	return nil, authFailure("card_account_setup_required", nil)
}
func (a *CardAuth) RetryOTP(ctx context.Context) (*sdkErrs.PinOTPRequired, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if !a.otpPending || a.config == nil {
		return nil, authFailure("otp_not_pending", nil)
	}
	p, e := a.postJSON(ctx, *a.config, "/api/v1/cardlogin/retry", a.withContext(map[string]any{}))
	if e != nil {
		return nil, e
	}
	i, ok := p["confirmInfo"].(map[string]any)
	if !ok {
		return nil, authFailure("invalid_otp_challenge", nil)
	}
	return &sdkErrs.PinOTPRequired{Lifetime: authNonnegative(i["lifetime"], true), RemainingAttempts: authNonnegative(i["remainingAttempts"], false)}, nil
}
func validPAN(pan string) bool {
	if len(pan) < 12 || len(pan) > 19 {
		return false
	}
	sum := 0
	for i := len(pan) - 1; i >= 0; i-- {
		d := int(pan[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if (len(pan)-1-i)%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}
