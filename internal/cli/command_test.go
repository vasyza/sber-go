package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeCommandOfflineStatus(t *testing.T) {
	dir := t.TempDir()
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
