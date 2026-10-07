package cli

import (
	"context"
	"encoding/json"
	"errors"
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
	NewPrimary       func() (PrimaryAuthenticator, error)
	NewPIN           func(string, sber.AuthOptions) (PINAuthenticator, error)
	NewPINFromBundle func(sber.SessionBundle, sber.AuthOptions) (PINAuthenticator, error)
	ReadSecret       func(context.Context, ownerinput.Prompt) (string, error)
	Enroll           func(context.Context, string, enrollment.Prepare) error
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

func runLogin(ctx context.Context, args *commandArguments, output, diagnostics io.Writer, dependencies *Authentication) int {
	profile, caBundle, remembered, selection := args.profile, args.ca, args.remembered, args.browser
	a := Authentication{}
	if dependencies != nil {
		a = *dependencies
	}
	if a.NewPrimary == nil {
		a.NewPrimary = func() (PrimaryAuthenticator, error) {
			options, err := selection.authOptions(caBundle, args.selectedProxy)
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
	message := "The owner login failed.\nThe command did not publish the profile."
	err := a.Enroll(ctx, profile, func(ctx context.Context) (enrollment.CandidateWriter, error) {
		var writer enrollment.CandidateWriter
		var err error
		if remembered != "" {
			options, optionsErr := selection.authOptions(caBundle, args.selectedProxy)
			if optionsErr != nil {
				err = optionsErr
			} else {
				writer, err = prepareRememberedLogin(ctx, a, remembered, options)
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
			message = "The profile file already exists.\nSelect a new path."
			if args.defaultProfile {
				message = "The default profile already exists.\nUse sber refresh-session to restore the session."
			}
		case errors.Is(err, enrollment.ErrBusy):
			message = "Another owner enrollment is active."
		case errors.Is(err, enrollment.ErrUnsupported):
			message = "Hidden terminal enrollment requires Linux or macOS."
		case errors.Is(err, enrollment.ErrUnsafe):
			message = "The private profile path is not safe."
		case errors.Is(err, enrollment.ErrPublish), errors.Is(err, enrollment.ErrCleanup):
			message = "The command cannot confirm publication of the profile.\nRead the profile metadata before you try again."
		}
		return fail(diagnostics, 3, message)
	}
	if err := json.NewEncoder(output).Encode(map[string]any{"profile_created": true, "bank_authorization_checked": true, "mutations_enabled": false}); err != nil {
		return fail(diagnostics, 3, "The command created the profile.\nThe command cannot write the output.")
	}
	return 0
}

// Only known classifications become diagnostics. Remote messages, tokens,
// arbitrary error strings and unknown codes must never reach terminal output.
func loginFailureMessage(err error) (message string) {
	defer func() {
		var phase *loginPhaseError
		if errors.As(err, &phase) {
			switch phase.phase {
			case "public-configuration", "owner-input", "login-password", "pin-login", "sms-confirmation", "pin-enrollment", "session-validation", "cleanup":
				message = "Authentication stage=" + phase.phase + ".\n" + message
			}
		}
	}()
	var captcha *sber.PinCaptchaRequired
	if errors.As(err, &captcha) {
		return "CAPTCHA requires owner interaction.\nUse the bank website or the explicit SDK challenge API.\nThe command did not publish the profile."
	}
	if message := transportFailureMessage(err, "bank login"); message != "" {
		return message + "\nThe command did not publish the profile."
	}
	var parse *sber.ParseError
	if errors.As(err, &parse) {
		return "The authentication session format is not supported.\nThe command did not publish the profile."
	}
	var auth *sber.PinAuthError
	if errors.As(err, &auth) {
		switch auth.Code {
		case "invalid_srp_challenge":
			return "The bank authentication challenge is not supported.\nThe command did not publish the profile."
		case "bootstrap_failed":
			if auth.StatusCode >= 100 && auth.StatusCode <= 599 {
				return fmt.Sprintf("The bank login page returned HTTP %d.\nThe command did not publish the profile.", auth.StatusCode)
			}
		case "invalid_frontend_config":
			return "The bank login page configuration is not supported.\nThe command did not publish the profile."
		case "browser_check_required":
			return "The bank login page requires browser initialization.\nThe command did not publish the profile."
		case "browser_bootstrap_unavailable":
			return "The selected browser runtime is not available.\nCheck its explicit paths.\nThe command did not publish the profile."
		case "browser_bootstrap_failed", "browser_bootstrap_timeout", "unsupported_browser_state":
			return "The selected browser initialization failed.\nCheck its private profile and verified certificate trust.\nThe command did not publish the profile."
		case "webauthn_required":
			return "The bank requires owner WebAuthn interaction.\nThe command did not publish the profile."
		case "attempts_limit_reached":
			return "The bank authentication attempt limit was reached.\nThe command did not publish the profile."
		case "browser_limit":
			return "The bank limit for remembered devices was reached.\nUse the bank website.\nThe command did not publish the profile."
		case "invalid_pin_birthdate":
			return "The bank did not accept the new PIN.\nUse a different PIN that meets the bank requirements.\nThe command did not publish the profile."
		case "invalid_pin":
			return "The online banking PIN is not accepted.\nCheck the bank PIN requirements.\nThe command did not publish the profile."
		case "invalid_decode_pin":
			return "The bank could not decode the PIN.\nThe authentication protocol needs review.\nThe command did not publish the profile."
		case "invalid_pin_public_key":
			return "The bank supplied a PIN encryption key that is not supported.\nThe command did not publish the profile."
		case "redirect_failed":
			if auth.StatusCode >= 100 && auth.StatusCode <= 599 {
				return fmt.Sprintf("The bank session navigation returned HTTP %d.\nThe command did not publish the profile.", auth.StatusCode)
			}
		case "missing_ufs_session":
			return "The authenticated session cookies are missing.\nThe command did not publish the profile."
		case "invalid_json", "invalid_csrf", "missing_redirect", "missing_ufs_host", "missing_ufs_api_host", "invalid_primary_auth_state", "invalid_auth_token", "missing_auth_token", "invalid_otp_challenge", "pin_create_not_ready", "redirect_loop":
			return "The bank authentication response format is not supported.\nThe command did not publish the profile."
		case "ufs_ready_failed", "ufs_bootstrap_failed":
			if auth.StatusCode >= 100 && auth.StatusCode <= 599 {
				return fmt.Sprintf("The bank session initialization returned HTTP %d.\nThe command did not publish the profile.", auth.StatusCode)
			}
		case "invalid_server_proof":
			return "The bank authentication proof is not valid.\nThe command did not publish the profile."
		}
		if auth.StatusCode >= 100 && auth.StatusCode <= 599 {
			return fmt.Sprintf("The bank authentication returned HTTP %d.\nThe command did not publish the profile.", auth.StatusCode)
		}
	}
	return "The owner login failed.\nThe command did not publish the profile."
}

func transportFailureMessage(err error, request string) string {
	var wire *sber.TransportError
	if !errors.As(err, &wire) {
		return ""
	}
	switch wire.Code {
	case "invalid_proxy":
		return "The proxy address is not valid."
	case "proxy_authentication":
		return "The proxy rejected its login values."
	case "proxy_connect", "proxy_failed":
		return "The command cannot connect through the proxy."
	case "invalid_ca_bundle":
		return "The command cannot load trusted PEM certificates.\nCheck --ca-bundle PATH."
	case "tls_untrusted":
		return "The TLS certificate is not trusted.\nUpdate the application or select a verified CA with --ca-bundle PATH."
	case "tls_hostname":
		return "The TLS certificate does not match the hostname."
	case "tls_expired":
		return "The TLS certificate has expired or is not yet valid."
	case "tls_invalid":
		return "The TLS certificate validation failed."
	case "timeout":
		return "The " + request + " request timed out."
	case "request_failed":
		return "The " + request + " request failed.\nNo complete result is available."
	case "close_failed":
		return "The command cannot close the authentication session."
	case "unsupported_cookie_metadata":
		return "The bank cookie attributes are not supported."
	case "invalid_encoding", "unsupported_encoding":
		return "The bank response encoding is not supported."
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
	phase := "public-configuration"
	defer func() {
		if closeErr := auth.Close(); closeErr != nil {
			writer = nil
			err = enrollment.ErrPrepare
			phase = "cleanup"
		}
		if err != nil {
			err = &loginPhaseError{phase: phase, cause: err}
		}
	}()
	if err := prepareAuthentication(ctx, auth); err != nil {
		return nil, err
	}
	phase = "owner-input"
	login, err := a.ReadSecret(ctx, ownerinput.Login)
	if err != nil {
		return nil, err
	}
	password, err := a.ReadSecret(ctx, ownerinput.Password)
	if err != nil {
		return nil, err
	}
	phase = "login-password"
	bundle, err := auth.Login(ctx, login, password, sber.PrimaryLoginOptions{})
	login, password = "", ""
	var otp *sber.PinOTPRequired
	if errors.As(err, &otp) {
		phase = "owner-input"
		code, readErr := a.ReadSecret(ctx, ownerinput.OTP)
		if readErr != nil {
			return nil, readErr
		}
		phase = "sms-confirmation"
		bundle, err = auth.ConfirmOTP(ctx, code)
		code = ""
	}
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		phase = "pin-enrollment"
		created, createErr := enrollOwnerPIN(ctx, a, auth, diagnostics)
		if createErr != nil {
			return nil, createErr
		}
		bundle = &created
	}
	phase = "session-validation"
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
			if _, err := fmt.Fprintf(diagnostics, "The new online banking PIN requires %d digits.\n", length); err != nil {
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
				if _, err := io.WriteString(diagnostics, "The PIN must contain the specified number of digits.\nEnter the PIN again.\n"); err != nil {
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
			if _, err := io.WriteString(diagnostics, "The PIN confirmation differs.\nEnter the new PIN again.\nConfirm the new PIN again.\n"); err != nil {
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
