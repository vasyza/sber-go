package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

func TestCLIReadRenewalKeepsOTPAuthenticatorAndClosesBeforeAdoption(t *testing.T) {
	auth := &syntheticRemembered{needsOTP: true}
	prompts := []ownerinput.Prompt{}
	var output, diagnostics bytes.Buffer
	client := &testutil.Client{}
	code := RunWithOptions(context.Background(), []string{"products", "--profile", "synthetic-selected", "--timeout", "45s"}, &output, &diagnostics, Options{
		Authentication: &Authentication{
			ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
				prompts = append(prompts, prompt)
				if prompt == ownerinput.PIN {
					return "12345", nil
				}
				if prompt == ownerinput.OTP {
					return "654321", nil
				}
				t.Fatal("read renewal requested primary credentials")
				return "", nil
			},
			NewPINFromBundle: func(b sber.SessionBundle, options sber.AuthOptions) (PINAuthenticator, error) {
				if len(b.Cookies) != 1 || b.Cookies[0].Value != "synthetic-current-cookie" || options.TransportOptions.CABundle != "" {
					t.Fatal("renewal discarded the current session or default trust")
				}
				return auth, nil
			},
		},
		OpenClientWithOptions: func(ctx context.Context, path string, options sber.ClientOptions, provider sber.PINProvider) (mcp.Client, error) {
			if path != "synthetic-selected" || options.TransportOptions.Timeout.String() != "45s" || options.AllowMutations || provider == nil || options.Renewal == nil {
				t.Fatal("read command lost renewal or enabled mutations")
			}
			current := syntheticLoginBundle()
			current.Cookies[0].Value = "synthetic-current-cookie"
			refreshed, err := options.Renewal(ctx, current, provider, options.AuthOptions)
			if err != nil || auth.closeCalls != 1 || len(refreshed.Cookies) == 0 || auth.otpCalls != 1 {
				t.Fatal("renewal did not finish cleanup before adoption")
			}
			return client, nil
		},
	})
	if code != 0 || len(prompts) != 2 || auth.loginCalls != 1 || client.Closes.Load() != 1 || strings.Contains(output.String()+diagnostics.String(), "12345") {
		t.Fatal("CLI renewal did not complete one hidden challenge sequence")
	}
}

func TestCLIExplicitNoRenewAndMCPNeverReadSecrets(t *testing.T) {
	for _, args := range [][]string{{"products", "--no-renew"}, {"mcp"}} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), append(args, "--profile", "synthetic-selected"), &output, &diagnostics, Options{
			Input: strings.NewReader(""),
			Authentication: &Authentication{ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
				t.Fatal("batch command requested a secret")
				return "", nil
			}},
			OpenClientWithOptions: func(_ context.Context, _ string, options sber.ClientOptions, provider sber.PINProvider) (mcp.Client, error) {
				if provider != nil || options.Renewal != nil || options.AllowMutations {
					t.Fatal("batch command enabled interactive renewal or mutations")
				}
				return &testutil.Client{}, nil
			},
		})
		if code != 0 {
			t.Fatalf("batch command failed: %d", code)
		}
	}
}
