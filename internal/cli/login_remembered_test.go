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

type syntheticRemembered struct {
	loginCalls, otpCalls, closeCalls int
	needsOTP                         bool
	failure                          error
	closeError                       error
}

func (a *syntheticRemembered) Login(_ context.Context, pin string, _ sber.CaptchaAnswer) (sber.SessionBundle, error) {
	a.loginCalls++
	if pin != "12345" {
		return sber.SessionBundle{}, errors.New("synthetic-private-wrong-pin")
	}
	if a.failure != nil {
		return sber.SessionBundle{}, a.failure
	}
	if a.needsOTP {
		return sber.SessionBundle{}, &sber.PinOTPRequired{}
	}
	return syntheticLoginBundle(), nil
}
func (a *syntheticRemembered) ConfirmOTP(_ context.Context, code string) (sber.SessionBundle, error) {
	a.otpCalls++
	if code != "654321" {
		return sber.SessionBundle{}, errors.New("synthetic-private-wrong-otp")
	}
	return syntheticLoginBundle(), nil
}
func (a *syntheticRemembered) Close() error { a.closeCalls++; return a.closeError }

func syntheticLoginBundle() sber.SessionBundle {
	b, _ := sber.NewSessionBundle(sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin, Cookies: []sber.CookieRecord{{Name: "fixture", Value: "synthetic", Domain: "online.sberbank.ru", Path: "/", Secure: true}}})
	return b
}

func TestRememberedLoginUsesHiddenPINAndOneOTPBeforePrivatePublication(t *testing.T) {
	for _, otp := range []bool{false, true} {
		t.Run(map[bool]string{false: "PIN", true: "PIN and OTP"}[otp], func(t *testing.T) {
			profile := filepath.Join(testPrivateDir(t), "new.json")
			auth := &syntheticRemembered{needsOTP: otp}
			var output, diagnostics bytes.Buffer
			var prompts []ownerinput.Prompt
			options := Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) {
					t.Fatal("remembered login requested primary credentials")
					return nil, nil
				},
				NewPIN: func(path string, o sber.AuthOptions) (PINAuthenticator, error) {
					if path != "selected-remembered.json" || o.TransportOptions.CABundle != "selected-ca.pem" {
						t.Fatal("explicit source or trust selection lost")
					}
					return auth, nil
				},
				ReadSecret: func(_ context.Context, p ownerinput.Prompt) (string, error) {
					prompts = append(prompts, p)
					switch p {
					case ownerinput.PIN:
						return "12345", nil
					case ownerinput.OTP:
						return "654321", nil
					}
					t.Fatal("unexpected credential prompt")
					return "", nil
				},
			}}
			if code := RunWithOptions(context.Background(), []string{"login", "--profile", profile, "--remembered-profile", "selected-remembered.json", "--ca-bundle", "selected-ca.pem"}, &output, &diagnostics, options); code != 0 {
				t.Fatalf("remembered login failed: %d", code)
			}
			if auth.loginCalls != 1 || auth.closeCalls != 1 || auth.otpCalls != map[bool]int{false: 0, true: 1}[otp] || len(prompts) != 1+auth.otpCalls || prompts[0] != ownerinput.PIN {
				t.Fatal("PIN/OTP flow replayed or cleanup lost")
			}
			if _, err := sber.LoadSessionBundle(profile); err != nil {
				t.Fatal("private published profile cannot reopen")
			}
			before, _ := os.ReadFile(profile)
			output.Reset()
			if code := RunWithOptions(context.Background(), []string{"login", "--profile", profile, "--remembered-profile", "selected-remembered.json"}, &output, &diagnostics, options); code != 3 {
				t.Fatal("existing profile was replaced")
			}
			after, _ := os.ReadFile(profile)
			if !bytes.Equal(before, after) || auth.loginCalls != 1 || output.Len() != 0 {
				t.Fatal("existing destination started login or changed")
			}
		})
	}
}

func TestRememberedLoginFailureDoesNotPublishOrExposeRemoteMessages(t *testing.T) {
	auth := &syntheticRemembered{failure: errors.New("synthetic-private-remote-response")}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "new.json"), "--remembered-profile", "selected.json"}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPIN:     func(string, sber.AuthOptions) (PINAuthenticator, error) { return auth, nil },
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "12345", nil },
	}})
	if code != 3 || output.Len() != 0 || strings.Contains(diagnostics.String(), "synthetic-private") || auth.loginCalls != 1 || auth.closeCalls != 1 {
		t.Fatal("PIN failure was replayed, published or exposed")
	}
}

func TestRememberedLoginCleanupFailurePreventsPublication(t *testing.T) {
	profile := filepath.Join(testPrivateDir(t), "new.json")
	auth := &syntheticRemembered{closeError: errors.New("synthetic-private-cleanup")}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", profile, "--remembered-profile", "selected.json"}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPIN:     func(string, sber.AuthOptions) (PINAuthenticator, error) { return auth, nil },
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "12345", nil },
	}})
	if code != 3 || output.Len() != 0 || strings.Contains(diagnostics.String(), "synthetic-private") || auth.closeCalls != 1 {
		t.Fatal("cleanup failure exposed data or reached publication")
	}
	if _, err := os.Lstat(profile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cleanup failure published a profile")
	}
}

func TestRememberedLoginDefaultFactoryRejectsMissingSourceBeforePINInput(t *testing.T) {
	dir := testPrivateDir(t)
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(dir, "new.json"), "--remembered-profile", filepath.Join(dir, "missing.json")}, &output, &diagnostics, Options{Authentication: &Authentication{
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
			t.Fatal("missing source requested PIN")
			return "", nil
		},
	}})
	if code != 3 || output.Len() != 0 {
		t.Fatal("missing remembered source accepted")
	}
}

func TestLoginBrowserSelectionRejectsPartialOrRelativePathsBeforeEnrollment(t *testing.T) {
	for _, args := range [][]string{{"--browser-profile", "/explicit"}, {"--browser-profile", "relative", "--playwright-driver", "/driver", "--firefox-executable", "/firefox"}, {"--playwright-driver", "/driver", "--firefox-executable", "/firefox"}} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), append([]string{"login", "--profile", "explicit-new.json"}, args...), &output, &diagnostics, Options{Authentication: &Authentication{
			Enroll: func(context.Context, string, enrollment.Prepare) error {
				t.Fatal("invalid browser selection reached enrollment")
				return nil
			},
		}})
		if code != 2 || output.Len() != 0 {
			t.Fatal("invalid browser arguments accepted")
		}
	}
}

func TestRememberedLoginForwardsExplicitBrowserRenderingWithoutSecretInputToBrowser(t *testing.T) {
	dir := testPrivateDir(t)
	var output, diagnostics bytes.Buffer
	auth := &syntheticRemembered{}
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(dir, "new.json"), "--remembered-profile", "selected.json", "--browser-profile", dir, "--playwright-driver", "/explicit/driver", "--firefox-executable", "/explicit/firefox"}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPIN: func(_ string, o sber.AuthOptions) (PINAuthenticator, error) {
			if !o.BrowserFirst || o.BrowserBootstrap == nil {
				t.Fatal("explicit browser initialization lost")
			}
			return auth, nil
		},
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "12345", nil },
	}})
	if code != 0 || auth.loginCalls != 1 {
		t.Fatal("explicit remembered browser login failed")
	}
}
