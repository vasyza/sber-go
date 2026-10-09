//go:build linux || darwin

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-sdk"
	"github.com/vasyza/sber-sdk/internal/ownerinput"
	"github.com/vasyza/sber-sdk/internal/testutil"
	"github.com/vasyza/sber-sdk/mcp"
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

func TestDefaultProfileLoginReuseAndRestoration(t *testing.T) {
	profile := filepath.Join(testPrivateDir(t), "configuration", "sber-sdk", "profile.json")
	primary := &syntheticPrimary{}
	client := &testutil.Client{}
	options := Options{
		DefaultProfilePath: func() (string, error) { return profile, nil },
		Authentication: &Authentication{
			NewPrimary: func() (PrimaryAuthenticator, error) { return primary, nil },
			ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
				if prompt == ownerinput.PIN {
					return "12345", nil
				}
				if prompt == ownerinput.OTP {
					return "654321", nil
				}
				return "synthetic-private-secret", nil
			},
		},
		OpenClient: func(path string) (mcp.Client, error) {
			if path != profile {
				t.Fatal("read did not reuse the default profile")
			}
			if _, err := sber.LoadSessionBundle(path); err != nil {
				t.Fatal("default profile could not be reopened")
			}
			return client, nil
		},
	}
	var output, diagnostics bytes.Buffer
	run := func(args ...string) int {
		output.Reset()
		diagnostics.Reset()
		code := RunWithOptions(context.Background(), args, &output, &diagnostics, options)
		for _, secret := range []string{"synthetic-private-secret", "12345", "654321", profile} {
			if strings.Contains(output.String()+diagnostics.String(), secret) {
				t.Fatal("default profile flow disclosed private state")
			}
		}
		return code
	}
	if code := run("login"); code != 0 || strings.Join(primary.steps, ",") != "login,otp,pin,close" {
		t.Fatalf("default enrollment failed: %d %s", code, diagnostics.String())
	}
	for _, expected := range []struct {
		path string
		mode os.FileMode
	}{{profile, 0600}, {filepath.Dir(profile), 0700}} {
		state, err := os.Stat(expected.path)
		if err != nil || state.Mode().Perm() != expected.mode {
			t.Fatal("default profile permissions were not private")
		}
	}
	if code := run("status"); code != 0 || !strings.Contains(output.String(), `"profile_exists":true`) {
		t.Fatal("status did not find the enrolled default profile")
	}
	if code := run("inspect-session"); code != 0 {
		t.Fatal("inspection did not reuse the enrolled default profile")
	}
	if code := run("products"); code != 0 || len(client.Requests()) != 1 || client.Closes.Load() != 1 {
		t.Fatal("products did not use and close the default client")
	}
	old, err := sber.LoadSessionBundle(profile)
	if err != nil {
		t.Fatal(err)
	}
	old.Cookies[0].Value = "synthetic-old-cookie"
	if err := old.Save(profile); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	remembered := &syntheticRemembered{needsOTP: true}
	options.Authentication.NewPIN = func(path string, _ sber.AuthOptions) (PINAuthenticator, error) {
		if path != profile {
			t.Fatal("restoration selected a different profile")
		}
		return remembered, nil
	}
	if code := run("refresh-session"); code != 0 || remembered.loginCalls != 1 || remembered.otpCalls != 1 || remembered.closeCalls != 1 {
		t.Fatal("default session restoration failed")
	}
	after, err := os.ReadFile(profile)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("restoration did not publish the default session")
	}
	options.Authentication.NewPrimary = func() (PrimaryAuthenticator, error) {
		t.Fatal("existing default profile started primary authentication")
		return nil, nil
	}
	if code := run("login"); code != 3 || !strings.Contains(diagnostics.String(), "Use sber refresh-session") {
		t.Fatal("existing default profile did not give a restoration instruction")
	}
	unchanged, err := os.ReadFile(profile)
	if err != nil || !bytes.Equal(after, unchanged) {
		t.Fatal("repeated login changed the saved default profile")
	}
}
