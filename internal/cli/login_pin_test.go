package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

type policyRejectedPrimary struct {
	configuredPrimary
	creations int
	failure   error
	always    bool
}

func (a *policyRejectedPrimary) CreatePIN(ctx context.Context, pin string) (sber.SessionBundle, error) {
	a.creations++
	if a.creations == 1 || a.always {
		return sber.SessionBundle{}, a.failure
	}
	return a.syntheticPrimary.CreatePIN(ctx, pin)
}

func TestRejectedNewPINRequiresFreshOwnerInputWithoutRepeatingLoginOrSMS(t *testing.T) {
	for _, tt := range []struct {
		name     string
		failure  *sber.PinAuthError
		always   bool
		attempts int
		status   int
	}{
		{"PIN policy", &sber.PinAuthError{Code: "invalid_pin", StatusCode: 400}, false, 2, 0},
		{"birthdate policy", &sber.PinAuthError{Code: "invalid_pin_birthdate", StatusCode: 422}, false, 2, 0},
		{"bounded owner corrections", &sber.PinAuthError{Code: "invalid_pin", StatusCode: 400}, true, 3, 3},
		{"uncertain service error", &sber.PinAuthError{Code: "invalid_pin", StatusCode: 500}, false, 1, 3},
		{"encryption error", &sber.PinAuthError{Code: "invalid_decode_pin", StatusCode: 400}, false, 1, 3},
		{"reset identity", &sber.PinAuthError{Code: "invalid_pin", StatusCode: 400, ResetCookies: true}, false, 1, 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			auth := &policyRejectedPrimary{failure: tt.failure, always: tt.always}
			newInputs := 0
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "profile.json")}, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) { return auth, nil },
				ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
					if prompt == ownerinput.NewPIN {
						newInputs++
					}
					return "13579", nil
				},
			}})
			if code != tt.status || auth.creations != tt.attempts || newInputs != tt.attempts {
				t.Fatal("PIN policy handling omitted fresh input, exceeded its bound, or replayed an uncertain attempt")
			}
			if !strings.HasPrefix(strings.Join(auth.steps, ","), "login,otp,") || auth.steps[len(auth.steps)-1] != "close" {
				t.Fatal("PIN correction lost the existing authentication state or cleanup")
			}
			if strings.Count(strings.Join(auth.steps, ","), "login") != 1 || strings.Count(strings.Join(auth.steps, ","), "otp") != 1 || strings.Contains(diagnostics.String(), "13579") {
				t.Fatal("PIN correction repeated primary authentication or exposed secret input")
			}
		})
	}
}

type configuredPrimary struct{ syntheticPrimary }

func (*configuredPrimary) LoadConfig(context.Context) (sber.FrontendConfig, error) {
	return sber.NewFrontendConfig("", "", 5, "", "", false, false), nil
}

func TestOwnerLoginCorrectsLocalPINWithoutRepeatingAuthentication(t *testing.T) {
	auth := &configuredPrimary{}
	inputs := []struct {
		prompt ownerinput.Prompt
		value  string
	}{
		{ownerinput.Login, "synthetic-private-login"},
		{ownerinput.Password, "synthetic-private-password"},
		{ownerinput.OTP, "synthetic-private-otp"},
		{ownerinput.NewPIN, "1234"},
		{ownerinput.NewPIN, "54321"},
		{ownerinput.ConfirmPIN, "54322"},
		{ownerinput.NewPIN, "54321"},
		{ownerinput.ConfirmPIN, "54321"},
	}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "profile.json")}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPrimary: func() (PrimaryAuthenticator, error) { return auth, nil },
		ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
			if len(inputs) == 0 || inputs[0].prompt != prompt {
				t.Fatal("invalid PIN did not preserve the challenge sequence")
			}
			value := inputs[0].value
			inputs = inputs[1:]
			return value, nil
		},
	}})
	if code != 0 || len(inputs) != 0 || strings.Join(auth.steps, ",") != "login,otp,pin,close" {
		t.Fatal("local PIN correction restarted or lost authentication")
	}
	if !strings.Contains(diagnostics.String(), "requires 5 digits") || !strings.Contains(diagnostics.String(), "confirmation differs") {
		t.Fatal("PIN correction did not explain its requirements")
	}
	for _, secret := range []string{"synthetic-private", "1234", "54321", "54322"} {
		if strings.Contains(output.String()+diagnostics.String(), secret) {
			t.Fatal("PIN correction exposed private input")
		}
	}
}
