package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
)

func TestNativeCommandOfflineStatus(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "sber")
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	build := exec.CommandContext(context.Background(), goBinary, "build", "-o", binary, "../../cmd/sber")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("native command unavailable: %v: %s", err, output)
	}
	output, err := exec.CommandContext(context.Background(), binary, "status", "--profile", filepath.Join(dir, "state", "profile.json")).CombinedOutput()
	if err != nil {
		t.Fatalf("native status failed: %v: %s", err, output)
	}
	var facts map[string]any
	if err := json.Unmarshal(output, &facts); err != nil {
		t.Fatal(err)
	}
	if facts["profile_exists"] != false || facts["bank_authorization_checked"] != false {
		t.Fatal("native offline status claimed bank state")
	}
	t.Run("default profile", func(t *testing.T) {
		configuration := filepath.Join(dir, "configuration")
		profile := filepath.Join(configuration, "sber-go", "profile.json")
		if runtime.GOOS == "darwin" {
			profile = filepath.Join(dir, "Library", "Application Support", "sber-go", "profile.json")
		}
		run := func(args ...string) (bytes.Buffer, bytes.Buffer, error) {
			var out, diagnostics bytes.Buffer
			command := exec.CommandContext(context.Background(), binary, args...)
			command.Env = []string{"HOME=" + dir, "XDG_CONFIG_HOME=" + configuration}
			command.Dir = t.TempDir()
			command.Stdout, command.Stderr = &out, &diagnostics
			err := command.Run()
			if strings.Contains(out.String()+diagnostics.String(), profile) || strings.Contains(out.String()+diagnostics.String(), "synthetic-native-profile-secret") {
				t.Fatal("native default command disclosed profile state")
			}
			return out, diagnostics, err
		}
		status := func(exists bool, args ...string) {
			out, diagnostics, err := run(args...)
			if err != nil {
				t.Fatalf("native default status failed: %v: %s", err, diagnostics.String())
			}
			var result map[string]any
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result["profile_exists"] != exists || result["bank_authorization_checked"] != false || diagnostics.Len() != 0 {
				t.Fatal("default status selected the wrong profile or claimed bank authorization")
			}
		}
		status(false, "status")
		if _, err := os.Lstat(filepath.Dir(profile)); !os.IsNotExist(err) {
			t.Fatal("default status created profile state")
		}
		out, diagnostics, err := run("products")
		if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 || out.Len() != 0 || diagnostics.String() != "The CLI has no saved profile.\nUse sber login to make a profile.\n" {
			t.Fatal("native missing profile did not stop with a login instruction")
		}
		if err := os.MkdirAll(filepath.Dir(profile), 0700); err != nil {
			t.Fatal(err)
		}
		bundle, err := sber.NewSessionBundle(sber.SessionBundle{
			APIBase: sber.AppOrigin, WebBase: sber.AppOrigin,
			Cookies: []sber.CookieRecord{{Name: "fixture", Value: "synthetic-native-profile-secret", Domain: "online.sberbank.ru", Path: "/", Secure: true}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := bundle.Save(profile); err != nil {
			t.Fatal(err)
		}
		status(true, "status")
		status(false, "status", "--profile", filepath.Join(dir, "different", "profile.json"))
		out, diagnostics, err = run("inspect-session")
		var inspection struct {
			Metadata struct{ Values string }
		}
		if err != nil || diagnostics.Len() != 0 || json.Unmarshal(out.Bytes(), &inspection) != nil || inspection.Metadata.Values != "<redacted>" {
			t.Fatal("native inspection did not use a redacted default profile")
		}
	})
	t.Run("saved proxy", func(t *testing.T) {
		configuration := filepath.Join(dir, "proxy-user")
		if err := os.MkdirAll(configuration, 0700); err != nil {
			t.Fatal(err)
		}
		run := func(args ...string) (string, int) {
			t.Helper()
			command := exec.CommandContext(context.Background(), binary, args...)
			command.Env = []string{"HOME=" + configuration, "XDG_CONFIG_HOME=" + configuration}
			command.Dir = t.TempDir()
			output, err := command.CombinedOutput()
			code := 0
			if err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if strings.Contains(string(output), "synthetic-native-proxy-secret") {
				t.Fatal("native proxy output disclosed credentials")
			}
			return string(output), code
		}
		if output, code := run("config", "set", "proxy", "socks5://127.0.0.1:1080:synthetic-native-proxy-secret-user:synthetic-native-proxy-secret-password"); code != 0 || output != "" {
			t.Fatal("native proxy setting failed")
		}
		if output, code := run("config", "get", "proxy"); code != 0 || output != "socks5://127.0.0.1:1080\n" {
			t.Fatal("native proxy persistence failed")
		}
		if output, code := run("config", "list"); code != 0 || output != "proxy=socks5://127.0.0.1:1080\n" {
			t.Fatal("native proxy list failed")
		}
		if output, code := run("config", "set", "proxy", "invalid:synthetic-native-proxy-secret"); code != 2 || !strings.Contains(output, "arguments are not valid") {
			t.Fatal("native proxy argument rejection failed")
		}
		if output, code := run("config", "unset", "proxy"); code != 0 || output != "" {
			t.Fatal("native proxy unset failed")
		}
		if output, code := run("config", "list"); code != 0 || output != "" {
			t.Fatal("native proxy unset retained settings")
		}
	})
	canary := "SYNTHETIC-private-native-argument"
	for _, test := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0},
		{[]string{"help", "inspect-session"}, 0},
		{[]string{"status", "--password=" + canary}, 2},
		{[]string{"help", canary}, 2},
		{[]string{"__complete", "status", "--password=" + canary, ""}, 2},
		{[]string{"__completeNoDesc", "status", "--password=" + canary, ""}, 2},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			command := exec.CommandContext(context.Background(), binary, test.args...)
			var out, diagnostics bytes.Buffer
			command.Stdout, command.Stderr = &out, &diagnostics
			code := 0
			if err := command.Run(); err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != test.code || strings.Contains(out.String()+diagnostics.String(), canary) {
				t.Fatalf("native help or privacy failure: exit=%d stdout=%q stderr=%q", code, out.String(), diagnostics.String())
			}
			if test.code == 0 {
				if diagnostics.Len() != 0 || !strings.Contains(out.String(), "Use:") {
					t.Fatal("native command help is missing")
				}
			} else if out.Len() != 0 || diagnostics.String() != "The command arguments are not valid.\nUse sber --help for command help.\n" {
				t.Fatal("native argument error is not static")
			}
		})
	}
}
