package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/mcp"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelpDoesNotInspectProfile(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "absent", "profile.json")
	for _, args := range [][]string{
		{"--help"}, {"-h"}, {"help"},
		{"help", "status"}, {"help", "inspect-session"},
		{"status", "--help"}, {"inspect-session", "-h"},
		{"inspect-session", "--profile", profile, "--help"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			if code := Run(context.Background(), args, &out, &diagnostics); code != 0 {
				t.Fatalf("help exit %d: %s", code, diagnostics.String())
			}
			if diagnostics.Len() != 0 || !strings.Contains(out.String(), "Use:") || !strings.Contains(out.String(), "Show command help.") {
				t.Fatalf("missing command help: stdout=%q stderr=%q", out.String(), diagnostics.String())
			}
			if strings.Contains(out.String(), profile) || strings.Contains(out.String(), "bank_authorization_checked") {
				t.Fatal("help disclosed a profile path or ran a command")
			}
		})
	}
	if _, err := os.Lstat(filepath.Dir(profile)); !os.IsNotExist(err) {
		t.Fatal("help created profile state")
	}
}

func TestCLIArgumentErrorsRemainStatic(t *testing.T) {
	canary := "SYNTHETIC-private-argument"
	for _, args := range [][]string{
		nil, {}, {canary}, {"completion"}, {"status"}, {"inspect-session"},
		{"status", "--profile"}, {"status", "--profile="},
		{"status", "--profile", ""},
		{"status", "--password=" + canary},
		{"status", "--profile", canary, "--help=" + canary},
		{"status", canary, "--profile", canary},
		{"status", "--profile", canary, "--", canary},
		{"help", canary}, {"help", "status", canary},
		{"__complete", "status", "--password=" + canary, ""},
		{"__completeNoDesc", canary, ""},
		{"help", "__complete"}, {"help", "__completeNoDesc"},
	} {
		var out, diagnostics bytes.Buffer
		if code := Run(context.Background(), args, &out, &diagnostics); code != 2 {
			t.Fatalf("argument rejection exit %d for %q", code, args)
		}
		if out.Len() != 0 || diagnostics.String() != "The command arguments are not valid.\nUse sber --help for command help.\n" {
			t.Fatalf("argument rejection changed or disclosed input: stdout=%q stderr=%q", out.String(), diagnostics.String())
		}
	}
}

func TestCLIFlagValuesDoNotPersistAcrossRuns(t *testing.T) {
	for range 4 {
		t.Run("run", func(t *testing.T) {
			t.Parallel()
			profile := filepath.Join(t.TempDir(), "absent", "profile.json")
			var out, diagnostics bytes.Buffer
			if code := Run(context.Background(), []string{"status", "--profile=" + profile}, &out, &diagnostics); code != 0 {
				t.Fatalf("status exit %d: %s", code, diagnostics.String())
			}
			var facts map[string]any
			if err := json.Unmarshal(out.Bytes(), &facts); err != nil {
				t.Fatal(err)
			}
			if facts["command"] != "status" || facts["profile_exists"] != false || facts["bank_authorization_checked"] != false {
				t.Fatalf("status result changed: %v", facts)
			}
			out.Reset()
			if code := Run(context.Background(), []string{"status"}, &out, &diagnostics); code != 2 || out.Len() != 0 {
				t.Fatal("a previous profile flag was reused")
			}
		})
	}
}

type failedCommandOutput struct{}

func (failedCommandOutput) Write([]byte) (int, error) {
	return 0, errors.New("SYNTHETIC-private-output-error")
}

func TestCLIHelpOutputFailureRemainsStatic(t *testing.T) {
	var diagnostics bytes.Buffer
	if code := Run(context.Background(), []string{"--help"}, failedCommandOutput{}, &diagnostics); code != 3 || diagnostics.String() != "The command cannot write the output.\n" {
		t.Fatalf("help output failure exit %d: %q", code, diagnostics.String())
	}
}

func TestCLICommandCatalogHelpAndRejectionDoNotOpenClient(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "absent", "profile.json")
	canary := "SYNTHETIC-private-argument"
	options := Options{OpenClient: func(string) (mcp.Client, error) {
		t.Fatal("help or invalid arguments opened a client")
		return nil, nil
	}}
	for _, definition := range commands {
		t.Run(definition.name, func(t *testing.T) {
			for _, args := range [][]string{
				{definition.name, "--profile", profile, "--help"},
				{"help", definition.name},
			} {
				var output, diagnostics bytes.Buffer
				if code := RunWithOptions(context.Background(), args, &output, &diagnostics, options); code != 0 {
					t.Fatalf("help exit %d: %s", code, diagnostics.String())
				}
				if diagnostics.Len() != 0 || !strings.Contains(output.String(), "Show command help.") || strings.Contains(output.String(), profile) {
					t.Fatal("help was incomplete or disclosed a private path")
				}
			}
			var output, diagnostics bytes.Buffer
			args := []string{definition.name, "--profile", profile, "--password=" + canary}
			if code := RunWithOptions(context.Background(), args, &output, &diagnostics, options); code != 2 || output.Len() != 0 {
				t.Fatalf("invalid arguments exit %d", code)
			}
			if diagnostics.String() != "The command arguments are not valid.\nUse sber --help for command help.\n" {
				t.Fatal("argument diagnostics were not static")
			}
		})
	}
	if _, err := os.Lstat(filepath.Dir(profile)); !os.IsNotExist(err) {
		t.Fatal("help or invalid arguments created profile state")
	}
}

func TestOfflineStatusDoesNotReadOrCreateState(t *testing.T) {
	parent := testPrivateDir(t)
	profile := filepath.Join(parent, "absent-state", "profile.json")
	var out, diagnostics bytes.Buffer
	if code := Run(context.Background(), []string{"status", "--profile", profile}, &out, &diagnostics); code != 0 {
		t.Fatalf("status exit %d: %s", code, diagnostics.String())
	}
	var value map[string]any
	if err := json.Unmarshal(out.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value["profile_exists"] != false || value["bank_authorization_checked"] != false {
		t.Fatal("offline status claimed a bank check")
	}
	if _, err := os.Lstat(filepath.Dir(profile)); !os.IsNotExist(err) {
		t.Fatal("status created state")
	}
}

func TestOfflineInspectOnlyEmitsRedactedMetadata(t *testing.T) {
	dir := testPrivateDir(t)
	profile := filepath.Join(dir, "profile.json")
	canary := "synthetic-cookie-private-only"
	bundle, err := sber.NewSessionBundle(sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin, Cookies: []sber.CookieRecord{{Name: "fixture", Value: canary, Domain: "online.sberbank.ru", Path: "/", Secure: true, HTTPOnly: true, HostOnly: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := bundle.Save(profile); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := Run(context.Background(), []string{"inspect-session", "--profile", profile}, &out, &diagnostics); code != 0 {
		t.Fatalf("inspect exit %d", code)
	}
	if strings.Contains(out.String()+diagnostics.String(), canary) {
		t.Fatal("inspect exposed cookie")
	}
	var data map[string]any
	if err := json.Unmarshal(out.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["command"] != "inspect-session" || data["bank_authorization_checked"] != false {
		t.Fatal("invalid inspection metadata")
	}
	meta, ok := data["metadata"].(map[string]any)
	if !ok || meta["cookies"] != float64(1) || meta["values"] != "<redacted>" {
		t.Fatal("missing redacted metadata")
	}
}

func TestCLIRejectsUnsupportedSecretArgumentsWithoutEcho(t *testing.T) {
	canary := "synthetic-argument-never-print"
	for _, args := range [][]string{{canary}, {"status", "--password=" + canary}, {"status", "--profile", filepath.Join(testPrivateDir(t), "profile.json"), canary}, {"login", "--pin=" + canary}} {
		var out, diagnostics bytes.Buffer
		if code := Run(context.Background(), args, &out, &diagnostics); code != 2 {
			t.Fatalf("unsupported arguments returned %d", code)
		}
		if strings.Contains(out.String()+diagnostics.String(), canary) {
			t.Fatal("invalid arguments echoed sensitive value")
		}
	}
}

func TestOfflineInspectUnsafeProfileFailsClosed(t *testing.T) {
	path := filepath.Join(testPrivateDir(t), "profile.json")
	if err := os.WriteFile(path, []byte(`{"synthetic":"do-not-echo"}`), 0644); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := Run(context.Background(), []string{"inspect-session", "--profile", path}, &out, &diagnostics); code != 3 {
		t.Fatalf("unsafe profile returned %d", code)
	}
	if out.Len() != 0 || strings.Contains(diagnostics.String(), "do-not-echo") {
		t.Fatal("unsafe profile contents printed")
	}
}

func TestOfflineCommandCancelledBeforeStateLookup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diagnostics bytes.Buffer
	if code := Run(ctx, []string{"status", "--profile", "/must-not-inspect"}, &out, &diagnostics); code != 2 {
		t.Fatalf("canceled command returned %d", code)
	}
	if out.Len() != 0 {
		t.Fatal("canceled command emitted metadata")
	}
}

// Profile fixtures must be private independently of the developer's umask.
func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// macOS /var is a symlink. Enrollment intentionally requires literal,
	// symlink-free paths; canonicalize only these generated test fixtures.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
