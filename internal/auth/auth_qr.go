package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

type QRStatus string

const (
	QRNew         QRStatus = "new"
	QRWaitConfirm QRStatus = "wait_confirm"
	QRConfirmed   QRStatus = "confirmed"
	QRRefused     QRStatus = "refused"
	QRExpired     QRStatus = "expired"
)

// QRCode contains an authentication challenge. Only Content reveals its value.
// Print the value only when displaying a QR to the account owner.
type QRCode struct {
	content  **string
	size     int
	lifetime *int
}

func (c QRCode) Content() string {
	if c.content == nil {
		return ""
	}
	return **c.content
}
func (c QRCode) Size() int { return c.size }
func (c QRCode) Lifetime() *int {
	if c.lifetime == nil {
		return nil
	}
	v := *c.lifetime
	return &v
}
func (QRCode) String() string               { return "QRCode(<redacted>)" }
func (c QRCode) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, c.String()) }
func (QRCode) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

// QRAuth polls a challenge until the owner confirms it in the bank application.
// Start is explicit; Poll never creates a new QR after expiry or refusal.
type qrState struct {
	guid                  string
	status                QRStatus
	expires               time.Time
	verificationAttempted bool
}
type QRAuth struct {
	*authFlow
	qr **qrState
}

func (a *QRAuth) state() *qrState { return *a.qr }

func NewQRAuth(o AuthOptions) (*QRAuth, error) {
	f, e := newAuthFlow(sdkSession.SessionBundle{APIBase: sdkSession.AppOrigin, WebBase: sdkSession.AppOrigin, Browser: o.Browser, Deviceprint: o.Deviceprint, AntifraudDeviceprint: o.AntifraudDeviceprint}, o, true)
	if e != nil {
		return nil, e
	}
	state := &qrState{}
	return &QRAuth{authFlow: f, qr: &state}, nil
}
func (a *QRAuth) Start(ctx context.Context) (QRCode, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return QRCode{}, e
	}
	defer done()
	if a.state().guid != "" {
		return QRCode{}, authFailure("qr_already_pending", nil)
	}
	c, e := a.loadConfig(ctx)
	if e != nil {
		return QRCode{}, e
	}
	body := map[string]any{"channel": a.uapiChannel(nil), "identifier": []any{map[string]any{"type": "qr", "data": map[string]any{"qr_size": c.QRSize(), "qr_format": "content"}}}}
	if c.QRScope() != "" {
		body["scope"] = map[string]any{"type": c.QRScope()}
	}
	p, e := a.postUAPI(ctx, "/uapi/v2/identify", body)
	if e != nil {
		return QRCode{}, e
	}
	op, _ := p["operation"].(map[string]any)
	guid, _ := op["guid"].(string)
	id, _ := p["identifier"].(map[string]any)
	data, _ := id["data"].(map[string]any)
	value, _ := data["value"].(string)
	if !authInput(guid, 8192) || !authInput(value, 16384) || id["type"] != "qr" || data["qr_format"] != "content" {
		return QRCode{}, authFailure("invalid_qr_challenge", nil)
	}
	life := authNonnegative(id["lifetime"], true)
	if life != nil {
		if *life == 0 || *life > 3600 {
			return QRCode{}, authFailure("invalid_qr_challenge", nil)
		}
		a.state().expires = time.Now().Add(time.Duration(*life) * time.Second)
	}
	size := c.QRSize()
	if v := authNonnegative(data["qr_size"], false); v != nil {
		if *v < 64 || *v > 2048 {
			return QRCode{}, authFailure("invalid_qr_challenge", nil)
		}
		size = *v
	}
	a.state().guid = guid
	a.state().status = QRNew
	a.qrPending = true
	ptr := &value
	return QRCode{content: &ptr, size: size, lifetime: life}, nil
}
func (a *QRAuth) Poll(ctx context.Context) (QRStatus, *sdkSession.SessionBundle, error) {
	ctx, done, e := a.enter(ctx)
	if e != nil {
		return "", nil, e
	}
	defer done()
	if a.authenticated {
		b, e := a.bundle.Clone()
		return QRConfirmed, &b, e
	}
	if a.state().guid == "" || a.config == nil {
		return "", nil, authFailure("qr_not_pending", nil)
	}
	if a.state().verificationAttempted {
		return a.state().status, nil, authFailure("qr_verification_uncertain", nil)
	}
	if a.state().status == QRRefused || a.state().status == QRExpired {
		return a.state().status, nil, nil
	}
	if !a.state().expires.IsZero() && !time.Now().Before(a.state().expires) {
		a.state().status = QRExpired
		a.qrPending = false
		return a.state().status, nil, nil
	}
	p, e := a.postUAPI(ctx, "/uapi/v2/getOperation", map[string]any{"channel": map[string]any{"type": "web_sbol"}, "operation": map[string]any{"scope": []string{"status"}, "guid": a.state().guid}})
	if e != nil {
		var auth *sdkErrs.PinAuthError
		if errors.As(e, &auth) && (auth.Code == "no_operation" || auth.Code == "operation_timeout" || auth.Code == "life_time_limit_reached") {
			a.state().status = QRExpired
			a.qrPending = false
			return a.state().status, nil, nil
		}
		return a.state().status, nil, e
	}
	if len(p) == 0 {
		return a.state().status, nil, nil
	}
	op, ok := p["operation"].(map[string]any)
	if !ok {
		return a.state().status, nil, authFailure("invalid_qr_state", nil)
	}
	status, ok := op["status"].(string)
	if !ok {
		return a.state().status, nil, authFailure("invalid_qr_state", nil)
	}
	switch QRStatus(status) {
	case QRNew, QRWaitConfirm:
		a.state().status = QRStatus(status)
		return a.state().status, nil, nil
	case QRRefused, QRExpired:
		a.state().status = QRStatus(status)
		a.qrPending = false
		return a.state().status, nil, nil
	case QRConfirmed:
	default:
		return a.state().status, nil, authFailure("invalid_qr_state", nil)
	}
	token, ok := op["ouid"].(string)
	if !ok || !authInput(token, 8192) {
		return a.state().status, nil, authFailure("invalid_auth_token", nil)
	}
	a.state().verificationAttempted = true
	p, e = a.postUAPI(ctx, "/uapi/v2/verify", map[string]any{"identifier": uapiIdentifier("ouid", token), "authenticator": map[string]any{"type": "pat_check"}, "channel": a.uapiChannel(nil)})
	if e != nil {
		return a.state().status, nil, e
	}
	b, e := a.finishUAPI(ctx, p)
	if e == nil {
		a.state().status = QRConfirmed
	}
	return a.state().status, b, e
}
