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
