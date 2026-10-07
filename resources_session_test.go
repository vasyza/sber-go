package sber

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func resourceSession(t *testing.T) (SessionBundle, SberCredentials) {
	t.Helper()
	c, err := NewSberCredentials("fixture-session", "fixture-token")
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.ToBundle(CredentialsBundleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return b, c
}
func TestResourceSessionExportMemoryAndExplicitSyntheticPath(t *testing.T) {
	b, c := resourceSession(t)
	r := &resourceScript{t: t, bundle: b, credentials: c}
	a := NewSessionAPI(r)
	got, err := a.Export()
	if err != nil || !reflect.DeepEqual(got, b) {
		t.Fatal("export lost session")
	}
	got.Cookies[0].Value = "changed"
	if r.bundle.Cookies[0].Value == "changed" {
		t.Fatal("export aliases requester")
	}
	path := filepath.Join(t.TempDir(), "synthetic-session.json")
	got, err = a.Export(path)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := LoadSessionBundle(path)
	if err != nil || !reflect.DeepEqual(restored, b) {
		t.Fatal("explicit save not exercised")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("not private")
	}
	failure := errors.New("fixture export failure")
	r.exportErr = failure
	if _, err = a.Export(); err != failure {
		t.Fatal("export error lost")
	}
	if len(r.calls) != 0 {
		t.Fatal("export sent mutation")
	}
}
func TestResourceSessionCredentialsExplicitRotatedValues(t *testing.T) {
	b, c := resourceSession(t)
	r := &resourceScript{t: t, bundle: b, credentials: c}
	a := NewSessionAPI(r)
	got, err := a.Credentials()
	if err != nil || got != c {
		t.Fatal("credentials lost")
	}
	r.credentials, _ = NewSberCredentials("fixture-rotated-session", "fixture-rotated-token")
	got, err = a.Credentials()
	if err != nil || got != r.credentials {
		t.Fatal("stale credentials")
	}
	failure := errors.New("fixture credentials failure")
	r.exportErr = failure
	if _, err = a.Credentials(); err != failure {
		t.Fatal("credentials error lost")
	}
	if len(r.calls) != 0 {
		t.Fatal("credentials mutated")
	}
}
func TestResourceSessionWarmUpExplicitForceAndDefault(t *testing.T) {
	r := &resourceScript{t: t}
	a := NewSessionAPI(r)
	if err := a.WarmUp(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.WarmUp(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.warm, []bool{true, false}) || len(r.calls) != 0 {
		t.Fatal("wrong session dispatch")
	}
	failure := errors.New("fixture warm failure")
	r.warmErr = failure
	if err := a.WarmUp(context.Background(), true); err != failure {
		t.Fatal("warm error lost")
	}
}
