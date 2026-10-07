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
		{"HTTP error", &sber.PinAuthError{Code: "bootstrap_failed", StatusCode: 403}, "bank login page returned HTTP 403"},
		{"session navigation", &sber.PinAuthError{Code: "redirect_failed", StatusCode: 500}, "bank session navigation returned HTTP 500"},
		{"config", &sber.PinAuthError{Code: "invalid_frontend_config"}, "bank login page configuration is not supported"},
		{"browser", &sber.PinAuthError{Code: "browser_check_required"}, "bank login page requires browser initialization"},
		{"bank PIN policy", &sber.PinAuthError{Code: "invalid_pin_birthdate", StatusCode: 400}, "bank did not accept the new PIN"},
		{"device limit", &sber.PinAuthError{Code: "browser_limit", StatusCode: 400}, "bank remembered-device limit reached"},
		{"PIN decode", &sber.PinAuthError{Code: "invalid_decode_pin", StatusCode: 400}, "bank could not decode the PIN"},
		{"session pair", &sber.PinAuthError{Code: "missing_ufs_session"}, "authenticated session cookies are missing"},
		{"unknown HTTP response", &sber.PinAuthError{Code: "synthetic-private-code", StatusCode: 500}, "bank authentication returned HTTP 500"},
		{"private metadata", &sber.PinAuthError{Code: "synthetic-private-code", Message: "synthetic-private-message"}, "owner login failed; profile not published"},
		{"private raw error", errors.New("synthetic-private-error"), "owner login failed; profile not published"},
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
