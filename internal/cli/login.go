package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"reflect"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

// PrimaryAuthenticator is the owner login state machine, separate from bank
// business operations. Challenge responses are supplied once, never retried.
type PrimaryAuthenticator interface {
	Login(context.Context, string, string, sber.PrimaryLoginOptions) (*sber.SessionBundle, error)
	ConfirmOTP(context.Context, string) (*sber.SessionBundle, error)
	CreatePIN(context.Context, string) (sber.SessionBundle, error)
	Close() error
}

// Authentication injects synthetic dependencies into application tests. The
// command's production defaults accept no alternative credential input channel.
type Authentication struct {
	NewPrimary func() (PrimaryAuthenticator, error)
	NewPIN     func(string, sber.AuthOptions) (PINAuthenticator, error)
	ReadSecret func(context.Context, ownerinput.Prompt) (string, error)
	Enroll     func(context.Context, string, enrollment.Prepare) error
}

func primaryWithOptions(options sber.AuthOptions) (PrimaryAuthenticator, error) {
	device, err := sber.GenerateDeviceprint()
	if err != nil {
		return nil, err
	}
	antifraud, err := sber.GenerateAntifraudDeviceprint(device.Value())
	if err != nil {
		return nil, err
	}
	deviceValue, antifraudValue := device.Value(), antifraud.Value()
	options.Deviceprint, options.AntifraudDeviceprint = &deviceValue, &antifraudValue
	return sber.NewPrimaryAuth(options)
}

func ownerSecret(ctx context.Context, prompt ownerinput.Prompt) (string, error) {
	secret, err := ownerinput.ReadOwnerSecret(ctx, prompt)
	if err != nil {
		return "", err
	}
	defer secret.Clear()
	return string(secret.Bytes()), nil
}

func runLogin(ctx context.Context, args []string, output, diagnostics io.Writer, dependencies *Authentication) int {
	flags := flag.NewFlagSet("sber login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "", "explicit new private profile")
	caBundle := flags.String("ca-bundle", "", "explicit trusted PEM certificate bundle")
	remembered := flags.String("remembered-profile", "", "explicit private source for remembered PIN login")
	selection := loginBrowserSelection{}
	flags.StringVar(&selection.profile, "browser-profile", "", "dedicated private Firefox profile with verified NSS trust")
	flags.StringVar(&selection.driver, "playwright-driver", "", "absolute matching installed Playwright driver directory")
	flags.StringVar(&selection.executable, "firefox-executable", "", "absolute matching installed Firefox executable")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *profile == "" {
		return fail(diagnostics, 2, "usage: sber login --profile NEW_PATH [--remembered-profile PATH] [--ca-bundle PATH]; enter secrets only at the hidden terminal prompts")
	}
	if !selection.valid() {
		return fail(diagnostics, 2, "browser initialization requires all three absolute paths: --browser-profile, --playwright-driver and --firefox-executable")
	}
	a := Authentication{}
	if dependencies != nil {
		a = *dependencies
	}
	if a.NewPrimary == nil {
		a.NewPrimary = func() (PrimaryAuthenticator, error) {
			options, err := selection.authOptions(*caBundle)
			if err != nil {
				return nil, err
			}
			return primaryWithOptions(options)
		}
	}
	if a.NewPIN == nil {
		a.NewPIN = func(path string, options sber.AuthOptions) (PINAuthenticator, error) {
			return sber.NewPINAuthFromProfile(path, options)
		}
	}
	if a.ReadSecret == nil {
		a.ReadSecret = ownerSecret
	}
	if a.Enroll == nil {
		a.Enroll = enrollment.Enroll
	}
	message := "owner login failed; profile not published"
	err := a.Enroll(ctx, *profile, func(ctx context.Context) (enrollment.CandidateWriter, error) {
		var writer enrollment.CandidateWriter
		var err error
		if *remembered != "" {
			options, optionsErr := selection.authOptions(*caBundle)
			if optionsErr != nil {
				err = optionsErr
			} else {
				writer, err = prepareRememberedLogin(ctx, a, *remembered, options)
			}
		} else {
			writer, err = prepareLogin(ctx, a, diagnostics)
		}
		if err != nil {
			message = loginFailureMessage(err)
		}
		return writer, err
	})
	if err != nil {
		if ctx.Err() != nil {
			return 130
		}
		switch {
		case errors.Is(err, enrollment.ErrExists):
			message = "profile already exists; select a new path"
		case errors.Is(err, enrollment.ErrBusy):
			message = "another owner enrollment is active"
		case errors.Is(err, enrollment.ErrUnsupported):
			message = "hidden terminal enrollment requires Linux or macOS"
		case errors.Is(err, enrollment.ErrUnsafe):
			message = "unsafe private profile path"
		case errors.Is(err, enrollment.ErrPublish), errors.Is(err, enrollment.ErrCleanup):
			message = "profile publication could not be confirmed; inspect profile metadata before trying again"
		}
		return fail(diagnostics, 3, message)
	}
	if err := json.NewEncoder(output).Encode(map[string]any{"profile_created": true, "bank_authorization_checked": true, "mutations_enabled": false}); err != nil {
		return fail(diagnostics, 3, "profile created; output failed")
	}
	return 0
}

// Only known classifications become diagnostics. Remote messages, tokens,
// arbitrary error strings and unknown codes must never reach terminal output.
func loginFailureMessage(err error) string {
	var captcha *sber.PinCaptchaRequired
	if errors.As(err, &captcha) {
		return "CAPTCHA requires owner interaction; use the bank UI or the explicit SDK challenge API"
	}
	if message := transportFailureMessage(err, "bank login"); message != "" {
		return message + "; profile not published"
	}
	var auth *sber.PinAuthError
	if errors.As(err, &auth) {
		switch auth.Code {
		case "bootstrap_failed":
			if auth.StatusCode >= 100 && auth.StatusCode <= 599 {
				return fmt.Sprintf("bank login page returned HTTP %d; profile not published", auth.StatusCode)
			}
		case "invalid_frontend_config":
			return "bank login page configuration is not supported; profile not published"
		case "browser_check_required":
			return "bank login page requires browser initialization; profile not published"
		case "browser_bootstrap_unavailable":
			return "selected browser runtime is unavailable; check its explicit paths; profile not published"
		case "browser_bootstrap_failed", "browser_bootstrap_timeout", "unsupported_browser_state":
			return "selected browser initialization failed; check its private profile and verified certificate trust; profile not published"
		case "webauthn_required":
			return "bank requires owner WebAuthn interaction; profile not published"
		case "attempts_limit_reached":
			return "bank authentication attempt limit reached; profile not published"
		case "invalid_pin":
			return "online-banking PIN does not meet the bank's digit requirements; profile not published"
		case "invalid_pin_public_key":
			return "bank supplied an unsupported PIN encryption key; profile not published"
		case "redirect_failed":
			if auth.StatusCode >= 100 && auth.StatusCode <= 599 {
				return fmt.Sprintf("bank session navigation returned HTTP %d; profile not published", auth.StatusCode)
			}
		}
	}
	return "owner login failed; profile not published"
}

func transportFailureMessage(err error, request string) string {
	var wire *sber.TransportError
	if !errors.As(err, &wire) {
		return ""
	}
	switch wire.Code {
	case "invalid_ca_bundle":
		return "cannot load trusted PEM certificates; check --ca-bundle PATH"
	case "tls_untrusted":
		return "TLS certificate is not trusted; supply the bank CA using --ca-bundle PATH"
	case "tls_hostname":
		return "bank TLS certificate does not match the hostname"
	case "tls_expired":
		return "bank TLS certificate has expired or is not yet valid"
	case "tls_invalid":
		return "bank TLS certificate validation failed"
	case "timeout":
		return request + " request timed out"
	}
	return ""
}

func prepareLogin(ctx context.Context, a Authentication, diagnostics io.Writer) (writer enrollment.CandidateWriter, err error) {
	auth, err := a.NewPrimary()
	if err != nil {
		return nil, err
	}
	if auth == nil || (reflect.ValueOf(auth).Kind() == reflect.Pointer && reflect.ValueOf(auth).IsNil()) {
		return nil, enrollment.ErrPrepare
	}
	defer func() {
		if closeErr := auth.Close(); closeErr != nil {
			writer = nil
			err = enrollment.ErrPrepare
		}
	}()
	login, err := a.ReadSecret(ctx, ownerinput.Login)
	if err != nil {
		return nil, err
	}
	password, err := a.ReadSecret(ctx, ownerinput.Password)
	if err != nil {
		return nil, err
	}
	bundle, err := auth.Login(ctx, login, password, sber.PrimaryLoginOptions{})
	login, password = "", ""
	var otp *sber.PinOTPRequired
	if errors.As(err, &otp) {
		code, readErr := a.ReadSecret(ctx, ownerinput.OTP)
		if readErr != nil {
			return nil, readErr
		}
		bundle, err = auth.ConfirmOTP(ctx, code)
		code = ""
	}
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		pin, readErr := readNewPIN(ctx, a, auth, diagnostics)
		if readErr != nil {
			return nil, readErr
		}
		created, createErr := auth.CreatePIN(ctx, pin)
		pin = ""
		if createErr != nil {
			return nil, createErr
		}
		bundle = &created
	}
	validated, err := bundle.Clone()
	if err != nil {
		return nil, err
	}
	if _, err = validated.ToSeed(true); err != nil {
		return nil, err
	}
	return func(ctx context.Context, path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return sdkSession.WriteEnrollmentCandidate(path, validated)
	}, nil
}

// Current configuration is optional for injected authenticators. Native auth
// supplies it from its already loaded, validated configuration without another
// request. Correcting local input must not repeat login, OTP or PIN creation.
func readNewPIN(ctx context.Context, a Authentication, auth PrimaryAuthenticator, diagnostics io.Writer) (string, error) {
	length := 0
	if provider, ok := auth.(interface {
		LoadConfig(context.Context) (sber.FrontendConfig, error)
	}); ok {
		config, err := provider.LoadConfig(ctx)
		if err != nil {
			return "", err
		}
		length = config.PINLength()
		if length < 4 || length > 12 {
			return "", &sber.PinAuthError{Code: "invalid_frontend_config"}
		}
		if diagnostics != nil {
			if _, err := fmt.Fprintf(diagnostics, "New online-banking PIN requires %d digits.\n", length); err != nil {
				return "", enrollment.ErrPrepare
			}
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		pin, err := a.ReadSecret(ctx, ownerinput.NewPIN)
		if err != nil {
			return "", err
		}
		if length != 0 && !pinDigits(pin, length) {
			pin = ""
			if diagnostics != nil {
				if _, err := io.WriteString(diagnostics, "PIN must contain the requested number of digits; enter it again.\n"); err != nil {
					return "", enrollment.ErrPrepare
				}
			}
			continue
		}
		confirm, err := a.ReadSecret(ctx, ownerinput.ConfirmPIN)
		if err != nil {
			return "", err
		}
		if pin != "" && pin == confirm {
			confirm = ""
			return pin, nil
		}
		pin, confirm = "", ""
		if length == 0 {
			return "", enrollment.ErrPrepare
		}
		if diagnostics != nil {
			if _, err := io.WriteString(diagnostics, "PIN confirmation differs; enter and confirm the new PIN again.\n"); err != nil {
				return "", enrollment.ErrPrepare
			}
		}
	}
}

func pinDigits(pin string, length int) bool {
	if len(pin) != length {
		return false
	}
	for i := range pin {
		if pin[i] < '0' || pin[i] > '9' {
			return false
		}
	}
	return true
}
