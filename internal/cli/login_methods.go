package cli

import (
	"context"
	"io"
	"os"
	"reflect"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
	"github.com/vasyza/sber-go/internal/ownerinput"
	sdkSession "github.com/vasyza/sber-go/internal/session"
)

type CardAuthenticator interface {
	LoadConfig(context.Context) (sber.FrontendConfig, error)
	Login(context.Context, string, sber.CaptchaAnswer) (*sber.SessionBundle, error)
	ConfirmOTP(context.Context, string) (*sber.SessionBundle, error)
	CreatePIN(context.Context, string) (sber.SessionBundle, error)
	Close() error
}
type QRAuthenticator interface {
	Start(context.Context) (sber.QRCode, error)
	Poll(context.Context) (sber.QRStatus, *sber.SessionBundle, error)
	Close() error
}
type cardLoginAdapter struct{ CardAuthenticator }

func (a cardLoginAdapter) Login(ctx context.Context, number, _ string, o sber.PrimaryLoginOptions) (*sber.SessionBundle, error) {
	return a.CardAuthenticator.Login(ctx, number, o.Captcha)
}
func (a cardLoginAdapter) LoadConfig(ctx context.Context) (sber.FrontendConfig, error) {
	return a.CardAuthenticator.LoadConfig(ctx)
}
func configureLoginMethods(a *Authentication, args *commandArguments, diagnostics io.Writer) {
	options := func() (sber.AuthOptions, error) {
		o, e := nativeAuthOptions(args.ca, args.selectedProxy)
		if e != nil {
			return o, e
		}
		return withDeviceIdentity(o)
	}
	if a.NewPhone == nil {
		a.NewPhone = func() (CredentialAuthenticator, error) {
			o, e := options()
			if e != nil {
				return nil, e
			}
			return sber.NewPhoneAuth(o)
		}
	}
	if a.NewCard == nil {
		a.NewCard = func() (CardAuthenticator, error) {
			o, e := options()
			if e != nil {
				return nil, e
			}
			return sber.NewCardAuth(o)
		}
	}
	if a.NewQR == nil {
		a.NewQR = func() (QRAuthenticator, error) {
			o, e := options()
			if e != nil {
				return nil, e
			}
			return sber.NewQRAuth(o)
		}
	}
	if a.DisplayQR == nil {
		a.DisplayQR = func(c sber.QRCode) error { return displayLoginQR(args.qrOutput, c, diagnostics) }
	}
}
func prepareMethodLogin(ctx context.Context, a Authentication, method string, diagnostics io.Writer) (enrollment.CandidateWriter, error) {
	switch method {
	case "phone":
		auth, e := a.NewPhone()
		if e != nil {
			return nil, e
		}
		return prepareCredentialLogin(ctx, a, auth, ownerinput.Phone, "phone-password", diagnostics)
	case "card":
		auth, e := a.NewCard()
		if e != nil {
			return nil, e
		}
		if nilAuthenticator(auth) {
			return nil, enrollment.ErrPrepare
		}
		return prepareCredentialLogin(ctx, a, cardLoginAdapter{auth}, ownerinput.CardNumber, "card-login", diagnostics)
	case "qr":
		return prepareQRLogin(ctx, a)
	default:
		return prepareLogin(ctx, a, diagnostics)
	}
}
func nilAuthenticator(auth any) bool {
	return auth == nil || reflect.ValueOf(auth).Kind() == reflect.Pointer && reflect.ValueOf(auth).IsNil()
}
func prepareQRLogin(ctx context.Context, a Authentication) (writer enrollment.CandidateWriter, err error) {
	auth, e := a.NewQR()
	if e != nil {
		return nil, e
	}
	if nilAuthenticator(auth) {
		return nil, enrollment.ErrPrepare
	}
	phase := "public-configuration"
	defer func() {
		if auth.Close() != nil {
			writer = nil
			err = enrollment.ErrPrepare
			phase = "cleanup"
		}
		if err != nil {
			err = &loginPhaseError{phase: phase, cause: err}
		}
	}()
	if e = prepareAuthentication(ctx, auth); e != nil {
		return nil, e
	}
	phase = "qr-login"
	child, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	challenge, e := auth.Start(child)
	if e != nil {
		return nil, e
	}
	if e = child.Err(); e != nil {
		return nil, e
	}
	if e = a.DisplayQR(challenge); e != nil {
		return nil, enrollment.ErrPrepare
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-child.Done():
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, &sber.PinAuthError{Code: "qr_expired"}
		case <-ticker.C:
		}
		status, b, e := auth.Poll(child)
		if e != nil {
			return nil, e
		}
		switch status {
		case sber.QRRefused:
			return nil, &sber.PinAuthError{Code: "qr_refused"}
		case sber.QRExpired:
			return nil, &sber.PinAuthError{Code: "qr_expired"}
		case sber.QRConfirmed:
			phase = "session-validation"
			if b == nil {
				return nil, enrollment.ErrPrepare
			}
			validated, e := b.Clone()
			if e != nil {
				return nil, e
			}
			if _, e = validated.ToSeed(true); e != nil {
				return nil, e
			}
			return func(ctx context.Context, path string) error {
				if e := ctx.Err(); e != nil {
					return e
				}
				return sdkSession.WriteEnrollmentCandidate(path, validated)
			}, nil
		case sber.QRNew, sber.QRWaitConfirm:
		default:
			return nil, &sber.PinAuthError{Code: "invalid_qr_state"}
		}
	}
}
func displayLoginQR(path string, c sber.QRCode, output io.Writer) error {
	qr, e := qrcode.New(c.Content(), qrcode.Medium)
	if e != nil {
		return e
	}
	if path != "" {
		raw, e := qr.PNG(384)
		if e != nil {
			return e
		}
		e = enrollment.Enroll(context.Background(), path, func(context.Context) (enrollment.CandidateWriter, error) {
			return func(ctx context.Context, candidate string) error {
				if e := ctx.Err(); e != nil {
					return e
				}
				return os.WriteFile(candidate, raw, 0600)
			}, nil
		})
		if e != nil {
			return e
		}
		_, e = io.WriteString(output, "The command saved the QR challenge to --qr-output.\nScan it in the bank application.\nConfirm this login in the application.\n")
		return e
	}
	_, e = io.WriteString(output, "Scan this QR in the bank application.\nConfirm this login in the application.\n"+qr.ToSmallString(false))
	return e
}
