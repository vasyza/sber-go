package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

type syntheticPrimary struct {
	steps      []string
	closeError error
}

func (a *syntheticPrimary) Login(context.Context, string, string, sber.PrimaryLoginOptions) (*sber.SessionBundle, error) {
	a.steps = append(a.steps, "login")
	return nil, &sber.PinOTPRequired{}
}
func (a *syntheticPrimary) ConfirmOTP(context.Context, string) (*sber.SessionBundle, error) {
	a.steps = append(a.steps, "otp")
	return nil, nil // The source protocol now requires enrollment of a PIN.
}
func (a *syntheticPrimary) CreatePIN(context.Context, string) (sber.SessionBundle, error) {
	a.steps = append(a.steps, "pin")
	return sber.NewSessionBundle(sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin, Cookies: []sber.CookieRecord{{Name: "fixture", Value: "synthetic-private-cookie", Domain: "online.sberbank.ru", Path: "/", Secure: true}}})
}
func (a *syntheticPrimary) Close() error {
	a.steps = append(a.steps, "close")
	return a.closeError
}

func TestOwnerLoginPublishesOnlyAfterOTPEnrollmentAndCleanup(t *testing.T) {
	profile := filepath.Join(testPrivateDir(t), "profile.json")
	auth := &syntheticPrimary{}
	var prompts []ownerinput.Prompt
	var out, diagnostic bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", profile}, &out, &diagnostic, Options{Authentication: &Authentication{
		NewPrimary: func() (PrimaryAuthenticator, error) { return auth, nil },
		ReadSecret: func(_ context.Context, p ownerinput.Prompt) (string, error) {
			prompts = append(prompts, p)
			return "synthetic-private-secret", nil
		},
		Enroll: func(ctx context.Context, path string, prepare enrollment.Prepare) error {
			if path != profile {
				t.Fatal("selected path lost")
			}
			writer, err := prepare(ctx)
			if err != nil {
				return err
			}
			if strings.Join(auth.steps, ",") != "login,otp,pin,close" {
				t.Fatal("profile publication preceded cleanup")
			}
			return writer(ctx, path)
		},
	}})
	if code != 0 {
		t.Fatalf("login failed: %d %s", code, diagnostic.String())
	}
	if len(prompts) != 5 || prompts[0] != ownerinput.Login || prompts[1] != ownerinput.Password || prompts[2] != ownerinput.OTP || prompts[3] != ownerinput.NewPIN || prompts[4] != ownerinput.ConfirmPIN {
		t.Fatal("owner challenge sequence lost")
	}
	var facts map[string]any
	if err := json.Unmarshal(out.Bytes(), &facts); err != nil {
		t.Fatal("creation status is not JSON")
	}
	if strings.Contains(out.String()+diagnostic.String(), "synthetic-private-secret") || facts["profile_created"] != true {
		t.Fatal("secret exposed or creation status lost")
	}
	if _, err := sber.LoadSessionBundle(profile); err != nil {
		t.Fatal("published profile cannot be reopened")
	}
}

func TestExistingProfileStopsBeforePromptsOrAuthentication(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", "explicit-synthetic"}, &out, &diagnostic, Options{Authentication: &Authentication{
		Enroll:     func(context.Context, string, enrollment.Prepare) error { return enrollment.ErrExists },
		NewPrimary: func() (PrimaryAuthenticator, error) { t.Fatal("existing profile triggered auth"); return nil, nil },
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
			t.Fatal("existing profile prompted secrets")
			return "", nil
		},
	}})
	if code != 3 || out.Len() != 0 {
		t.Fatal("existing profile treated as new enrollment")
	}
}

func TestOwnerLoginFailureNeverPublishesOrEchoesPrivateErrors(t *testing.T) {
	for _, mode := range []string{"mismatch", "input", "cleanup"} {
		t.Run(mode, func(t *testing.T) {
			auth := &syntheticPrimary{}
			if mode == "cleanup" {
				auth.closeError = errors.New("synthetic-private-error")
			}
			published := false
			var out, diagnostic bytes.Buffer
			code := RunWithOptions(context.Background(), []string{"login", "--profile", "explicit-synthetic"}, &out, &diagnostic, Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) { return auth, nil },
				ReadSecret: func(_ context.Context, p ownerinput.Prompt) (string, error) {
					if mode == "input" {
						return "", errors.New("synthetic-private-error")
					}
					if mode == "mismatch" && p == ownerinput.ConfirmPIN {
						return "different", nil
					}
					return "synthetic-private-secret", nil
				},
				Enroll: func(ctx context.Context, _ string, prepare enrollment.Prepare) error {
					writer, err := prepare(ctx)
					published = err == nil && writer != nil
					return err
				},
			}})
			if code != 3 || published || out.Len() != 0 || strings.Contains(diagnostic.String(), "synthetic-private") {
				t.Fatal("failed enrollment escaped boundary")
			}
		})
	}
}

func TestLoginRejectsSecretArgumentsBeforeEnrollment(t *testing.T) {
	var out, diagnostic bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", "explicit", "--password=synthetic-private-secret"}, &out, &diagnostic, Options{Authentication: &Authentication{
		Enroll: func(context.Context, string, enrollment.Prepare) error {
			t.Fatal("secret argument reached enrollment")
			return nil
		},
	}})
	if code != 2 || strings.Contains(diagnostic.String(), "synthetic-private-secret") {
		t.Fatal("secret argument accepted or echoed")
	}
}
