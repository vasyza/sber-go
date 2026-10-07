package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

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
