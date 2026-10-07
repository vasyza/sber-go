package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

type preparingPrimary struct {
	syntheticPrimary
	prepared bool
	failure  error
}

func (a *preparingPrimary) LoadConfig(context.Context) (sber.FrontendConfig, error) {
	a.prepared = true
	return sber.NewFrontendConfig(sber.AppOrigin, "synthetic-process", 5, "", "", true, false), a.failure
}

type preparingPIN struct {
	syntheticRemembered
	prepared bool
	failure  error
}

func (a *preparingPIN) LoadConfig(context.Context) (sber.FrontendConfig, error) {
	a.prepared = true
	return sber.FrontendConfig{}, a.failure
}

func TestAuthenticationPreparesPublicConfigurationBeforeSecretInput(t *testing.T) {
	for _, mode := range []string{"primary", "remembered", "refresh"} {
		t.Run(mode, func(t *testing.T) {
			profile := filepath.Join(testPrivateDir(t), "selected.json")
			if mode == "refresh" {
				if err := syntheticLoginBundle().Save(profile); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(profile)
			failure := errors.New("synthetic-private-browser-detail")
			primary := &preparingPrimary{failure: failure}
			pin := &preparingPIN{failure: failure}
			args := []string{"login", "--profile", profile}
			if mode == "remembered" {
				args = append(args, "--remembered-profile", "synthetic-identity")
			} else if mode == "refresh" {
				args[0] = "refresh-session"
			}
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), args, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) { return primary, nil },
				NewPIN:     func(string, sber.AuthOptions) (PINAuthenticator, error) { return pin, nil },
				ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
					t.Fatal("public preparation failure requested a secret")
					return "", nil
				},
			}})
			if code != 3 || output.Len() != 0 || bytes.Contains(diagnostics.Bytes(), []byte("synthetic-private")) {
				t.Fatal("public preparation failure escaped the CLI boundary")
			}
			after, _ := os.ReadFile(profile)
			if !bytes.Equal(before, after) || primary.steps != nil && len(primary.steps) != 1 || pin.loginCalls != 0 || pin.otpCalls != 0 {
				t.Fatal("preparation failure changed the profile or sent authentication")
			}
			if mode == "primary" {
				if !primary.prepared || len(primary.steps) != 1 || primary.steps[0] != "close" {
					t.Fatal("primary preparation or cleanup was omitted")
				}
			} else if !pin.prepared || pin.closeCalls != 1 {
				t.Fatal("PIN preparation or cleanup was omitted")
			}
		})
	}
}

func TestPrimaryPreparedConfigurationIsReadyAtFirstSecretPrompt(t *testing.T) {
	auth := &preparingPrimary{}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "new.json")}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPrimary: func() (PrimaryAuthenticator, error) { return auth, nil },
		ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
			if !auth.prepared {
				t.Fatal("secret prompt preceded public preparation")
			}
			if prompt == ownerinput.NewPIN || prompt == ownerinput.ConfirmPIN {
				return "12345", nil
			}
			return "synthetic-secret", nil
		},
	}})
	if code != 0 || len(auth.steps) != 4 || auth.steps[0] != "login" || auth.steps[3] != "close" {
		t.Fatal("public preparation changed the one-attempt primary flow")
	}
}
