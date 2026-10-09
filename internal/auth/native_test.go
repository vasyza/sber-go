package auth

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

func TestNativeAuthOptionsHaveNoBrowserRuntime(t *testing.T) {
	for _, name := range []string{"BrowserBootstrap", "BrowserFirst", "BrowserBootstrapTimeout"} {
		if _, ok := reflect.TypeFor[AuthOptions]().FieldByName(name); ok {
			t.Errorf("native auth still exposes %s", name)
		}
	}
}

func TestNativeAuthSendsDeclaredSDKIdentityAndPreservesExplicitHeaders(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "explicit"}[explicit], func(t *testing.T) {
			s := newAuthScript(t)
			o := methodOptions(s)
			o.Transport = nil
			if explicit {
				o.Browser.Headers = []sdkSession.BrowserHeader{{Name: "User-Agent", Value: "synthetic-explicit-agent"}}
			}
			o.TransportFactory = func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				ua := b.Browser.AsMap()["user-agent"]
				if explicit {
					if ua != "synthetic-explicit-agent" {
						t.Error("explicit client identity changed")
					}
				} else if !strings.Contains(ua, "compatible; sber-sdk/") || strings.Contains(ua, "Chrome/") || strings.Contains(ua, "Firefox/") {
					t.Error("native client identity missing or claims a browser engine")
				}
				return s, nil
			}
			a, err := NewPrimaryAuth(o)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
		})
	}
}

func TestNativeConfigCannotResurrectAfterClose(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	step := methodBootstrap()
	step.run = func(context.Context) { close(started); <-release }
	s := newAuthScript(t, step)
	a, e := NewPrimaryAuth(methodOptions(s))
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := a.LoadConfig(context.Background()); done <- e }()
	<-started
	if e = a.Close(); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e = <-done; !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed auth accepted config")
	}
	if _, e = a.ExportSession(); !errors.Is(e, sdkErrs.ErrClosed) {
		t.Fatal("closed auth exported session")
	}
}

func TestAllNativeMethodsStopBeforeAuthenticationWhenPublicPageIsRejected(t *testing.T) {
	for _, method := range []string{"login", "phone", "card", "qr"} {
		t.Run(method, func(t *testing.T) {
			s := newAuthScript(t, authStep{method: "GET", target: sdkTransport.PublicBootstrapURL, response: authPage(`<html><title>Нельзя войти в СберБанк Онлайн в этом браузере.</title><body>synthetic-private-support-id</body></html>`)})
			opts := methodOptions(s)
			var a interface {
				LoadConfig(context.Context) (sdkSession.FrontendConfig, error)
				Stage() AuthStage
				Close() error
			}
			var err error
			switch method {
			case "login":
				a, err = NewPrimaryAuth(opts)
			case "phone":
				a, err = NewPhoneAuth(opts)
			case "card":
				a, err = NewCardAuth(opts)
			case "qr":
				a, err = NewQRAuth(opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			config, err := a.LoadConfig(context.Background())
			authErrorCode(t, err, "login_page_rejected")
			var failure *sdkErrs.PinAuthError
			if !errors.As(err, &failure) || failure.StatusCode != 200 || config != (sdkSession.FrontendConfig{}) || a.Stage() != AuthStageBootstrap || s.calls != 1 {
				t.Fatal("rejected public page advanced authentication")
			}
			if strings.Contains(err.Error(), "synthetic-private") {
				t.Fatal("public rejection exposed response details")
			}
		})
	}
}
