package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func methodOptions(s *authScript) AuthOptions {
	d := "version=fixture&screen=synthetic"
	return AuthOptions{Transport: s, Deviceprint: &d}
}

func methodBootstrap() authStep {
	return authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(true))}
}

func methodRedirect(t *testing.T, s *authScript) []authStep {
	return primaryFinishSteps(t, s)[1:]
}

func TestPhoneSRPOTPAndSession(t *testing.T) {
	s := newAuthScript(t)
	begin, proof := authJSON(200, map[string]any{}), authJSON(200, map[string]any{})
	var expectedM string
	s.steps = []authStep{methodBootstrap(), {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/authenticate", response: begin, inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		id := b["identifier"].(map[string]any)
		a := b["authenticator"].(map[string]any)
		if id["type"] != "phone_login" || id["data"].(map[string]any)["value"] != "79990000000" || a["type"] != "web_sbol_srp_pass" || b["flow"] != "authcode" {
			t.Error("phone identification contract differs")
		}
		B, salt, M, R := authServerChallenge(a["data"].(map[string]any)["srp_a"].(string), "fixture-password")
		expectedM = M
		begin.Content = authJSON(200, map[string]any{"ouid": "synthetic-srp", "authenticator": []any{map[string]any{"type": "web_sbol_srp_pass", "data": map[string]any{"srp_b": B, "srp_s": salt}}}}).Content
		proof.Content = authJSON(200, map[string]any{"srp_r": R, "ouid": "synthetic-otp", "authenticator": []any{map[string]any{"type": "sms_otp", "lifetime": 90, "attempts_remaining": 3}}}).Content
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/verify", response: proof, inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if b["identifier"].(map[string]any)["data"].(map[string]any)["value"] != "synthetic-srp" || b["authenticator"].(map[string]any)["data"].(map[string]any)["srp_m"] != expectedM {
			t.Error("phone proof or token lost")
		}
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/verify", response: authJSON(200, map[string]any{"response_data": map[string]any{"host": "https://web2.online.sberbank.ru/auth/web", "authcode": "synthetic"}}), inspect: func(b map[string]any, _ map[string]string, o sdkTransport.RequestOptions) {
		if b["authenticator"].(map[string]any)["type"] != "sms_otp" || b["identifier"].(map[string]any)["data"].(map[string]any)["value"] != "synthetic-otp" {
			t.Error("OTP contract differs")
		}
	}}}
	r := methodRedirect(t, s)
	r[0].target = "https://web2.online.sberbank.ru/auth/web?AuthToken=synthetic"
	s.steps = append(s.steps, r...)
	a, e := NewPhoneAuth(methodOptions(s))
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	_, e = a.Login(context.Background(), "+7 (999) 000-00-00", "fixture-password", PrimaryLoginOptions{})
	var otp *sdkErrs.PinOTPRequired
	if !errors.As(e, &otp) || otp.Lifetime == nil || *otp.Lifetime != 90 {
		t.Fatal("SMS challenge missing", e)
	}
	if a.Stage() != AuthStageOTP {
		t.Fatal("phone OTP stage hidden")
	}
	_, e = a.Login(context.Background(), "79990000000", "fixture-password", PrimaryLoginOptions{})
	authErrorCode(t, e, "otp_already_pending")
	b, e := a.ConfirmOTP(context.Background(), "00000")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.ToSeed(true); e != nil {
		t.Fatal(e)
	}
}

func TestPhoneRejectsFalseServerProof(t *testing.T) {
	s := newAuthScript(t)
	begin := authJSON(200, map[string]any{})
	s.steps = []authStep{methodBootstrap(), {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/authenticate", response: begin, inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
		B, salt, _, _ := authServerChallenge(b["authenticator"].(map[string]any)["data"].(map[string]any)["srp_a"].(string), "fixture-password")
		begin.Content = authJSON(200, map[string]any{"ouid": "synthetic", "authenticator": []any{map[string]any{"type": "web_sbol_srp_pass", "data": map[string]any{"srp_b": B, "srp_s": salt}}}}).Content
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/verify", response: authJSON(200, map[string]any{"srp_r": "01", "response_data": map[string]any{"host": "https://evil.example/"}})}}
	a, _ := NewPhoneAuth(methodOptions(s))
	defer a.Close()
	_, e := a.Login(context.Background(), "79990000000", "fixture-password", PrimaryLoginOptions{})
	authErrorCode(t, e, "invalid_server_proof")
}

func TestPhoneServerRequestedEncryptedPasswordTransition(t *testing.T) {
	for _, stage := range []string{"identify", "verify"} {
		t.Run(stage, func(t *testing.T) {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			if err != nil {
				t.Fatal(err)
			}
			pub := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
			s := newAuthScript(t, methodBootstrap())
			transition := authJSON(403, map[string]any{"error": map[string]any{"code": "password_bypass"}, "ouid": "synthetic-rsa", "passwordEncryptionKey": pub})
			if stage == "verify" {
				begin := authJSON(200, map[string]any{})
				s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/authenticate", response: begin, inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
					B, salt, _, _ := authServerChallenge(b["authenticator"].(map[string]any)["data"].(map[string]any)["srp_a"].(string), "fixture-password")
					begin.Content = authJSON(200, map[string]any{"ouid": "synthetic-srp", "authenticator": []any{map[string]any{"type": "web_sbol_srp_pass", "data": map[string]any{"srp_b": B, "srp_s": salt}}}}).Content
				}})
			}
			endpoint := "/uapi/v2/authenticate"
			if stage == "verify" {
				endpoint = "/uapi/v2/verify"
			}
			s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront" + endpoint, response: transition}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/authenticate", response: authJSON(200, map[string]any{"ouid": "synthetic-otp", "authenticator": []any{map[string]any{"type": "sms_otp", "lifetime": 90}}}), inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
				id := b["identifier"].(map[string]any)
				a := b["authenticator"].(map[string]any)
				if id["type"] != "ouid" || id["data"].(map[string]any)["value"] != "synthetic-rsa" || a["type"] != "encodedPassword" {
					t.Fatal("server-requested password transition lost")
				}
				ciphertext, err := base64.StdEncoding.DecodeString(a["data"].(map[string]any)["value"].(string))
				if err != nil {
					t.Fatal(err)
				}
				plain, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, key, ciphertext, nil)
				if err != nil || string(plain) != "fixture-password" {
					t.Fatal("password encryption differs")
				}
			}}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/verify", response: authJSON(200, map[string]any{"response_data": map[string]any{"host": "https://web2.online.sberbank.ru/auth/web", "authcode": "synthetic"}})})
			r := methodRedirect(t, s)
			r[0].target = "https://web2.online.sberbank.ru/auth/web?AuthToken=synthetic"
			s.steps = append(s.steps, r...)
			a, err := NewPhoneAuth(methodOptions(s))
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			_, err = a.Login(context.Background(), "79990000000", "fixture-password", PrimaryLoginOptions{})
			var otp *sdkErrs.PinOTPRequired
			if !errors.As(err, &otp) {
				t.Fatal("password transition did not reach SMS", err)
			}
			if b, err := a.ConfirmOTP(context.Background(), "00000"); err != nil || b == nil {
				t.Fatal("password transition did not create a session", err)
			}
		})
	}
}

func TestQRPendingConfirmedAndNoSecretFormatting(t *testing.T) {
	s := newAuthScript(t)
	s.steps = []authStep{methodBootstrap(), {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/identify", response: authJSON(200, map[string]any{"operation": map[string]any{"guid": "synthetic-guid"}, "identifier": map[string]any{"type": "qr", "lifetime": 120, "data": map[string]any{"value": "synthetic-qr-content", "qr_format": "content", "qr_size": 190}}}), inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
		if b["identifier"].([]any)[0].(map[string]any)["type"] != "qr" {
			t.Error("QR identify contract differs")
		}
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/getOperation", response: &sdkTransport.Response{StatusCode: 204}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/getOperation", response: authJSON(200, map[string]any{"operation": map[string]any{"status": "wait_confirm"}})}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/getOperation", response: authJSON(200, map[string]any{"operation": map[string]any{"status": "confirmed", "ouid": "synthetic-ouid"}})}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/verify", response: authJSON(200, map[string]any{"response_data": map[string]any{"host": "https://web2.online.sberbank.ru/auth/web", "authcode": "synthetic"}}), inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
		if b["authenticator"].(map[string]any)["type"] != "pat_check" || b["identifier"].(map[string]any)["data"].(map[string]any)["value"] != "synthetic-ouid" {
			t.Error("confirmed QR proof contract differs")
		}
	}}}
	r := methodRedirect(t, s)
	r[0].target = "https://web2.online.sberbank.ru/auth/web?AuthToken=synthetic"
	s.steps = append(s.steps, r...)
	a, e := NewQRAuth(methodOptions(s))
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	c, e := a.Start(context.Background())
	if e != nil || c.Content() != "synthetic-qr-content" {
		t.Fatal("QR missing", e)
	}
	if a.Stage() != AuthStageQR || c.Lifetime() == nil || *c.Lifetime() != 120 {
		t.Fatal("QR metadata or stage lost")
	}
	if strings.Contains(fmt.Sprintf("%#v", c), "synthetic-qr-content") {
		t.Fatal("QR secret escaped formatting")
	}
	for _, want := range []QRStatus{QRNew, QRWaitConfirm, QRConfirmed} {
		got, b, e := a.Poll(context.Background())
		if e != nil || got != want {
			t.Fatal("QR state", got, e)
		}
		if want == QRConfirmed {
			if b == nil {
				t.Fatal("confirmed without session")
			}
			if _, e = b.ToSeed(true); e != nil {
				t.Fatal(e)
			}
		}
	}
}

func TestCardEncryptedIdentifyConfirmAndPIN(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	pub := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PublicKey(&key.PublicKey))
	html := strings.Replace(authHTML(true), `baseApiUrl:`, `encryptionKeyId:"synthetic-key",encryptionKeyValue:"`+pub+`",baseApiUrl:`, 1)
	s := newAuthScript(t)
	s.steps = []authStep{{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(html)}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/cardlogin/identify", response: authJSON(200, map[string]any{"confirmInfo": map[string]any{"lifetime": 90}}), inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
		ct, e := base64.StdEncoding.DecodeString(b["cardNumber"].(string))
		if e != nil {
			t.Fatal(e)
		}
		plain, e := rsa.DecryptOAEP(sha1.New(), rand.Reader, key, ct, nil)
		if e != nil || string(plain) != "4111111111111111" || b["keyId"] != "synthetic-key" {
			t.Error("PAN encryption contract differs")
		}
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/cardlogin/confirm", response: authJSON(200, map[string]any{"createPinAvailable": true, "publicKey": pub}), inspect: func(b map[string]any, _ map[string]string, _ sdkTransport.RequestOptions) {
		if b["confirmPassword"] != "00000" {
			t.Error("card OTP missing")
		}
	}}, {method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/create", response: authJSON(200, map[string]any{})}}
	s.steps = append(s.steps, primaryFinishSteps(t, s)...)
	a, e := NewCardAuth(methodOptions(s))
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	_, e = a.Login(context.Background(), "4111 1111 1111 1111", CaptchaAnswer{})
	var otp *sdkErrs.PinOTPRequired
	if !errors.As(e, &otp) {
		t.Fatal(e)
	}
	if a.Stage() != AuthStageOTP {
		t.Fatal("card OTP stage hidden")
	}
	b, e := a.ConfirmOTP(context.Background(), "00000")
	if e != nil || b != nil {
		t.Fatal("PIN enrollment not retained", e)
	}
	if a.Stage() != AuthStagePINEnrollment {
		t.Fatal("card PIN stage hidden")
	}
	created, e := a.CreatePIN(context.Background(), "13579")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = created.ToSeed(true); e != nil {
		t.Fatal(e)
	}
}

func qrFixtureStart() authStep {
	return authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/identify", response: authJSON(200, map[string]any{"operation": map[string]any{"guid": "synthetic-guid"}, "identifier": map[string]any{"type": "qr", "lifetime": 120, "data": map[string]any{"value": "synthetic-qr-content", "qr_format": "content", "qr_size": 190}}})}
}
func TestQRRefusalExpiryUnknownStatusAndVerificationNoReplay(t *testing.T) {
	for _, scenario := range []string{"refused", "expired", "unknown", "proof-failure"} {
		t.Run(scenario, func(t *testing.T) {
			s := newAuthScript(t, methodBootstrap(), qrFixtureStart())
			status := "refused"
			if scenario == "unknown" {
				status = "owner-cookie-secret"
			}
			if scenario == "proof-failure" {
				status = "confirmed"
			}
			if scenario != "expired" {
				s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/getOperation", response: authJSON(200, map[string]any{"operation": map[string]any{"status": status, "ouid": "synthetic-ouid"}})})
			}
			if scenario == "proof-failure" {
				s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/uapi/v2/verify", err: errors.New("synthetic-private-transport-detail")})
			}
			a, e := NewQRAuth(methodOptions(s))
			if e != nil {
				t.Fatal(e)
			}
			defer a.Close()
			if _, e = a.Start(context.Background()); e != nil {
				t.Fatal(e)
			}
			if scenario == "expired" {
				a.state().expires = time.Now().Add(-time.Second)
			}
			got, b, e := a.Poll(context.Background())
			if b != nil {
				t.Fatal("failure published session")
			}
			switch scenario {
			case "refused", "expired":
				if e != nil || string(got) != scenario {
					t.Fatal("terminal QR state differs")
				}
				if _, _, e = a.Poll(context.Background()); e != nil {
					t.Fatal("terminal QR polled again")
				}
			case "unknown":
				authErrorCode(t, e, "invalid_qr_state")
			case "proof-failure":
				if e == nil {
					t.Fatal("uncertain QR verification succeeded")
				}
				_, _, e = a.Poll(context.Background())
				authErrorCode(t, e, "qr_verification_uncertain")
			}
		})
	}
}
func TestAuthSuccessStatusWithErrorEnvelopeCannotPublishSession(t *testing.T) {
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"state": "NOT_REQUIRED", "pinInfo": map[string]any{"webPinSkip": true}})
	s.steps = append(s.steps, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/auth", response: authJSON(200, map[string]any{"error": map[string]any{"code": "invalid_credentials"}, "redirect": "https://web2.online.sberbank.ru/auth/web?ticket=synthetic"})})
	a := newPrimaryForScript(t, s)
	_, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{})
	authErrorCode(t, e, "invalid_credentials")
	if a.Stage() == AuthStageAuthenticated {
		t.Fatal("error envelope accepted as session")
	}
}

func TestNewAuthTypesDoNotExposeProcessTokensInFormatFallbacks(t *testing.T) {
	p, _ := NewPhoneAuth(methodOptions(newAuthScript(t)))
	defer p.Close()
	p.state().token = "synthetic-secret-ouid"
	q, _ := NewQRAuth(methodOptions(newAuthScript(t)))
	defer q.Close()
	q.state().guid = "synthetic-secret-guid"
	for _, v := range []any{p, *p, q, *q} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%p", "%w", "%+w", "%#w"} {
			if strings.Contains(fmt.Sprintf(verb, v), "synthetic-secret") || strings.Contains(fmt.Errorf(verb, v).Error(), "synthetic-secret") {
				t.Errorf("process token escaped %s", verb)
			}
		}
	}
}

func TestMethodsRejectInvalidInputBeforeNetwork(t *testing.T) {
	s := newAuthScript(t)
	a, _ := NewPhoneAuth(methodOptions(s))
	defer a.Close()
	_, e := a.Login(context.Background(), "+700", "fixture", PrimaryLoginOptions{})
	authErrorCode(t, e, "invalid_phone")
	c, _ := NewCardAuth(methodOptions(s))
	defer c.Close()
	_, e = c.Login(context.Background(), "4111111111111112", CaptchaAnswer{})
	authErrorCode(t, e, "invalid_card")
}
