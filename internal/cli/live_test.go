package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

func TestNativeReadCommandsUseSelectedClientAndClose(t *testing.T) {
	for _, command := range []string{"products", "accounts", "cards", "operations", "operations-page", "check-session"} {
		t.Run(command, func(t *testing.T) {
			client := &testutil.Client{}
			var out, diagnostics bytes.Buffer
			opened := 0
			code := RunWithOptions(context.Background(), []string{command, "--profile", "explicit-synthetic-path"}, &out, &diagnostics, Options{OpenClient: func(path string) (mcp.Client, error) {
				opened++
				if path != "explicit-synthetic-path" {
					t.Fatal("selected profile lost")
				}
				return client, nil
			}})
			if code != 0 || opened != 1 || client.Closes.Load() != 1 {
				t.Fatalf("command failed or session leaked: %d %s", code, diagnostics.String())
			}
			if strings.Contains(out.String(), "synthetic-private-cookie") || strings.Contains(out.String(), "4111111111111111") {
				t.Fatal("credential or PAN exposed")
			}
			if command == "products" || command == "accounts" {
				if !strings.Contains(out.String(), "9007199254740993.10") {
					t.Fatal("money rounded")
				}
			}
			if command == "operations" && !strings.Contains(out.String(), `"WindowCompleteness": "unknown"`) {
				t.Fatal("history lost unknown coverage")
			}
		})
	}
}

func TestLiveArgumentsFailBeforeSessionConstruction(t *testing.T) {
	for _, extra := range [][]string{{"--password=synthetic-private"}, {"--limit=0"}, {"--from=invalid"}, {"--profile=explicit", "trailing-secret"}} {
		var out, diagnostics bytes.Buffer
		args := append([]string{"products", "--profile", "explicit"}, extra...)
		code := RunWithOptions(context.Background(), args, &out, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { t.Fatal("invalid arguments opened a session"); return nil, nil }})
		if code != 2 || strings.Contains(diagnostics.String(), "synthetic-private") || out.Len() != 0 {
			t.Fatal("invalid input was accepted or echoed")
		}
	}
}

func TestBankFailureNeverReturnsSuccessfulPartialOutput(t *testing.T) {
	client := &testutil.Client{Read: func(context.Context, string, map[string]any) (map[string]any, error) {
		return nil, errors.New("synthetic-private-error")
	}}
	var out, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), []string{"operations", "--profile", "explicit"}, &out, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
	if code != 3 || out.Len() != 0 || client.Closes.Load() != 1 || strings.Contains(diagnostics.String(), "synthetic-private-error") {
		t.Fatal("failure escaped application boundary")
	}
}
