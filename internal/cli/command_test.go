package cli

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
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
}
