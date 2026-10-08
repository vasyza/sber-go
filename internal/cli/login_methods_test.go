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
	"github.com/vasyza/sber-go/internal/ownerinput"
)

type syntheticCard struct{ syntheticPrimary }

type syntheticPhone struct{ syntheticPrimary }

func (*syntheticPhone) Login(context.Context, string, string, sber.PrimaryLoginOptions) (*sber.SessionBundle, error) {
	b := syntheticLoginBundle()
	return &b, nil
}

func (s *syntheticCard) LoadConfig(context.Context) (sber.FrontendConfig, error) {
	return sber.NewFrontendConfig(sber.AppOrigin, "synthetic-process", 5, "", "", true, false), nil
}

func (s *syntheticCard) Login(ctx context.Context, pan string, o sber.CaptchaAnswer) (*sber.SessionBundle, error) {
	b := syntheticLoginBundle()
	return &b, nil
}

func TestLoginPhoneAndCardSelectOnlyRequiredSecrets(t *testing.T) {
	for _, method := range []string{"phone", "card"} {
		t.Run(method, func(t *testing.T) {
			dir := testPrivateDir(t)
			profile := filepath.Join(dir, "new.json")
			var out, diag bytes.Buffer
			var prompts []ownerinput.Prompt
			code := RunWithOptions(context.Background(), []string{"login", "--method", method, "--profile", profile}, &out, &diag, Options{Authentication: &Authentication{
				NewPhone: func() (CredentialAuthenticator, error) { return &syntheticPhone{}, nil },
				NewCard:  func() (CardAuthenticator, error) { return &syntheticCard{}, nil },
				ReadSecret: func(_ context.Context, p ownerinput.Prompt) (string, error) {
					prompts = append(prompts, p)
					return "synthetic-owner-input", nil
				},
			}})
			if code != 0 {
				t.Fatal(code, diag.String())
			}
			if _, e := os.Stat(profile); e != nil {
				t.Fatal(e)
			}
			if method == "phone" && (len(prompts) != 2 || prompts[0] != ownerinput.Phone || prompts[1] != ownerinput.Password) {
				t.Fatal("wrong phone prompts")
			}
			if method == "card" && (len(prompts) != 1 || prompts[0] != ownerinput.CardNumber) {
				t.Fatal("wrong card prompts")
			}
		})
	}
}

func TestLoginMethodArgumentsFailBeforeInput(t *testing.T) {
	for _, flags := range [][]string{{"--method", "unknown"}, {"--method", "phone", "--remembered-profile", "source.json"}, {"--method", "card", "--qr-output", "/tmp/qr.png"}, {"--method", "qr", "--qr-output", "relative.png"}, {"--browser-profile", "/old/browser"}, {"--method", "qr", "--qr-output", "/synthetic/new.json"}} {
		var out, diag bytes.Buffer
		code := RunWithOptions(context.Background(), append([]string{"login", "--profile", "/synthetic/new.json"}, flags...), &out, &diag, Options{Authentication: &Authentication{ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
			t.Fatal("invalid arguments requested secret")
			return "", nil
		}}})
		if code != 2 {
			t.Fatal("invalid method arguments accepted")
		}
	}
}

type syntheticQR struct {
	status     sber.QRStatus
	bundle     *sber.SessionBundle
	closeError error
	closed     bool
	started    bool
	startHook  func()
}

func (q *syntheticQR) Start(context.Context) (sber.QRCode, error) {
	q.started = true
	if q.startHook != nil {
		q.startHook()
	}
	return sber.QRCode{}, nil
}
func (q *syntheticQR) Poll(context.Context) (sber.QRStatus, *sber.SessionBundle, error) {
	return q.status, q.bundle, nil
}
func (q *syntheticQR) Close() error { q.closed = true; return q.closeError }

func TestQRLoginPublishesOnlyReadySessionAfterCleanup(t *testing.T) {
	for _, scenario := range []string{"ready", "refused", "missing-session", "cleanup-failure"} {
		t.Run(scenario, func(t *testing.T) {
			profile := filepath.Join(testPrivateDir(t), "qr.json")
			b := syntheticLoginBundle()
			q := &syntheticQR{status: sber.QRConfirmed, bundle: &b}
			if scenario == "refused" {
				q.status = sber.QRRefused
			}
			if scenario == "missing-session" {
				q.bundle = nil
			}
			if scenario == "cleanup-failure" {
				q.closeError = errors.New("synthetic-private-cleanup")
			}
			displayed := false
			var out, diag bytes.Buffer
			code := RunWithOptions(context.Background(), []string{"login", "--method", "qr", "--profile", profile}, &out, &diag, Options{Authentication: &Authentication{
				NewQR:     func() (QRAuthenticator, error) { return q, nil },
				DisplayQR: func(sber.QRCode) error { displayed = true; return nil },
				ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) {
					t.Fatal("QR requested a credential")
					return "", nil
				},
			}})
			if !q.closed || !displayed {
				t.Fatal("QR challenge or cleanup missing")
			}
			if scenario == "ready" {
				if code != 0 {
					t.Fatal("QR login failed", diag.String())
				}
				if _, err := sber.LoadSessionBundle(profile); err != nil {
					t.Fatal("confirmed QR profile missing", err)
				}
			} else {
				if code != 3 {
					t.Fatal("invalid QR result succeeded")
				}
				if _, err := os.Stat(profile); !os.IsNotExist(err) {
					t.Fatal("invalid QR published a profile")
				}
				if strings.Contains(diag.String(), "synthetic-private-cleanup") {
					t.Fatal("QR diagnostic exposed private details")
				}
			}
		})
	}
}

func TestQRCanceledContextDoesNotStartOrDisplayChallenge(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := &syntheticQR{}
	displayed := false
	writer, err := prepareQRLogin(ctx, Authentication{NewQR: func() (QRAuthenticator, error) { return q, nil }, DisplayQR: func(sber.QRCode) error { displayed = true; return nil }})
	if writer != nil || err == nil || q.started || displayed || !q.closed {
		t.Fatal("canceled QR operation produced a challenge")
	}
}

func TestQRLoginCancelsBeforeChallengeDisplay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &syntheticQR{startHook: cancel}
	displayed := false
	writer, err := prepareQRLogin(ctx, Authentication{NewQR: func() (QRAuthenticator, error) { return q, nil }, DisplayQR: func(sber.QRCode) error { displayed = true; return nil }})
	if writer != nil || !errors.Is(err, context.Canceled) || displayed || !q.closed {
		t.Fatal("QR cancellation was ignored or mislabeled")
	}
}
