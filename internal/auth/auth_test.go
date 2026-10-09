package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

func TestAuthFactoriesFingerprintsAndProfileOptions(t *testing.T) {
	d, anti, e := GenerateFingerprints()
	if e != nil || d.Value() == "" || anti.Value() == "" {
		t.Fatal("identity pair generation failed")
	}
	want, _ := sdkSession.GenerateAntifraudDeviceprint(d.Value())
	if anti.Value() != want.Value() {
		t.Fatal("antifraud identity not derived from this deviceprint")
	}
	b := authBundle(t)
	path := filepath.Join(testPrivateDir(t), "synthetic-profile.json")
	if e = b.Save(path); e != nil {
		t.Fatal(e)
	}
	created := 0
	o := AuthOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: "synthetic-ca", Timeout: 123}, TransportFactory: func(b sdkSession.SessionBundle, o sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		created++
		if o.CABundle != "synthetic-ca" || o.Timeout != 123 || !o.AllowUnready || b.Deviceprint == nil {
			t.Error("factory dropped auth options/profile")
		}
		return newAuthScript(t), nil
	}}
	pin, e := NewPINAuthFromProfile(path, o)
	if e != nil {
		t.Fatal(e)
	}
	if pin.Stage() != AuthStageBootstrap {
		t.Fatal("anonymous profile claimed ready")
	}
	_ = pin.Close()
	if pin.Stage() != AuthStageClosed {
		t.Fatal("stage not closed")
	}
	primary, e := NewPrimaryAuthFromProfile(path, o)
	if e != nil {
		t.Fatal(e)
	}
	_ = primary.Close()
	dValue := d.Value()
	o.Deviceprint = &dValue
	cold, e := NewPrimaryAuth(o)
	if e != nil {
		t.Fatal(e)
	}
	export, e := cold.ExportSession()
	if e != nil || len(export.Cookies) != 0 {
		t.Fatal("cold auth did not start blank")
	}
	_ = cold.Close()
	if created != 3 {
		t.Fatal("factory construction count mismatch")
	}
	var cb PINProvider = func(context.Context) (string, error) { return "13579", nil }
	_ = cb
	if strings.Contains(fmt.Sprintf("%+v", pin), "synthetic-ca") {
		t.Fatal("auth leaked options")
	}
}
func TestAuthPrimarySuccessClearsProcessProof(t *testing.T) {
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"pinInfo": map[string]any{"webPinSkip": true}})
	s.steps = append(s.steps, primaryFinishSteps(t, s)...)
	a := newPrimaryForScript(t, s)
	if _, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{}); e != nil {
		t.Fatal(e)
	}
	if a.Stage() != AuthStageAuthenticated || a.primarySRP != nil || a.primaryToken != "" || a.primaryOTPPending || a.pinPublicKey != "" {
		t.Fatal("completed primary retained process proof/token state")
	}
}

func TestAuthOptionsFormattingIsAlwaysRedacted(t *testing.T) {
	d := "synthetic-device-secret"
	o := AuthOptions{Deviceprint: &d, AntifraudDeviceprint: &d, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return nil, nil
	}}
	b, e := json.Marshal(o)
	if e != nil || strings.Contains(string(b), d) || strings.Contains(fmt.Sprintf("%+v %#v", o, o), d) {
		t.Fatal("auth options not explicitly redacted")
	}
}
func TestAuthPrimaryEnrollmentKeyRetryBoundaries(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	public := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"pinInfo": map[string]any{"publicKey": public}})
	s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/create", response: authJSON(503, map[string]any{"error": map[string]any{"code": "temporarily_unavailable"}})}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/create", response: authJSON(200, map[string]any{})}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/auth", err: &sdkErrs.TransportError{Code: "synthetic-failure"}})
	a := newPrimaryForScript(t, s)
	b, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	if e != nil || b != nil {
		t.Fatal("enrollment state not returned")
	}
	_, e = a.CreatePIN(context.Background(), "1234")
	authErrorCode(t, e, "invalid_pin")
	if a.pinPublicKey != public || s.calls != 3 {
		t.Fatal("invalid input consumed enrollment key")
	}
	_, e = a.CreatePIN(context.Background(), "13579")
	authErrorCode(t, e, "temporarily_unavailable")
	if a.pinPublicKey != public {
		t.Fatal("rejected PIN consumed enrollment key")
	}
	_, e = a.CreatePIN(context.Background(), "13579")
	if e == nil || a.pinPublicKey != "" {
		t.Fatal("accepted PIN did not consume key before finish")
	}
	_, e = a.CreatePIN(context.Background(), "13579")
	authErrorCode(t, e, "pin_create_not_ready")
	if s.calls != 6 {
		t.Fatal("spent PIN enrollment replayed")
	}
}
func TestAuthMalformedRSAHasNoPostOrSecretRetention(t *testing.T) {
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"pinInfo": map[string]any{"publicKey": "synthetic-invalid-key"}})
	a := newPrimaryForScript(t, s)
	_, _ = a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	_, e := a.CreatePIN(context.Background(), "13579")
	authErrorCode(t, e, "invalid_pin_public_key")
	if s.calls != 3 || a.pinPublicKey == "" || strings.Contains(fmt.Sprintf("%+v", e), "13579") {
		t.Fatal("malformed RSA replay/secret/state failure")
	}
}

// Synthetic scripts port pin.py and tests/test_{pin,primary}_auth.py; they
// cannot make a network request. Flow-only fake proofs are NOT crypto KATs.
type authStep struct {
	method, target string
	response       *sdkTransport.Response
	err            error
	inspect        func(map[string]any, map[string]string, sdkTransport.RequestOptions)
	run            func(context.Context)
}
type authScript struct {
	t      *testing.T
	mu     sync.Mutex
	jar    *sdkSession.CookieJar
	steps  []authStep
	calls  int
	closes int
}

func newAuthScript(t *testing.T, steps ...authStep) *authScript {
	t.Helper()
	j, err := sdkSession.NewCookieJar(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &authScript{t: t, jar: j, steps: steps}
}
func (s *authScript) request(ctx context.Context, method, target string, body map[string]any, form map[string]string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	s.mu.Lock()
	if s.calls >= len(s.steps) {
		s.mu.Unlock()
		s.t.Error("unexpected request; credentials must not replay")
		return nil, errors.New("unexpected synthetic request")
	}
	step := s.steps[s.calls]
	s.calls++
	s.mu.Unlock()
	if method != step.method || target != step.target {
		s.t.Errorf("wrong synthetic request method/path (step %d)", s.calls)
	}
	if step.inspect != nil {
		step.inspect(body, form, o)
	}
	if step.run != nil {
		step.run(ctx)
	}
	return step.response, step.err
}
func (s *authScript) Get(ctx context.Context, u string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	return s.request(ctx, "GET", u, nil, nil, o)
}
func (s *authScript) Post(ctx context.Context, u string, b map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	return s.request(ctx, "POST", u, b, nil, o)
}
func (s *authScript) PostForm(ctx context.Context, u string, b map[string]string, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	return s.request(ctx, "FORM", u, nil, b, o)
}
func (s *authScript) CookieJar() *sdkSession.CookieJar { return s.jar }
func (s *authScript) Close() error                     { s.mu.Lock(); defer s.mu.Unlock(); s.closes++; return nil }
func authHTML(primary bool) string {
	n := strings.Repeat("f", 512)
	if primary {
		return `<script>window.config = {baseApiUrl:"CSAFront",processId:"process-fixture",authTypeByCookie:"start",srpConfig:{enabled:true,N:"` + n + `",g:"2"},pinConfig:{enabled:true,hasPin:false},validation:{pin:{length:5}},isSeamlessWeb:true,isUfsRedirectMethodPostEnabled:true};</script>`
	}
	return `<script>window.config = {baseApiUrl:"CSAFront",processId:"process-fixture",authTypeByCookie:"pin",pinConfig:{enabled:true,hasPin:true,length:5,srp_N:"` + n + `",srp_g:"2"}};</script>`
}
func authPage(text string) *sdkTransport.Response {
	return &sdkTransport.Response{StatusCode: 200, Headers: http.Header{}, Content: []byte(text)}
}
func authJSON(status int, p map[string]any) *sdkTransport.Response {
	b, _ := json.Marshal(p)
	return &sdkTransport.Response{StatusCode: status, Headers: http.Header{}, Content: b}
}
func authBundle(t *testing.T) sdkSession.SessionBundle {
	t.Helper()
	d := "version=fixture&screen=synthetic"
	b, err := sdkSession.NewSessionBundle(sdkSession.SessionBundle{APIBase: sdkSession.AppOrigin, WebBase: sdkSession.AppOrigin, Deviceprint: &d})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func authErrorCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *sdkErrs.PinAuthError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("expected auth code %s; got %v", code, err)
	}
}
func authHeader(o sdkTransport.RequestOptions, name string) string {
	p := o.Headers[name]
	if p == nil {
		return ""
	}
	return *p
}
func setAuthCookies(t *testing.T, j *sdkSession.CookieJar) {
	t.Helper()
	u, _ := url.Parse(sdkSession.AppOrigin + "/")
	if err := j.ApplySetCookie(u, []string{"UFS-SESSION=synthetic-session; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly", "UFS-TOKEN=synthetic-token; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly"}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthBootstrapStrictDefault(t *testing.T) {
	for _, tc := range []struct{ html, code string }{{authHTML(false), ""}, {"<html>malformed anonymous page</html>", "invalid_frontend_config"}, {`<script src="/TSPD/a.js"></script>Enable JavaScript`, "browser_check_required"}, {`<html><title>Нельзя войти в СберБанк Онлайн в этом браузере.</title></html>`, "login_page_rejected"}} {
		t.Run(tc.code, func(t *testing.T) {
			s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(tc.html), inspect: func(_ map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
				if authHeader(o, "Sec-Fetch-Site") != "none" || o.Headers["Content-Type"] != nil || o.Headers["Origin"] != nil {
					t.Error("not document navigation")
				}
			}})
			a, err := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
			if err != nil {
				t.Fatal(err)
			}
			c, err := a.LoadConfig(context.Background())
			if tc.code != "" {
				authErrorCode(t, err, tc.code)
			} else if err != nil || c.ProcessID() != "process-fixture" || c.PINLength() != 5 {
				t.Fatal("strict config missing")
			}
			if _, err := a.ExportSession(); err != nil {
				t.Fatal(err)
			}
			if err := a.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := a.LoadConfig(context.Background()); !errors.Is(err, sdkErrs.ErrClosed) {
				t.Fatal("closed auth resurrected")
			}
			if err := a.Close(); err != nil || s.closes != 1 {
				t.Fatal("close not idempotent")
			}
		})
	}
}

// Profile fixtures must be private independently of the developer's umask.
func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
