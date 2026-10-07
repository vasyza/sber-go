package sber

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAuthOptionsFormattingIsAlwaysRedacted(t *testing.T) {
	d := "synthetic-device-secret"
	o := AuthOptions{Deviceprint: &d, AntifraudDeviceprint: &d, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return nil, nil }}
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
	s.steps = append(s.steps, authStep{method: "POST", target: AppOrigin + "/CSAFront/api/v1/pin/create", response: authJSON(503, map[string]any{"error": map[string]any{"code": "temporarily_unavailable"}})}, authStep{method: "POST", target: AppOrigin + "/CSAFront/api/v1/pin/create", response: authJSON(200, map[string]any{})}, authStep{method: "POST", target: AppOrigin + "/CSAFront/api/v1/auth", err: &TransportError{Code: "synthetic-failure"}})
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
