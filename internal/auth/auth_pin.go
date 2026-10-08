package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	"github.com/vasyza/sber-go/internal/srp"
)

// CaptchaAnswer contains an owner-supplied answer, never a solver. Nil is absent;
// image and audio answers are mutually exclusive. Formatting always redacts.
type CaptchaAnswer struct{ Code, AudioCode *string }

func (CaptchaAnswer) String() string               { return "CaptchaAnswer(<redacted>)" }
func (c CaptchaAnswer) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, c.String()) }
func (CaptchaAnswer) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }
func (c CaptchaAnswer) validate() error {
	if c.Code != nil && c.AudioCode != nil {
		return authFailure("invalid_captcha_answer", nil)
	}
	for _, s := range []*string{c.Code, c.AudioCode} {
		if s != nil && !authInput(*s, 32) {
			return authFailure("invalid_captcha_answer", nil)
		}
	}
	return nil
}
func authInput(s string, limit int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) > 0 && utf8.RuneCountInString(s) <= limit && !sdkSession.HasControls(s)
}
func validateAuthPIN(pin string, length int) error {
	if len(pin) != length {
		return authFailure("invalid_pin", nil)
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return authFailure("invalid_pin", nil)
		}
	}
	return nil
}
func (a *PINAuth) Login(ctx context.Context, pin string, answer CaptchaAnswer) (sdkSession.SessionBundle, error) {
	ctx, done, err := a.enter(ctx)
	if err != nil {
		return sdkSession.SessionBundle{}, err
	}
	defer done()
	if a.authenticated {
		return a.bundle.Clone()
	}
	if a.otpPending {
		return sdkSession.SessionBundle{}, authFailure("otp_already_pending", nil)
	}
	if err = answer.validate(); err != nil {
		return sdkSession.SessionBundle{}, err
	}
	config, err := a.loadConfig(ctx)
	if err != nil {
		return sdkSession.SessionBundle{}, err
	}
	if err = validateAuthPIN(pin, config.PINLength()); err != nil {
		return sdkSession.SessionBundle{}, err
	}
	if a.srp == nil {
		a.srp, err = srp.New(config.NHex(), config.GHex())
		if err != nil {
			return sdkSession.SessionBundle{}, authFailure("invalid_srp_challenge", nil)
		}
	}
	body := a.withContext(map[string]any{"srp_A": a.srp.PublicHex()})
	if answer.Code != nil {
		body["captchaCode"] = *answer.Code
	}
	if answer.AudioCode != nil {
		body["audioCaptchaCode"] = *answer.AudioCode
	}
	begin, err := a.postJSON(ctx, config, "/api/v1/pin/begin", body)
	if err != nil {
		return sdkSession.SessionBundle{}, err
	}
	B, okB := begin["srp_B"].(string)
	salt, okS := begin["srp_s"].(string)
	if !okB || !okS {
		return sdkSession.SessionBundle{}, authFailure("invalid_srp_challenge", nil)
	}
	M, err := a.srp.Process(pin, salt, B)
	pin = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	if err != nil {
		return sdkSession.SessionBundle{}, authFailure("invalid_srp_challenge", nil)
	}
	logon, err := a.postJSON(ctx, config, "/api/v1/pin/logon", a.withContext(map[string]any{"srp_M": M}))
	if err != nil {
		return sdkSession.SessionBundle{}, err
	}
	proof, ok := logon["srp_R"].(string)
	valid := false
	if ok {
		valid, _ = a.srp.Verify(proof)
	}
	if !valid {
		return sdkSession.SessionBundle{}, authFailure("invalid_server_proof", nil)
	}
	if info, exists := logon["confirmInfo"]; exists && info != nil {
		obj, ok := info.(map[string]any)
		if !ok {
			return sdkSession.SessionBundle{}, authFailure("invalid_otp_challenge", nil)
		}
		a.otpPending = true
		a.srp = nil
		return sdkSession.SessionBundle{}, authOTPRequired(obj)
	}
	redirect, ok := logon["redirect"].(string)
	if !ok || redirect == "" {
		return sdkSession.SessionBundle{}, authFailure("missing_redirect", nil)
	}
	return a.finishRedirect(ctx, config, redirect)
}
