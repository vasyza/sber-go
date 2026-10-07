package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
