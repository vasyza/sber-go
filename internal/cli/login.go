package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
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
	ReadSecret func(context.Context, ownerinput.Prompt) (string, error)
	Enroll     func(context.Context, string, enrollment.Prepare) error
}

func defaultPrimary() (PrimaryAuthenticator, error) {
	device, err := sber.GenerateDeviceprint()
	if err != nil {
		return nil, err
	}
	antifraud, err := sber.GenerateAntifraudDeviceprint(device.Value())
	if err != nil {
		return nil, err
	}
	deviceValue, antifraudValue := device.Value(), antifraud.Value()
	return sber.NewPrimaryAuth(sber.AuthOptions{Deviceprint: &deviceValue, AntifraudDeviceprint: &antifraudValue})
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
	if flags.Parse(args) != nil || flags.NArg() != 0 || *profile == "" {
		return fail(diagnostics, 2, "usage: sber login --profile PATH; enter secrets only at the hidden terminal prompts")
	}
	a := Authentication{}
	if dependencies != nil {
		a = *dependencies
	}
	if a.NewPrimary == nil {
		a.NewPrimary = defaultPrimary
	}
	if a.ReadSecret == nil {
		a.ReadSecret = ownerSecret
	}
	if a.Enroll == nil {
		a.Enroll = enrollment.Enroll
	}
	message := "owner login failed; profile not published"
	err := a.Enroll(ctx, *profile, func(ctx context.Context) (enrollment.CandidateWriter, error) {
		writer, err := prepareLogin(ctx, a)
		var captcha *sber.PinCaptchaRequired
		if errors.As(err, &captcha) {
			message = "CAPTCHA requires owner interaction; use the bank UI or the explicit SDK challenge API"
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
			message = "hidden terminal enrollment is supported on Linux; use the SDK authentication API on macOS"
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

func prepareLogin(ctx context.Context, a Authentication) (writer enrollment.CandidateWriter, err error) {
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
		pin, readErr := a.ReadSecret(ctx, ownerinput.NewPIN)
		if readErr != nil {
			return nil, readErr
		}
		confirm, readErr := a.ReadSecret(ctx, ownerinput.ConfirmPIN)
		if readErr != nil {
			return nil, readErr
		}
		if pin == "" || pin != confirm {
			return nil, enrollment.ErrPrepare
		}
		created, createErr := auth.CreatePIN(ctx, pin)
		pin, confirm = "", ""
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
