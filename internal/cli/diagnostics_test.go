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
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

type rejectedPrimary struct {
	syntheticPrimary
	failure error
}

func TestOwnerLoginRejectsInvalidCABundleBeforeSecretInput(t *testing.T) {
	path := filepath.Join(testPrivateDir(t), "untrusted.pem")
	if err := os.WriteFile(path, []byte("synthetic-private-invalid-certificate"), 0600); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", "selected-synthetic", "--ca-bundle", path}, &output, &diagnostics, Options{Authentication: &Authentication{
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
			t.Fatal("invalid trust configuration requested credentials")
			return "", nil
		},
		Enroll: func(ctx context.Context, _ string, prepare enrollment.Prepare) error {
			if _, err := prepare(ctx); err != nil {
				return enrollment.ErrPrepare
			}
			t.Fatal("invalid trust configuration reached publication")
			return nil
		},
	}})
	if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), "cannot load trusted PEM certificates") || strings.Contains(diagnostics.String(), "synthetic-private") {
		t.Fatal("invalid CA failed unsafely")
	}
}

func (a *rejectedPrimary) Login(context.Context, string, string, sber.PrimaryLoginOptions) (*sber.SessionBundle, error) {
	return nil, a.failure
}

func TestOwnerLoginReportsSafeFailureReasons(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want string
	}{
		{"untrusted TLS", &sber.TransportError{Code: "tls_untrusted"}, "TLS certificate is not trusted"},
		{"invalid CA", &sber.TransportError{Code: "invalid_ca_bundle"}, "cannot load trusted PEM certificates"},
		{"timeout", &sber.TransportError{Code: "timeout"}, "bank login request timed out"},
		{"connection failure", &sber.TransportError{Code: "request_failed"}, "bank login request failed"},
		{"TLS closure through proxy", &sber.TransportError{Code: "proxy_tls_closed"}, "TLS connection through the proxy closed before the bank request"},
		{"cookie format", &sber.TransportError{Code: "unsupported_cookie_metadata"}, "bank cookie attributes are not supported"},
		{"response encoding", &sber.TransportError{Code: "invalid_encoding"}, "bank response encoding is not supported"},
		{"challenge", &sber.PinAuthError{Code: "invalid_srp_challenge"}, "bank authentication challenge is not supported"},
		{"session format", sber.NewParseError("synthetic-private-field"), "authentication session format is not supported"},
		{"HTTP error", &sber.PinAuthError{Code: "bootstrap_failed", StatusCode: 403}, "bank login page returned HTTP 403"},
		{"session navigation", &sber.PinAuthError{Code: "redirect_failed", StatusCode: 500}, "bank session navigation returned HTTP 500"},
		{"config", &sber.PinAuthError{Code: "invalid_frontend_config"}, "bank login page configuration is not supported"},
		{"rejected public page", &sber.PinAuthError{Code: "login_page_rejected", StatusCode: 200, Message: "synthetic-private-support-id"}, "bank rejected the login page for this connection"},
		{"browser", &sber.PinAuthError{Code: "browser_check_required"}, "bank requires an interactive browser security check"},
		{"bank PIN policy", &sber.PinAuthError{Code: "invalid_pin_birthdate", StatusCode: 400}, "bank did not accept the new PIN"},
		{"device limit", &sber.PinAuthError{Code: "browser_limit", StatusCode: 400}, "bank limit for remembered devices was reached"},
		{"PIN decode", &sber.PinAuthError{Code: "invalid_decode_pin", StatusCode: 400}, "bank could not decode the PIN"},
		{"session pair", &sber.PinAuthError{Code: "missing_ufs_session"}, "authenticated session cookies are missing"},
		{"unknown HTTP response", &sber.PinAuthError{Code: "synthetic-private-code", StatusCode: 500}, "bank authentication returned HTTP 500"},
		{"private metadata", &sber.PinAuthError{Code: "synthetic-private-code", Message: "synthetic-private-message"}, "The owner login failed.\nThe command did not publish the profile."},
		{"private raw error", errors.New("synthetic-private-error"), "The owner login failed.\nThe command did not publish the profile."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), []string{"login", "--profile", "selected-synthetic"}, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) { return &rejectedPrimary{failure: tt.err}, nil },
				ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "synthetic-private-secret", nil },
				Enroll: func(ctx context.Context, _ string, prepare enrollment.Prepare) error {
					if _, err := prepare(ctx); err != nil {
						return enrollment.ErrPrepare
					}
					t.Fatal("failed login reached publication")
					return nil
				},
			}})
			if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), tt.want) || strings.Contains(diagnostics.String(), "synthetic-private") {
				t.Fatal("login failure lost safe diagnosis or exposed private data")
			}
		})
	}
}

func TestRememberedAuthenticationFailureIdentifiesItsPhaseWithoutPrivateData(t *testing.T) {
	for _, command := range []string{"login", "refresh-session"} {
		t.Run(command, func(t *testing.T) {
			profile := filepath.Join(testPrivateDir(t), "profile.json")
			if command == "refresh-session" {
				if err := syntheticLoginBundle().Save(profile); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{command, "--profile", profile}
			if command == "login" {
				args = append(args, "--remembered-profile", "synthetic-private-source")
			}
			auth := &syntheticRemembered{failure: &sber.TransportError{Code: "request_failed"}}
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), args, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPIN:     func(string, sber.AuthOptions) (PINAuthenticator, error) { return auth, nil },
				ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "12345", nil },
			}})
			if code != 3 || output.Len() != 0 || auth.loginCalls != 1 || auth.closeCalls != 1 {
				t.Fatal("failed authentication omitted cleanup, retried, or published success")
			}
			if !strings.Contains(diagnostics.String(), "stage=pin-login") || !strings.Contains(diagnostics.String(), "bank login request failed") {
				t.Fatal("remembered authentication lost its phase or safe error classification")
			}
			if strings.Contains(diagnostics.String(), "synthetic-private") || strings.Contains(diagnostics.String(), "12345") {
				t.Fatal("authentication diagnostics exposed private data")
			}
		})
	}
}

type failedPINEnrollment struct{ syntheticPrimary }

func (*failedPINEnrollment) CreatePIN(context.Context, string) (sber.SessionBundle, error) {
	return sber.SessionBundle{}, errors.New("synthetic-private-remote-body")
}

func TestPrimaryFailureReportsLocalPhaseWithoutRemoteMetadata(t *testing.T) {
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "new.json")}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPrimary: func() (PrimaryAuthenticator, error) { return &failedPINEnrollment{}, nil },
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "synthetic-private-input", nil },
	}})
	if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), "stage=pin-enrollment") || strings.Contains(diagnostics.String(), "synthetic-private") {
		t.Fatal("PIN enrollment failure lost its local phase or exposed remote data")
	}
}

func TestBankErrorClassificationsExcludeRemoteDetails(t *testing.T) {
	for _, tt := range []struct {
		err     error
		message string
	}{
		{&sber.APIRejected{Code: "synthetic-private", Text: "synthetic-private", UUID: "synthetic-private"}, "bank rejected the request"},
		{&sber.APIError{StatusCode: 500, Message: "synthetic-private"}, "HTTP 500"},
		{sber.NewParseError("synthetic-private"), "bank response format is not supported"},
	} {
		var output, diagnostics bytes.Buffer
		client := &testutil.Client{Read: func(context.Context, string, map[string]any) (map[string]any, error) { return nil, tt.err }}
		code := RunWithOptions(context.Background(), []string{"products", "--profile", "synthetic-selected"}, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
		if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), tt.message) || strings.Contains(diagnostics.String(), "synthetic-private") {
			t.Fatal("bank failure lost its safe classification")
		}
	}
}
