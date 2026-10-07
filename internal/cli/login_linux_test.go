//go:build linux

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

func TestOwnerLoginUsesRealEnrollmentWithSDKSessionWriter(t *testing.T) {
	profile := filepath.Join(testPrivateDir(t), "private", "profile.json")
	auth := &syntheticPrimary{}
	var out, diagnostics bytes.Buffer
	options := Options{Authentication: &Authentication{
		NewPrimary: func() (PrimaryAuthenticator, error) { return auth, nil },
		ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "synthetic-private-secret", nil },
	}}
	if code := RunWithOptions(context.Background(), []string{"login", "--profile", profile}, &out, &diagnostics, options); code != 0 {
		t.Fatalf("real enrollment failed: %d %s", code, diagnostics.String())
	}
	if _, err := sber.LoadSessionBundle(profile); err != nil {
		t.Fatal("enrolled SDK profile cannot be reopened")
	}
	before, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal("synthetic profile absent")
	}
	options.Authentication.NewPrimary = func() (PrimaryAuthenticator, error) {
		t.Fatal("existing profile started another login")
		return nil, nil
	}
	out.Reset()
	diagnostics.Reset()
	if code := RunWithOptions(context.Background(), []string{"login", "--profile", profile}, &out, &diagnostics, options); code != 3 {
		t.Fatal("existing profile was not protected")
	}
	after, err := os.ReadFile(profile)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("existing profile bytes changed")
	}
}
