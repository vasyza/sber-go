package rentalcli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
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
