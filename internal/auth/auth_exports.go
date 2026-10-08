package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func (f *authFlow) resetProcess() {
	f.config = nil
	f.srp = nil
	f.csrf = ""
	f.otpPending = false
	f.qrPending = false
	f.primarySRP = nil
	f.primaryToken = ""
	f.primaryOTPPending = false
	f.pinPublicKey = ""
}
func authNonnegative(v any, coerce bool) *int {
	var text string
	switch n := v.(type) {
	case float64:
		if n < 0 || n != float64(int(n)) {
			return nil
		}
		x := int(n)
		if x < 0 {
			return nil
		}
		return &x
	case json.Number:
		text = n.String()
	case string:
		if !coerce {
			return nil
		}
		text = n
	default:
		return nil
	}
	if text == "" {
		return nil
	}
	for _, c := range text {
		if c < '0' || c > '9' {
			return nil
		}
	}
	n, e := strconv.Atoi(text)
	if e != nil {
		return nil
	}
	return &n
}
func authOTPRequired(p map[string]any) *sdkErrs.PinOTPRequired {
	return &sdkErrs.PinOTPRequired{PinAuthError: sdkErrs.PinAuthError{RemainingAttempts: authNonnegative(p["remainingAttempts"], false)}, Lifetime: authNonnegative(p["lifetime"], false)}
}
func authCaptchaLink(p map[string]any, key, base string) *string {
	raw, ok := p[key].(string)
	if !ok {
		return nil
	}
	if !sdkSession.IsOnlineURL(raw) {
		raw = base + "/" + strings.TrimLeft(raw, "/")
	}
	u, e := sdkSession.SafeCaptchaURL(raw, base)
	if e != nil {
		return nil
	}
	parsed, _ := url.Parse(u)
	q := parsed.Query()
	q.Set("noc", strconv.FormatInt(time.Now().UnixMilli(), 10))
	parsed.RawQuery = q.Encode()
	u = parsed.String()
	return &u
}
func (f *authFlow) pinResponseError(p map[string]any, r *sdkTransport.Response, base string) error {
	code := "unknown_pin_error"
	if e, ok := p["error"].(map[string]any); ok {
		if c, ok := e["code"].(string); ok && c != "" {
			code = c
		}
	}
	var details map[string]any
	if d, ok := p["pinInfo"].(map[string]any); ok {
		details = d
	} else {
		details, _ = p["confirmInfo"].(map[string]any)
	}
	e := authFailure(code, r)
	if details != nil {
		e.RemainingAttempts = authNonnegative(details["remainingAttempts"], false)
		e.ResetCookies = authTruthy(details["resetCookies"])
	}
	if code == "need_captcha" || code == "captcha_required" || code == "invalid_captcha" {
		c, _ := p["captcha"].(map[string]any)
		return &sdkErrs.PinCaptchaRequired{PinAuthError: *e, ImageURL: authCaptchaLink(c, "imageUrl", base), AudioURL: authCaptchaLink(c, "audioUrl", base)}
	}
	return e
}
func authTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case map[string]any:
		return len(x) > 0
	case []any:
		return len(x) > 0
	}
	return true
}
func (a *PINAuth) ConfirmOTP(ctx context.Context, code string) (sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return sdkSession.SessionBundle{}, e
	}
	defer done()
	if !a.otpPending || a.config == nil {
		return sdkSession.SessionBundle{}, authFailure("otp_not_pending", nil)
	}
	if !authInput(code, 32) {
		return sdkSession.SessionBundle{}, authFailure("invalid_otp_code", nil)
	}
	p, e := a.postJSON(ctx, *a.config, "/api/v1/pin/otp/confirm", a.withContext(map[string]any{"confirmPassword": code}))
	if e != nil {
		return sdkSession.SessionBundle{}, e
	}
	redirect, ok := p["redirect"].(string)
	if !ok || redirect == "" {
		return sdkSession.SessionBundle{}, authFailure("missing_redirect", nil)
	}
	return a.finishRedirect(ctx, *a.config, redirect)
}
func (a *PINAuth) RetryOTP(ctx context.Context) (*sdkErrs.PinOTPRequired, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if !a.otpPending || a.config == nil {
		return nil, authFailure("otp_not_pending", nil)
	}
	body := map[string]any{}
	if a.options.IsPWA {
		body["isPwa"] = true
	}
	p, e := a.postJSON(ctx, *a.config, "/api/v1/pin/otp/retry", body)
	if e != nil {
		return nil, e
	}
	info, ok := p["confirmInfo"].(map[string]any)
	if !ok {
		return nil, authFailure("invalid_otp_challenge", nil)
	}
	return authOTPRequired(info), nil
}

// FetchCaptcha retrieves an observed challenge asset with this process's cookies.
// It never submits credentials, follows redirects, or solves the challenge.
func (f *authFlow) FetchCaptcha(ctx context.Context, raw string) ([]byte, error) {
	ctx, done, e := f.enter(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	if f.config == nil {
		return nil, authFailure("config_not_loaded", nil)
	}
	target, e := sdkSession.SafeCaptchaURL(raw, f.config.BaseURL())
	if e != nil {
		return nil, e
	}
	u, _ := url.Parse(target)
	audio := strings.Contains(strings.ToLower(u.Path), "audio")
	accept := "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8"
	dest := "image"
	if audio {
		accept = "audio/webm,audio/ogg,audio/wav,audio/*;q=0.9,*/*;q=0.8"
		dest = "audio"
	}
	o := sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: sdkTransport.HeaderOverrides{"Accept": sdkTransport.PtrString(accept), "Content-Type": nil, "Origin": nil, "Referer": sdkTransport.PtrString(sdkTransport.PublicBootstrapURL), "Sec-Fetch-Dest": sdkTransport.PtrString(dest), "Sec-Fetch-Mode": sdkTransport.PtrString("no-cors"), "Sec-Fetch-Site": sdkTransport.PtrString(authFetchSite(sdkTransport.PublicBootstrapURL, target)), "X-Requested-With": nil}}
	r, e := f.transport.Get(ctx, target, o)
	if err := f.check(ctx); err != nil {
		return nil, err
	}
	if e != nil {
		return nil, authRequestError(e)
	}
	if r == nil || r.StatusCode != 200 {
		return nil, authFailure("captcha_fetch_failed", r)
	}
	if len(r.Content) == 0 || len(r.Content) > 10*1024*1024 {
		return nil, authFailure("invalid_captcha_body", r)
	}
	return append([]byte(nil), r.Content...), nil
}

func authRequestError(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, sdkErrs.ErrClosed) {
		return sdkErrs.ErrClosed
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return sdkTransport.TransportFailure(e)
	}
	var safe sdkErrs.SberError
	if errors.As(e, &safe) {
		return safe
	}
	return &sdkErrs.TransportError{Code: "request_failed"}
}

// PINProvider is called by future renewal clients at the point of use; the
// returned secret is ephemeral. Auth objects never retain a provider or PIN.
type PINProvider func(context.Context) (string, error)
type AuthStage string

const (
	AuthStageBootstrap     AuthStage = "bootstrap"
	AuthStageConfigured    AuthStage = "configured"
	AuthStageOTP           AuthStage = "otp"
	AuthStageQR            AuthStage = "qr"
	AuthStagePINEnrollment AuthStage = "pin_enrollment"
	AuthStageAuthenticated AuthStage = "authenticated"
	AuthStageClosed        AuthStage = "closed"
)

// Stage is a serialized snapshot and may wait for an in-flight operation.
func (f *authFlow) Stage() AuthStage {
	_, done, e := f.enter(context.Background())
	if e != nil {
		return AuthStageClosed
	}
	defer done()
	if f.authenticated {
		return AuthStageAuthenticated
	}
	if f.otpPending || f.primaryOTPPending {
		return AuthStageOTP
	}
	if f.qrPending {
		return AuthStageQR
	}
	if f.pinPublicKey != "" {
		return AuthStagePINEnrollment
	}
	if f.config != nil {
		return AuthStageConfigured
	}
	return AuthStageBootstrap
}

// GenerateFingerprints mints the audited synthetic device identity and its
// percent-encoded antifraud counterpart. It does not spoof observed HTTP/TLS.
func GenerateFingerprints() (sdkSession.Deviceprint, sdkSession.Deviceprint, error) {
	d, e := sdkSession.GenerateDeviceprint()
	if e != nil {
		return sdkSession.Deviceprint{}, sdkSession.Deviceprint{}, e
	}
	a, e := sdkSession.GenerateAntifraudDeviceprint(d.Value())
	if e != nil {
		return sdkSession.Deviceprint{}, sdkSession.Deviceprint{}, e
	}
	return d, a, nil
}
func NewPINAuthFromProfile(path string, o AuthOptions, load ...sdkSession.SessionLoadOptions) (*PINAuth, error) {
	b, e := sdkSession.LoadSessionBundle(path, load...)
	if e != nil {
		return nil, e
	}
	return NewPINAuth(b, o)
}
func NewPrimaryAuthFromProfile(path string, o AuthOptions, load ...sdkSession.SessionLoadOptions) (*PrimaryAuth, error) {
	b, e := sdkSession.LoadSessionBundle(path, load...)
	if e != nil {
		return nil, e
	}
	return NewPrimaryAuthFromBundle(b, o)
}
