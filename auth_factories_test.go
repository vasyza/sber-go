package sber

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthFactoriesFingerprintsAndProfileOptions(t *testing.T) {
	d, anti, e := GenerateFingerprints()
	if e != nil || d.Value() == "" || anti.Value() == "" {
		t.Fatal("identity pair generation failed")
	}
	want, _ := GenerateAntifraudDeviceprint(d.Value())
	if anti.Value() != want.Value() {
		t.Fatal("antifraud identity not derived from this deviceprint")
	}
	b := authBundle(t)
	path := filepath.Join(t.TempDir(), "synthetic-profile.json")
	if e = b.Save(path); e != nil {
		t.Fatal(e)
	}
	provider := BrowserBootstrapFunc(func(context.Context, SessionBundle, string) (BrowserBootstrapResult, error) {
		return BrowserBootstrapResult{}, nil
	})
	created := 0
	o := AuthOptions{TransportOptions: TransportOptions{CABundle: "synthetic-ca", Timeout: 123}, BrowserBootstrap: provider, BrowserFirst: true, TransportFactory: func(b SessionBundle, o TransportOptions) (Transport, error) {
		created++
		if o.CABundle != "synthetic-ca" || o.Timeout != 123 || !o.AllowUnready || b.Deviceprint == nil {
			t.Error("factory dropped auth options/profile")
		}
		return newAuthScript(t), nil
	}}
	pin, e := NewPINAuthFromProfile(path, o)
	if e != nil {
		t.Fatal(e)
	}
	if pin.Stage() != AuthStageBootstrap {
		t.Fatal("anonymous profile claimed ready")
	}
	_ = pin.Close()
	if pin.Stage() != AuthStageClosed {
		t.Fatal("stage not closed")
	}
	primary, e := NewPrimaryAuthFromProfile(path, o)
	if e != nil {
		t.Fatal(e)
	}
	_ = primary.Close()
	dValue := d.Value()
	o.Deviceprint = &dValue
	cold, e := NewPrimaryAuth(o)
	if e != nil {
		t.Fatal(e)
	}
	export, e := cold.ExportSession()
	if e != nil || len(export.Cookies) != 0 {
		t.Fatal("cold auth did not start blank")
	}
	_ = cold.Close()
	if created != 3 {
		t.Fatal("factory construction count mismatch")
	}
	var cb PINProvider = func(context.Context) (string, error) { return "13579", nil }
	_ = cb
	if strings.Contains(fmt.Sprintf("%+v", pin), "synthetic-ca") {
		t.Fatal("auth leaked options")
	}
}
func TestAuthPrimarySuccessClearsProcessProof(t *testing.T) {
	s := newAuthScript(t)
	s.steps = primaryNativeSteps(t, map[string]any{"pinInfo": map[string]any{"webPinSkip": true}})
	s.steps = append(s.steps, primaryFinishSteps(t, s)...)
	a := newPrimaryForScript(t, s)
	if _, e := a.Login(context.Background(), "fixture-login", "fixture-password", PrimaryLoginOptions{}); e != nil {
		t.Fatal(e)
	}
	if a.Stage() != AuthStageAuthenticated || a.primarySRP != nil || a.primaryToken != "" || a.primaryOTPPending || a.pinPublicKey != "" {
		t.Fatal("completed primary retained process proof/token state")
	}
}
