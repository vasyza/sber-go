package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

type authFlakyClose struct{ *authScript }

func (s *authFlakyClose) Close() error {
	s.authScript.Close()
	if s.closes == 1 {
		return errors.New("synthetic private close URL")
	}
	return nil
}
func TestAuthFailedCloseRetryStaysClosed(t *testing.T) {
	s := &authFlakyClose{newAuthScript(t)}
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	if e := a.Close(); e == nil {
		t.Fatal("failed close suppressed")
	}
	if _, e := a.LoadConfig(context.Background()); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("failed close reopened")
	}
	if e := a.Close(); e != nil {
		t.Fatal(e)
	}
	if e := a.Close(); e != nil || s.closes != 2 {
		t.Fatal("close cleanup retry count wrong")
	}
}
func TestAuthTransportFailureRedactedAndNoReplay(t *testing.T) {
	s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(authHTML(false))}, authStep{method: "POST", target: sdkSession.AppOrigin + "/CSAFront/api/v1/pin/begin", err: errors.New("private-support-id synthetic-password synthetic-otp")})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	defer a.Close()
	_, e := a.Login(context.Background(), "13579", CaptchaAnswer{})
	if e == nil || strings.Contains(fmt.Sprintf("%+v", e), "private-support-id") || s.calls != 2 {
		t.Fatal("raw transport error leak or request replay")
	}
}
func TestAuthCloseRacingCredentialResponseNeverReady(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	steps, _, _ := pinNativeSteps(t, "")
	s := newAuthScript(t)
	s.steps = append(steps, authStep{method: "GET", target: sdkSession.AppOrigin + "/finish", response: authPage(`{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`), run: func(context.Context) { close(started); <-release; setAuthCookies(t, s.jar) }})
	a, _ := NewPINAuth(authBundle(t), AuthOptions{Transport: s})
	done := make(chan error, 1)
	go func() { _, e := a.Login(context.Background(), "13579", CaptchaAnswer{}); done <- e }()
	<-started
	_ = a.Close()
	close(release)
	if e := <-done; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("late final response claimed ready")
	}
	if a.authenticated {
		t.Fatal("closed auth state committed")
	}
}
