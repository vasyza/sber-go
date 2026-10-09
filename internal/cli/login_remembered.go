package cli

import (
	"context"
	"errors"
	"reflect"
	"time"

	sber "github.com/vasyza/sber-sdk"
	"github.com/vasyza/sber-sdk/internal/enrollment"
	"github.com/vasyza/sber-sdk/internal/ownerinput"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
)

// PINAuthenticator authenticates one remembered device without changing its PIN.
type PINAuthenticator interface {
	Login(context.Context, string, sber.CaptchaAnswer) (sber.SessionBundle, error)
	ConfirmOTP(context.Context, string) (sber.SessionBundle, error)
	Close() error
}

func nativeAuthOptions(ca string, proxy sber.ProxyOptions) (sber.AuthOptions, error) {
	return sber.AuthOptions{TransportOptions: sber.TransportOptions{CABundle: ca, Timeout: 60 * time.Second, Proxy: proxy}}, nil
}

func prepareRememberedLogin(ctx context.Context, a Authentication, source string, options sber.AuthOptions) (writer enrollment.CandidateWriter, err error) {
	validated, err := authenticateRemembered(ctx, a.ReadSecret, func() (PINAuthenticator, error) {
		return a.NewPIN(source, options)
	})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return sdkSession.WriteEnrollmentCandidate(path, validated)
	}, nil
}

// Authentication and cleanup finish before callers can publish a session.
func authenticateRemembered(ctx context.Context, read func(context.Context, ownerinput.Prompt) (string, error), create func() (PINAuthenticator, error)) (validated sber.SessionBundle, err error) {
	phase := "public-configuration"
	defer func() {
		if err != nil {
			err = &loginPhaseError{phase: phase, cause: err}
		}
	}()
	auth, err := create()
	if err != nil {
		return validated, err
	}
	if auth == nil || reflect.ValueOf(auth).Kind() == reflect.Pointer && reflect.ValueOf(auth).IsNil() {
		return validated, enrollment.ErrPrepare
	}
	defer func() {
		if auth.Close() != nil {
			validated = sber.SessionBundle{}
			err = enrollment.ErrPrepare
			phase = "cleanup"
		}
	}()
	if err := prepareAuthentication(ctx, auth); err != nil {
		return validated, err
	}
	phase = "owner-input"
	pin, err := read(ctx, ownerinput.PIN)
	if err != nil {
		return validated, err
	}
	phase = "pin-login"
	bundle, err := auth.Login(ctx, pin, sber.CaptchaAnswer{})
	pin = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	var otp *sber.PinOTPRequired
	if errors.As(err, &otp) {
		phase = "owner-input"
		code, readErr := read(ctx, ownerinput.OTP)
		if readErr != nil {
			return validated, readErr
		}
		phase = "sms-confirmation"
		bundle, err = auth.ConfirmOTP(ctx, code)
		code = "" //nolint:ineffassign,wastedassign // Explicitly mark the end of the transient credential lifetime; strings cannot be zeroed in place.
	}
	if err != nil {
		return validated, err
	}
	phase = "session-validation"
	validated, err = bundle.Clone()
	if err != nil {
		return validated, err
	}
	if _, err = validated.ToSeed(true); err != nil {
		return sber.SessionBundle{}, err
	}
	return validated, nil
}
