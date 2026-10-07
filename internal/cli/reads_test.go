package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestNativeReadCommandsUseSelectedClientAndClose(t *testing.T) {
	for _, command := range []string{"products", "accounts", "cards", "operations", "operations-page", "check-session"} {
		t.Run(command, func(t *testing.T) {
			client := &testutil.Client{}
			var out, diagnostics bytes.Buffer
			opened := 0
			code := RunWithOptions(context.Background(), []string{command, "--profile", "explicit-synthetic-path"}, &out, &diagnostics, Options{OpenClient: func(path string) (mcp.Client, error) {
				opened++
				if path != "explicit-synthetic-path" {
					t.Fatal("selected profile lost")
				}
				return client, nil
			}})
			if code != 0 || opened != 1 || client.Closes.Load() != 1 {
				t.Fatalf("command failed or session leaked: %d %s", code, diagnostics.String())
			}
			if strings.Contains(out.String(), "synthetic-private-cookie") || strings.Contains(out.String(), "4111111111111111") {
				t.Fatal("credential or PAN exposed")
			}
			if command == "products" || command == "accounts" {
				if !strings.Contains(out.String(), "9007199254740993.10") {
					t.Fatal("money rounded")
				}
			}
			if command == "operations" && !strings.Contains(out.String(), `"WindowCompleteness": "unknown"`) {
				t.Fatal("history lost unknown coverage")
			}
		})
	}
}

func TestLiveArgumentsFailBeforeSessionConstruction(t *testing.T) {
	for _, extra := range [][]string{{"--password=synthetic-private"}, {"--limit=0"}, {"--from=invalid"}, {"--profile=explicit", "trailing-secret"}} {
		var out, diagnostics bytes.Buffer
		args := append([]string{"products", "--profile", "explicit"}, extra...)
		code := RunWithOptions(context.Background(), args, &out, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { t.Fatal("invalid arguments opened a session"); return nil, nil }})
		if code != 2 || strings.Contains(diagnostics.String(), "synthetic-private") || out.Len() != 0 {
			t.Fatal("invalid input was accepted or echoed")
		}
	}
}

func TestBankFailureNeverReturnsSuccessfulPartialOutput(t *testing.T) {
	client := &testutil.Client{Read: func(context.Context, string, map[string]any) (map[string]any, error) {
		return nil, errors.New("synthetic-private-error")
	}}
	var out, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"operations", "--profile", "explicit"}, &out, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
	if code != 3 || out.Len() != 0 || client.Closes.Load() != 1 || strings.Contains(diagnostics.String(), "synthetic-private-error") {
		t.Fatal("failure escaped application boundary")
	}
}

func TestAdditionalReadCommandsRouteAndPreserveExactMoney(t *testing.T) {
	for _, tt := range []struct {
		args     []string
		path     string
		response string
		body     map[string]any
	}{
		{[]string{"card-info", "--card-id", "00123", "--card-id", "456"}, sber.CardInfoPath, `{"success":true,"body":{"cardDetails":{"cards":[{"id":"123","number":"4111111111111111","limits":{"availableLimit":{"amount":"9007199254740993.10","currency":"RUB"}}}]}}}`, map[string]any{"cardIds": []int64{123, 456}}},
		{[]string{"card-limits", "--card-id", "123"}, sber.CardInfoPath, `{"success":true,"body":{"cardDetails":{"cards":[{"id":"123","limits":{"availableLimit":{"amount":"9007199254740993.10","currency":"RUB"}}}]}}}`, map[string]any{"cardIds": []int64{123}}},
		{[]string{"operation-details", "--operation-id", "fixture-operation"}, sber.OperationDetailsPath, `{"success":true,"body":{"uohId":"fixture-operation","header":{"operationAmount":{"amount":"9007199254740993.10","currency":"RUB"}}}}`, map[string]any{"uohId": "fixture-operation"}},
		{[]string{"analytics", "--from", "2026-08-01", "--to", "2026-08-31", "--income-type", "income", "--between-own=false", "--open-banking"}, sber.PFMAmountsPath, `{"success":true,"body":{"amounts":[{"nationalAmount":{"amount":"9007199254740993.10","currency":"RUB"}}]}}`, nil},
		{[]string{"portfolio"}, sber.ProductsPath, "", nil},
	} {
		t.Run(tt.args[0], func(t *testing.T) {
			client := &testutil.Client{}
			if tt.response != "" {
				client.Read = func(_ context.Context, path string, body map[string]any) (map[string]any, error) {
					if path != tt.path || tt.body != nil && !reflect.DeepEqual(body, tt.body) {
						t.Fatal("CLI changed the endpoint or request contract")
					}
					if tt.args[0] == "analytics" {
						filter := body["filter"].(map[string]any)
						if filter["incomeType"] != "income" || filter["betweenOwnFilter"] != "off" || filter["openBankingFilter"] != "on" || filter["from"] != "2026-08-01T00:00:00+03:00" || filter["to"] != "2026-08-31T23:59:59+03:00" {
							t.Fatal("analytics filters changed")
						}
					}
					return sber.DecodeJSON(strings.NewReader(tt.response))
				}
			}
			var output, diagnostics bytes.Buffer
			args := append(append([]string{}, tt.args...), "--profile", "synthetic-selected")
			code := RunWithOptions(context.Background(), args, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
			if code != 0 || client.Closes.Load() != 1 || len(client.Requests()) != 1 || client.Requests()[0].Path != tt.path {
				t.Fatalf("read command failed: status=%d", code)
			}
			if !strings.Contains(output.String(), "9007199254740993.10") || strings.Contains(output.String(), "4111111111111111") || !json.Valid(output.Bytes()) {
				t.Fatal("read command lost exact data or exposed PAN")
			}
		})
	}
}

func TestReadCommandArgumentsFailBeforeOpeningProfile(t *testing.T) {
	for _, args := range [][]string{
		{"card-info"}, {"card-info", "--card-id", "-1"}, {"card-info", "--card-id", "9007199254740992"},
		{"card-limits", "--card-id", "123", "--card-id", "456"}, {"operation-details", "--operation-id", "bad/id"},
		{"analytics", "--from", "2026-08-01"}, {"analytics", "--from", "2026-08-01", "--to", "2026-08-31", "--income-type", "both"},
		{"products", "--offset", "10"}, {"accounts", "--from", "2026-08-01"}, {"operations-page", "--max-pages", "3"},
		{"operations", "--resource", "unsupported:123"}, {"operations-page", "--resource", "card:bad/id"},
		{"operations-page", "--offset", strconv.Itoa(int(^uint(0) >> 1))},
	} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), append(args, "--profile", "synthetic-selected"), &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { t.Fatal("invalid command opened the profile"); return nil, nil }})
		if code != 2 || output.Len() != 0 {
			t.Fatalf("invalid arguments accepted for %s", args[0])
		}
	}
}

func TestSessionExportUsesPrivateNoReplacePublication(t *testing.T) {
	destination := filepath.Join(testPrivateDir(t), "export.json")
	for _, want := range []int{0, 3} {
		client := &testutil.Client{}
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), []string{"export-session", "--profile", "synthetic-selected", "--destination", destination}, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
		if code != want || client.Closes.Load() != 1 || len(client.Requests()) != 0 || strings.Contains(output.String(), "synthetic-private-cookie") {
			t.Fatalf("private export boundary failed: status=%d", code)
		}
		b, err := sber.LoadSessionBundle(destination)
		if err != nil || len(b.Cookies) != 1 || b.Cookies[0].Value != "synthetic-private-cookie" {
			t.Fatal("private export cannot be reopened")
		}
		info, err := os.Stat(destination)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("export permissions are not private")
		}
	}
}

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
