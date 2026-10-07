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

func (b loginBrowserSelection) authOptions(ca string, proxy sber.ProxyOptions) (sber.AuthOptions, error) {
	o := sber.AuthOptions{TransportOptions: sber.TransportOptions{CABundle: ca, Timeout: 60 * time.Second, Proxy: proxy}}
	if b.profile == "" {
		return o, nil
	}
	provider, err := browser.NewFirefoxBootstrap(browser.FirefoxOptions{ProfileDir: b.profile, DriverDir: b.driver, FirefoxExecutable: b.executable, Timeout: 60 * time.Second, Proxy: proxy})
	if err != nil {
		return sber.AuthOptions{}, err
	}
	o.BrowserFirst, o.BrowserBootstrap, o.BrowserBootstrapTimeout = true, provider, 60*time.Second
	return o, nil
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
		}
	}()
	if err := prepareAuthentication(ctx, auth); err != nil {
		return validated, err
	}
	pin, err := read(ctx, ownerinput.PIN)
	if err != nil {
		return validated, err
	}
	bundle, err := auth.Login(ctx, pin, sber.CaptchaAnswer{})
	pin = ""
	var otp *sber.PinOTPRequired
	if errors.As(err, &otp) {
		code, readErr := read(ctx, ownerinput.OTP)
		if readErr != nil {
			return validated, readErr
		}
		bundle, err = auth.ConfirmOTP(ctx, code)
		code = ""
	}
	if err != nil {
		return validated, err
	}
	validated, err = bundle.Clone()
	if err != nil {
		return validated, err
	}
	if _, err = validated.ToSeed(true); err != nil {
		return sber.SessionBundle{}, err
	}
	return validated, nil
}
