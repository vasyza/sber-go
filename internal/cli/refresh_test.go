package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

func TestRefreshSessionUsesPINAndOTPPreservesProfileOnFailure(t *testing.T) {
	for _, tt := range []struct {
		name    string
		failure error
		cleanup error
		code    int
	}{
		{"success", nil, nil, 0},
		{"login failure", errors.New("synthetic-private-bank-error"), nil, 3},
		{"cleanup failure", nil, errors.New("synthetic-private-cleanup-error"), 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profile := filepath.Join(testPrivateDir(t), "selected.json")
			old := syntheticLoginBundle()
			old.Cookies[0].Value = "synthetic-old-cookie"
			if err := old.Save(profile); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(profile)
			if err != nil {
				t.Fatal(err)
			}
			auth := &syntheticRemembered{needsOTP: true, failure: tt.failure, closeError: tt.cleanup}
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), []string{"refresh-session", "--profile", profile}, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPIN: func(path string, options sber.AuthOptions) (PINAuthenticator, error) {
					if path != profile || options.TransportOptions.CABundle != "" {
						t.Fatal("refresh lost profile or default certificate trust")
					}
					return auth, nil
				},
				ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
					switch prompt {
					case ownerinput.PIN:
						return "12345", nil
					case ownerinput.OTP:
						return "654321", nil
					}
					t.Fatal("refresh requested primary credentials or a new PIN")
					return "", nil
				},
			}})
			if code != tt.code || auth.loginCalls != 1 || auth.closeCalls != 1 {
				t.Fatalf("refresh status=%d, login=%d, close=%d", code, auth.loginCalls, auth.closeCalls)
			}
			after, err := os.ReadFile(profile)
			if err != nil {
				t.Fatal(err)
			}
			if tt.code == 0 {
				if bytes.Equal(before, after) || auth.otpCalls != 1 || !strings.Contains(output.String(), `"session_refreshed":true`) {
					t.Fatal("refresh did not publish the authenticated profile")
				}
			} else if !bytes.Equal(before, after) || output.Len() != 0 {
				t.Fatal("failed refresh changed the profile or returned success")
			}
			for _, secret := range []string{"12345", "654321", "synthetic-private", "synthetic-old-cookie"} {
				if strings.Contains(output.String()+diagnostics.String(), secret) {
					t.Fatal("refresh exposed sensitive data")
				}
			}
		})
	}
}

func TestExpiredCLIReportsRecoveryInstruction(t *testing.T) {
	client := &testutil.Client{Read: func(context.Context, string, map[string]any) (map[string]any, error) {
		return nil, &sber.AuthenticationExpired{}
	}}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"products", "--profile", "synthetic-selected"}, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
	if code != 3 || output.Len() != 0 || client.Closes.Load() != 1 || !strings.Contains(diagnostics.String(), "refresh-session") {
		t.Fatal("expired session did not give a safe recovery action")
	}
}

func TestCommandHelpDoesNotOpenProfilesOrRequestSecrets(t *testing.T) {
	for _, args := range [][]string{
		{"products", "--help"}, {"login", "--help"}, {"help", "operations"}, {"refresh-session", "--help"},
		{"products", "--profile", "/synthetic/selected", "--help"},
		{"login", "--profile", "/synthetic/selected", "--help"},
		{"refresh-session", "--profile", "/synthetic/selected", "--help"},
		{"status", "--profile", "/synthetic/selected", "--help"},
	} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), args, &output, &diagnostics, Options{
			OpenClient: func(string) (mcp.Client, error) { t.Fatal("help opened a profile"); return nil, nil },
			Authentication: &Authentication{ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
				t.Fatal("help requested secret input")
				return "", nil
			}},
		})
		if code != 0 || output.Len() == 0 || diagnostics.Len() != 0 || !strings.Contains(output.String(), "--profile") {
			t.Fatalf("command help failed for %s", args[0])
		}
	}
}
