package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

var syntheticEnvironmentCredentials = map[string]string{
	"SBER_LOGIN":       "synthetic-env-login",
	"SBER_PASSWORD":    "synthetic-env-password # literal",
	"SBER_PINCODE":     "12345",
	"SBER_PHONE":       "79000000001",
	"SBER_CARD_NUMBER": "1111222233334444",
}

func credentialTestSource(t *testing.T, source string) []string {
	t.Helper()
	for name, value := range syntheticEnvironmentCredentials {
		t.Setenv(name, value)
		if source == "file" {
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	if source == "environment" {
		return nil
	}
	path := filepath.Join(testPrivateDir(t), "credentials.env")
	var raw strings.Builder
	raw.WriteString("# Synthetic credentials only.\n")
	for name, value := range syntheticEnvironmentCredentials {
		fmt.Fprintf(&raw, "export %s='%s' # literal value\n", name, value)
	}
	if err := os.WriteFile(path, []byte(raw.String()), 0600); err != nil {
		t.Fatal(err)
	}
	return []string{"--env-file", path}
}

type environmentLogin struct {
	t                               *testing.T
	method                          string
	needsOTP                        bool
	createError                     error
	logins, otps, creations, closes int
}

func (a *environmentLogin) LoadConfig(context.Context) (sber.FrontendConfig, error) {
	return sber.NewFrontendConfig(sber.AppOrigin, "synthetic-process", 5, "", "", true, false), nil
}

func (a *environmentLogin) Login(_ context.Context, login, password string, _ sber.PrimaryLoginOptions) (*sber.SessionBundle, error) {
	a.logins++
	name := "SBER_LOGIN"
	switch a.method {
	case "phone":
		name = "SBER_PHONE"
	case "card":
		name = "SBER_CARD_NUMBER"
	}
	if login != syntheticEnvironmentCredentials[name] || a.method != "card" && password != syntheticEnvironmentCredentials["SBER_PASSWORD"] {
		a.t.Fatal("authentication did not use the selected environment credentials")
	}
	if a.needsOTP {
		return nil, &sber.PinOTPRequired{}
	}
	return nil, nil // PIN enrollment follows authentication.
}

func (a *environmentLogin) ConfirmOTP(_ context.Context, code string) (*sber.SessionBundle, error) {
	a.otps++
	if code != "654321" {
		a.t.Fatal("authentication lost the terminal SMS code")
	}
	return nil, nil
}

func (a *environmentLogin) CreatePIN(_ context.Context, pin string) (sber.SessionBundle, error) {
	a.creations++
	if pin != syntheticEnvironmentCredentials["SBER_PINCODE"] {
		a.t.Fatal("PIN enrollment did not use the configured PIN")
	}
	return syntheticLoginBundle(), a.createError
}

func (a *environmentLogin) Close() error { a.closes++; return nil }

type environmentCard struct{ *environmentLogin }

func (a *environmentCard) Login(ctx context.Context, pan string, _ sber.CaptchaAnswer) (*sber.SessionBundle, error) {
	return a.environmentLogin.Login(ctx, pan, "", sber.PrimaryLoginOptions{})
}

func TestAuthenticationEnvironmentCredentialsAcrossMethods(t *testing.T) {
	for _, source := range []string{"environment", "file"} {
		for _, method := range []string{"login", "phone", "card", "remembered", "refresh"} {
			t.Run(source+"/"+method, func(t *testing.T) {
				flags := credentialTestSource(t, source)
				profile := filepath.Join(testPrivateDir(t), "profile.json")
				args := []string{"login", "--profile", profile}
				primary := &environmentLogin{t: t, method: method, needsOTP: true}
				pin := &syntheticRemembered{}
				switch method {
				case "phone", "card":
					args = append(args, "--method", method)
				case "remembered":
					args = append(args, "--remembered-profile", "synthetic-source.json")
				case "refresh":
					args[0] = "refresh-session"
					if err := syntheticLoginBundle().Save(profile); err != nil {
						t.Fatal(err)
					}
				}
				terminalPrompts := 0
				var output, diagnostics bytes.Buffer
				code := RunWithOptions(context.Background(), append(args, flags...), &output, &diagnostics, Options{Authentication: &Authentication{
					NewPrimary: func() (PrimaryAuthenticator, error) { return primary, nil },
					NewPhone:   func() (CredentialAuthenticator, error) { return primary, nil },
					NewCard:    func() (CardAuthenticator, error) { return &environmentCard{primary}, nil },
					NewPIN:     func(string, sber.AuthOptions) (PINAuthenticator, error) { return pin, nil },
					ReadTerminalSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
						terminalPrompts++
						if prompt != ownerinput.OTP {
							t.Fatal("a configured credential triggered a terminal prompt")
						}
						return "654321", nil
					},
				}})
				if code != 0 {
					t.Fatalf("configured authentication failed: exit %d", code)
				}
				if method == "remembered" || method == "refresh" {
					if terminalPrompts != 0 || pin.loginCalls != 1 || pin.closeCalls != 1 || pin.otpCalls != 0 {
						t.Fatal("headless PIN authentication prompted, repeated login, or lost cleanup")
					}
				} else if terminalPrompts != 1 || primary.logins != 1 || primary.otps != 1 || primary.creations != 1 || primary.closes != 1 {
					t.Fatal("environment authentication did not request only one terminal SMS code")
				}
				if _, err := sber.LoadSessionBundle(profile); err != nil {
					t.Fatal("configured authentication did not publish a valid private profile")
				}
				for _, value := range syntheticEnvironmentCredentials {
					if strings.Contains(output.String()+diagnostics.String(), value) {
						t.Fatal("configured authentication disclosed a credential")
					}
				}
			})
		}
	}
}

func TestEnvironmentPINDoesNotRepeatInvalidEnrollment(t *testing.T) {
	for _, scenario := range []string{"wrong length", "bank rejection"} {
		t.Run(scenario, func(t *testing.T) {
			credentialTestSource(t, "environment")
			primary := &environmentLogin{t: t, method: "login"}
			wantCreations := 0
			if scenario == "wrong length" {
				t.Setenv("SBER_PINCODE", "1234")
			} else {
				primary.createError = &sber.PinAuthError{Code: "invalid_pin", StatusCode: 400}
				wantCreations = 1
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			profile := filepath.Join(testPrivateDir(t), "new.json")
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(ctx, []string{"login", "--profile", profile}, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) { return primary, nil },
				ReadTerminalSecret: func(context.Context, ownerinput.Prompt) (string, error) {
					t.Fatal("a fixed PIN failure requested terminal input")
					return "", nil
				},
			}})
			if code != 3 || ctx.Err() != nil || primary.creations != wantCreations || primary.closes != 1 || output.Len() != 0 {
				t.Fatal("configured PIN enrollment looped, retried, or returned success")
			}
			if _, err := os.Lstat(profile); !os.IsNotExist(err) {
				t.Fatal("failed PIN enrollment published a profile")
			}
		})
	}
}

func TestEnvironmentRefreshSMSFailurePreservesProfile(t *testing.T) {
	for _, source := range []string{"environment", "file"} {
		t.Run(source, func(t *testing.T) {
			flags := credentialTestSource(t, source)
			profile := filepath.Join(testPrivateDir(t), "profile.json")
			if err := syntheticLoginBundle().Save(profile); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(profile)
			pin := &syntheticRemembered{needsOTP: true}
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), append([]string{"refresh-session", "--profile", profile}, flags...), &output, &diagnostics, Options{Authentication: &Authentication{
				NewPIN: func(string, sber.AuthOptions) (PINAuthenticator, error) { return pin, nil },
				ReadTerminalSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
					if prompt != ownerinput.OTP {
						t.Fatal("configured PIN was requested through the terminal")
					}
					return "", ownerinput.ErrTerminal
				},
			}})
			after, _ := os.ReadFile(profile)
			if code != 3 || output.Len() != 0 || !bytes.Equal(before, after) || pin.loginCalls != 1 || pin.otpCalls != 0 || pin.closeCalls != 1 {
				t.Fatal("headless SMS challenge replayed authentication or changed the profile")
			}
			if !strings.Contains(diagnostics.String(), "SMS code") || !strings.Contains(diagnostics.String(), "terminal") {
				t.Fatal("headless SMS failure did not explain the required owner input")
			}
		})
	}
}

func TestEnvironmentPINEnablesBatchReadRenewal(t *testing.T) {
	for _, source := range []string{"environment", "file"} {
		t.Run(source, func(t *testing.T) {
			flags := credentialTestSource(t, source)
			pin := &syntheticRemembered{}
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), append([]string{"products", "--profile", "synthetic-selected"}, flags...), &output, &diagnostics, Options{
				Authentication: &Authentication{NewPINFromBundle: func(sber.SessionBundle, sber.AuthOptions) (PINAuthenticator, error) { return pin, nil }},
				OpenClientWithOptions: func(ctx context.Context, _ string, options sber.ClientOptions, provider sber.PINProvider) (mcp.Client, error) {
					if provider == nil || options.Renewal == nil || options.AllowMutations {
						t.Fatal("configured PIN did not enable read-only renewal without a terminal")
					}
					if _, err := options.Renewal(ctx, syntheticLoginBundle(), provider, options.AuthOptions); err != nil {
						t.Fatal("configured read renewal failed")
					}
					return &testutil.Client{}, nil
				},
			})
			if code != 0 || pin.loginCalls != 1 || pin.closeCalls != 1 {
				t.Fatal("configured batch renewal failed")
			}
		})
	}
}

func TestCredentialFileFailureStopsBeforeAuthentication(t *testing.T) {
	for _, scenario := range []string{"missing", "permissions", "parent permissions", "symlink", "hard link", "directory", "oversized", "syntax"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(testPrivateDir(t), "credentials.env")
			if scenario != "missing" {
				if err := os.WriteFile(path, []byte("SBER_PINCODE=12345\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "permissions":
				os.Chmod(path, 0644)
			case "parent permissions":
				os.Chmod(filepath.Dir(path), 0755)
			case "symlink":
				os.Rename(path, path+".target")
				os.Symlink(path+".target", path)
			case "hard link":
				os.Link(path, path+".alias")
			case "directory":
				os.Remove(path)
				os.Mkdir(path, 0700)
			case "oversized":
				os.WriteFile(path, bytes.Repeat([]byte("x"), 65537), 0600)
			case "syntax":
				os.WriteFile(path, []byte("synthetic-private-invalid-file\n"), 0600)
			}
			var output, diagnostics bytes.Buffer
			code := RunWithOptions(context.Background(), []string{"login", "--profile", filepath.Join(testPrivateDir(t), "new.json"), "--env-file", path}, &output, &diagnostics, Options{Authentication: &Authentication{
				NewPrimary: func() (PrimaryAuthenticator, error) {
					t.Fatal("invalid credential file started authentication")
					return nil, errors.New("synthetic-unused")
				},
			}})
			if code != 3 || output.Len() != 0 || !strings.Contains(diagnostics.String(), "credential file") || strings.Contains(diagnostics.String(), path) || strings.Contains(diagnostics.String(), "synthetic-private") {
				t.Fatal("credential file failure was accepted or disclosed private state")
			}
		})
	}
}

func TestConfiguredPINHonorsRenewalExclusions(t *testing.T) {
	for _, source := range []string{"environment", "file"} {
		flags := credentialTestSource(t, source)
		for _, command := range []string{"products", "mcp", "export-session", "inspect-credentials", "card-rename", "transfer-own"} {
			args := &commandArguments{noRenew: command == "products"}
			if len(flags) != 0 {
				args.envFile = flags[1]
			}
			options, provider, err := readClientOptions(command, args, nil)
			if err != nil || provider != nil || options.Renewal != nil {
				t.Fatal("configured PIN enabled restoration for an excluded command")
			}
		}
	}
}

type expiredCredentialTransport struct {
	t      *testing.T
	jar    *sber.CookieJar
	reads  int
	closes int
}

func (tr *expiredCredentialTransport) Get(context.Context, string, sber.RequestOptions) (*sber.Response, error) {
	tr.reads++
	return &sber.Response{StatusCode: http.StatusUnauthorized, Headers: http.Header{}, Content: []byte(`{}`)}, nil
}

func (tr *expiredCredentialTransport) Post(context.Context, string, map[string]any, sber.RequestOptions) (*sber.Response, error) {
	tr.reads++
	return &sber.Response{StatusCode: http.StatusUnauthorized, Headers: http.Header{}, Content: []byte(`{}`)}, nil
}

func (tr *expiredCredentialTransport) PostForm(context.Context, string, map[string]string, sber.RequestOptions) (*sber.Response, error) {
	tr.t.Fatal("renewal made an unexpected credential POST")
	return nil, errors.New("synthetic-unused")
}

func (tr *expiredCredentialTransport) CookieJar() *sber.CookieJar { return tr.jar }
func (tr *expiredCredentialTransport) Close() error               { tr.closes++; return nil }

func TestEnvironmentSDKReadRenewalPreservesSMSClassification(t *testing.T) {
	credentialTestSource(t, "environment")
	dp, af := "synthetic-device", "synthetic-antifraud"
	bundle, err := (sber.SberCredentials{UFSSession: "synthetic-session", UFSToken: "synthetic-token"}).ToBundle(sber.CredentialsBundleOptions{
		APIBase: sber.AppOrigin, WebBase: sber.AppOrigin, Deviceprint: &dp, AntifraudDeviceprint: &af,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(testPrivateDir(t), "profile.json")
	if err := bundle.Save(profile); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(profile)
	jar, err := sber.NewCookieJar(bundle.Cookies)
	if err != nil {
		t.Fatal(err)
	}
	transport := &expiredCredentialTransport{t: t, jar: jar}
	pin := &syntheticRemembered{needsOTP: true}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"products", "--profile", profile}, &output, &diagnostics, Options{
		Authentication: &Authentication{
			NewPINFromBundle: func(sber.SessionBundle, sber.AuthOptions) (PINAuthenticator, error) { return pin, nil },
			ReadTerminalSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
				if prompt != ownerinput.OTP {
					t.Fatal("configured PIN prompted during SDK renewal")
				}
				return "", ownerinput.ErrTerminal
			},
		},
		OpenClientWithOptions: func(ctx context.Context, path string, options sber.ClientOptions, provider sber.PINProvider) (mcp.Client, error) {
			options.Transport = transport
			return sber.NewSberClientFromPINProfile(ctx, path, provider, options)
		},
	})
	after, _ := os.ReadFile(profile)
	if code != 3 || output.Len() != 0 || !bytes.Equal(before, after) || pin.loginCalls != 1 || pin.otpCalls != 0 || pin.closeCalls != 1 || transport.reads != 1 || transport.closes != 1 {
		t.Fatal("SDK SMS renewal repeated a request, lost cleanup, or changed the profile")
	}
	if !strings.Contains(diagnostics.String(), "SMS code") || !strings.Contains(diagnostics.String(), "terminal") {
		t.Fatal("SDK redaction removed the required SMS instruction")
	}
}
