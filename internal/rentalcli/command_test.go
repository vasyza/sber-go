package rentalcli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNativePreviewHelpDoesNotReadLedger(t *testing.T) {
	binary := buildSyntheticPreview(t)
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			code, out, diagnostics := executeSyntheticPreview(t, binary, []byte("SYNTHETIC-not-a-ledger"), args...)
			if code != 0 || diagnostics != "" || !strings.Contains(string(out), "rental-check < explicit-ledger.json") || !strings.Contains(string(out), "Show command help.") {
				t.Fatalf("native help failed: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
			}
			if strings.Contains(string(out), "SYNTHETIC-not-a-ledger") || strings.Contains(string(out), "candidate_decisions") {
				t.Fatal("help disclosed input or evaluated the ledger")
			}
		})
	}
}

func TestPreviewCommandHelpAndCancellationDoNotReadInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		ctx  context.Context
		args []string
		code int
	}{
		{context.Background(), []string{"--help"}, 0},
		{context.Background(), []string{"help"}, 0},
		{context.Background(), []string{"--unknown"}, 2},
		{nil, nil, 2},
		{ctx, nil, 2},
	} {
		input := &countingInput{reader: strings.NewReader("SYNTHETIC-never-read")}
		var out, diagnostics bytes.Buffer
		if code := RunCommand(test.ctx, test.args, input, &out, &diagnostics); code != test.code || input.read != 0 {
			t.Fatalf("help or rejected command consumed input: exit=%d bytes=%d", code, input.read)
		}
		if test.code != 0 && out.Len() != 0 {
			t.Fatal("rejected command emitted preview output")
		}
	}
}

func TestPreviewCommandPreservesFailureExitCodes(t *testing.T) {
	ledger := syntheticProofDocument(t)
	ledger["Tenants"] = nil
	for _, test := range []struct {
		input   io.Reader
		output  io.Writer
		code    int
		message string
	}{
		{nil, io.Discard, 2, "The command context, input, or output is not valid.\n"},
		{strings.NewReader(`{}`), nil, 2, "The command context, input, or output is not valid.\n"},
		{strings.NewReader(`{`), io.Discard, 3, "The preview JSON is not valid.\n"},
		{failingInput{}, io.Discard, 3, "The preview JSON is not valid.\n"},
		{strings.NewReader(`{"AsOf":null}`), io.Discard, 3, "The preview input format is not valid.\n"},
		{bytes.NewReader(documentBytes(t, ledger)), io.Discard, 4, "The rental ledger is not valid.\n"},
		{bytes.NewReader(documentBytes(t, syntheticProofDocument(t))), shortWriter{}, 5, "The command cannot write the preview.\n"},
	} {
		var diagnostics bytes.Buffer
		if code := RunCommand(context.Background(), nil, test.input, test.output, &diagnostics); code != test.code || diagnostics.String() != test.message {
			t.Fatalf("preview error changed at command boundary: exit=%d stderr=%q", code, diagnostics.String())
		}
	}
}

func TestPreviewCommandHelpRejectsShortOutput(t *testing.T) {
	var diagnostics bytes.Buffer
	if code := RunCommand(context.Background(), []string{"--help"}, strings.NewReader(""), shortWriter{}, &diagnostics); code != 5 || diagnostics.String() != "The command cannot write the help text.\n" {
		t.Fatalf("help short write did not fail: exit=%d stderr=%q", code, diagnostics.String())
	}
}

func TestNativePreviewArgumentErrorsRemainStatic(t *testing.T) {
	binary := buildSyntheticPreview(t)
	canary := "SYNTHETIC-private-argument"
	for _, args := range [][]string{
		{canary}, {"--password=" + canary}, {"--help=" + canary},
		{"help", canary}, {"completion"}, {"--", canary},
		{"__complete", "--password=" + canary, ""},
		{"__completeNoDesc", canary, ""},
		{"help", "__complete"}, {"help", "__completeNoDesc"},
	} {
		code, out, diagnostics := executeSyntheticPreview(t, binary, []byte("SYNTHETIC-not-a-ledger"), args...)
		if code != 2 || len(out) != 0 || diagnostics != "The command arguments are not valid.\nUse rental-check --help for command help.\n" {
			t.Fatalf("native argument rejection changed or disclosed input: exit=%d stdout=%q stderr=%q", code, out, diagnostics)
		}
	}
}

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
		if code != 3 || len(out) != 0 || diagnostics != "The preview input format is not valid.\n" {
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
	if code != 2 || len(out) != 0 || diagnostics != "The command arguments are not valid.\nUse rental-check --help for command help.\n" {
		t.Fatal("native argument rejection disclosed argument or emitted decisions")
	}
}
