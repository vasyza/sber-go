package rentalcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNativePreviewCommandNeverEnablesDelivery(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "rental-check")
	build := exec.CommandContext(context.Background(), filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "../../cmd/rental-check")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("preview command unavailable: %v: %s", err, out)
	}
	data, err := json.Marshal(syntheticInput())
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(context.Background(), binary)
	command.Stdin = bytes.NewReader(data)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("preview command failed: %v: %s", err, output)
	}
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		t.Fatal(err)
	}
	if value["reminders_enabled"] != false || value["bank_authorization_checked"] != false {
		t.Fatal("preview claimed live integration")
	}
}
