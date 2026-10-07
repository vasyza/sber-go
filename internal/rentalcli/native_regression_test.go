package rentalcli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func buildSyntheticPreview(t testing.TB) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "rental-check")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "../../cmd/rental-check")
	command.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=sum.golang.org", "GOWORK=off")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native synthetic preview build failed: %v: %s", err, output)
	}
	return binary
}

func executeSyntheticPreview(t testing.TB, binary string, data []byte, args ...string) (int, []byte, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdin = bytes.NewReader(data)
	var out, diagnostics bytes.Buffer
	command.Stdout, command.Stderr = &out, &diagnostics
	err := command.Run()
	code := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok || ctx.Err() != nil {
			t.Fatalf("native subprocess unavailable: %v", err)
		}
		code = exit.ExitCode()
	}
	return code, out.Bytes(), diagnostics.String()
}

func TestNativePreviewPreservesExactCutoffDecisions(t *testing.T) {
	binary := buildSyntheticPreview(t)
	for _, test := range syntheticCutoffCases() {
		t.Run(test.name, func(t *testing.T) {
			document := cutoffDocument(t, test)
			code, out, diagnostics := executeSyntheticPreview(t, binary, documentBytes(t, document))
			if code != 0 || diagnostics != "" {
				t.Fatalf("native cutoff failed: exit=%d stderr=%q", code, diagnostics)
			}
			assertCutoffDecision(t, test, document, decodeDecisions(t, out))
		})
	}
}

func TestNativePreviewRejectsUnsupportedTimestampAndMissingProofBeforeOutput(t *testing.T) {
	binary := buildSyntheticPreview(t)
	check := func(t *testing.T, document map[string]any) {
		t.Helper()
		code, out, diagnostics := executeSyntheticPreview(t, binary, documentBytes(t, document))
		if code != 3 || len(out) != 0 || diagnostics != "invalid preview schema\n" {
			t.Fatalf("native schema failure not static/decision-free: exit=%d stdout=%s stderr=%q", code, out, diagnostics)
		}
	}
	for position := range 9 {
		document := syntheticProofDocument(t)
		field := timestampPositions(document)[position]
		t.Run("precision/"+field.name, func(t *testing.T) {
			field.object[field.key] = strings.TrimSuffix(field.object[field.key].(string), "Z") + ".0000000001Z"
			check(t, document)
		})
	}
	for _, value := range []string{"2026-03-01T0:00:00Z", "2026-03-01T00:00:00,1Z", "2026-03-01T00:00:00+00:60", "2026-03-01T00:00:00+24:00"} {
		t.Run("grammar/"+value, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document["AsOf"] = value
			check(t, document)
		})
	}
	for _, field := range []string{"HasGaps", "Truncated", "PageUncertain"} {
		t.Run("proof-presence/"+field, func(t *testing.T) {
			document := syntheticProofDocument(t)
			document["Receipts"] = nil
			delete(firstObject(document, "Evidence"), field)
			check(t, document)
		})
	}
	code, out, diagnostics := executeSyntheticPreview(t, binary, nil, "SYNTHETIC-never-echo")
	if code != 2 || len(out) != 0 || diagnostics != "usage: rental-check < explicit-ledger.json\n" {
		t.Fatal("native argument rejection disclosed argument or emitted decisions")
	}
}
