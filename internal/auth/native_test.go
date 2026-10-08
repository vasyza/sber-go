package auth

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
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
				} else if !strings.Contains(ua, "compatible; sber-go/") || strings.Contains(ua, "Chrome/") || strings.Contains(ua, "Firefox/") {
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
