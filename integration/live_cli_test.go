//go:build live

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestLiveCLIReadOnly exercises the actual command binary. The opt-in guard
// precedes file access and builds. All bank data stays in transient memory.
func TestLiveCLIReadOnly(t *testing.T) {
	if !*liveEnabled {
		t.Skip("requires explicit -sber-live after owner login")
	}
	if !filepath.IsAbs(*liveProfile) {
		t.Fatal("requires an explicit absolute private profile path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "sber")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "../cmd/sber")
	if build.Run() != nil {
		t.Fatal("cannot build the native CLI")
	}
	read := func(name string, arguments ...string) json.RawMessage {
		args := []string{name, "--profile", *liveProfile}
		switch name {
		case "status", "inspect-session", "inspect-credentials", "export-session":
		default:
			args = append(args, "--no-renew")
		}
		if *liveCA != "" && name != "status" && name != "inspect-session" {
			args = append(args, "--ca-bundle", *liveCA)
		}
		args = append(args, arguments...)
		// Do not expose diagnostics or process arguments: these contain owner paths.
		result, err := exec.CommandContext(ctx, binary, args...).Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				// Only fixed public CLI diagnostics can enter test output.
				message := strings.TrimSpace(string(exit.Stderr))
				for _, prefix := range []string{"session expired;", "bank rejected the request;", "bank returned HTTP ", "bank request timed out;", "cannot open private session", "session cleanup failed", "bank response format is not supported;", "bank request failed;", "destination already exists;", "cannot serialize result", "output failed"} {
					if strings.HasPrefix(message, prefix) {
						t.Fatalf("native %s failed: exit=%d classification=%s", name, exit.ExitCode(), strings.TrimSuffix(prefix, ";"))
					}
				}
				t.Fatalf("native %s failed: exit=%d; diagnostics withheld", name, exit.ExitCode())
			}
			t.Fatalf("native %s could not complete", name)
		}
		if !json.Valid(result) {
			t.Fatalf("native %s returned invalid JSON", name)
		}
		t.Logf("native %s passed", name)
		return result
	}
	read("check-session")
	read("status")
	read("inspect-session")
	read("inspect-credentials")
	var products struct {
		Accounts []json.RawMessage `json:"accounts"`
		Cards    []struct {
			ID string `json:"id"`
		} `json:"cards"`
	}
	if json.Unmarshal(read("products"), &products) != nil {
		t.Fatal("cannot decode products")
	}
	read("accounts")
	read("cards")
	read("portfolio")
	validCard := regexp.MustCompile(`^[0-9]{1,16}$`)
	if len(products.Cards) > 0 {
		selected := ""
		for _, card := range products.Cards {
			if validCard.MatchString(card.ID) {
				selected = card.ID
				break
			}
		}
		if selected == "" {
			t.Fatal("no supported numeric product ID in returned cards")
		}
		read("card-info", "--card-id", selected)
		read("card-limits", "--card-id", selected)
	} else {
		t.Log("card endpoint verification unavailable: no cards")
	}
	now := time.Now().In(time.FixedZone("Moscow", 3*60*60))
	from, to := now.AddDate(0, 0, -7).Format(time.DateOnly), now.Format(time.DateOnly)
	var page struct {
		Operations []struct {
			ID string `json:"id"`
		} `json:"operations"`
	}
	if json.Unmarshal(read("operations-page", "--from", from, "--to", to, "--limit", "5"), &page) != nil {
		t.Fatal("cannot decode history page")
	}
	if len(page.Operations) > 5 {
		t.Fatal("history exceeded the requested bound")
	}
	if len(page.Operations) > 0 {
		read("operation-details", "--operation-id", page.Operations[0].ID)
	} else {
		t.Log("operation details verification unavailable: empty history page")
	}
	read("operations", "--from", from, "--to", to, "--limit", "100", "--max-pages", "2")
	read("analytics", "--from", from, "--to", to)
	exportDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("cannot determine private export test directory")
	}
	if os.Chmod(exportDir, 0700) != nil {
		t.Fatal("cannot make the export test directory private")
	}
	read("export-session", "--destination", filepath.Join(exportDir, "copy.json"))
	t.Logf("native read checks complete: accounts=%d cards=%d operations=%d; full history coverage remains unknown", len(products.Accounts), len(products.Cards), len(page.Operations))
}
