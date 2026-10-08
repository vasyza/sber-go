package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestRememberedLoginForwardsNativeProxyOptions(t *testing.T) {
	dir := testPrivateDir(t)
	var output, diagnostics bytes.Buffer
	auth := &syntheticRemembered{}
	code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(dir, "new.json"), "--remembered-profile", "selected.json", "--proxy", "socks5://127.0.0.1:1080:u:p"}, &output, &diagnostics, Options{Authentication: &Authentication{
		NewPIN: func(_ string, o sber.AuthOptions) (PINAuthenticator, error) {
			if o.TransportOptions.Proxy != (sber.ProxyOptions{URL: "socks5://127.0.0.1:1080", Username: "u", Password: "p"}) {
				t.Fatal("native proxy options lost")
			}
			return auth, nil
		},
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "12345", nil },
	}})
	if code != 0 || auth.loginCalls != 1 {
		t.Fatal("native remembered login failed")
	}
}

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
