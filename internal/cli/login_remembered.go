package cli

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"time"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/browser"
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

// PINAuthenticator authenticates one remembered device without changing its PIN.
type PINAuthenticator interface {
	Login(context.Context, string, sber.CaptchaAnswer) (sber.SessionBundle, error)
	ConfirmOTP(context.Context, string) (sber.SessionBundle, error)
	Close() error
}

type loginBrowserSelection struct{ profile, driver, executable string }

func (b loginBrowserSelection) valid() bool {
	if b.profile == "" && b.driver == "" && b.executable == "" {
		return true
	}
	return filepath.IsAbs(b.profile) && filepath.IsAbs(b.driver) && filepath.IsAbs(b.executable)
}

func (b loginBrowserSelection) authOptions(ca string) (sber.AuthOptions, error) {
	o := sber.AuthOptions{TransportOptions: sber.TransportOptions{CABundle: ca, Timeout: 60 * time.Second}}
	if b.profile == "" {
		return o, nil
	}
	provider, err := browser.NewFirefoxBootstrap(browser.FirefoxOptions{ProfileDir: b.profile, DriverDir: b.driver, FirefoxExecutable: b.executable, Timeout: 60 * time.Second})
	if err != nil {
		return sber.AuthOptions{}, err
	}
	o.BrowserFirst, o.BrowserBootstrap, o.BrowserBootstrapTimeout = true, provider, 60*time.Second
	return o, nil
}

func prepareRememberedLogin(ctx context.Context, a Authentication, source string, options sber.AuthOptions) (writer enrollment.CandidateWriter, err error) {
	auth, err := a.NewPIN(source, options)
	if err != nil {
		return nil, err
	}
	if auth == nil || reflect.ValueOf(auth).Kind() == reflect.Pointer && reflect.ValueOf(auth).IsNil() {
		return nil, enrollment.ErrPrepare
	}
	defer func() {
		if auth.Close() != nil {
			writer = nil
			err = enrollment.ErrPrepare
		}
	}()
	pin, err := a.ReadSecret(ctx, ownerinput.PIN)
	if err != nil {
		return nil, err
	}
	bundle, err := auth.Login(ctx, pin, sber.CaptchaAnswer{})
	pin = ""
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
