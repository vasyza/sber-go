package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/vasyza/sber-go"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineStatusDoesNotReadOrCreateState(t *testing.T) {
	parent := t.TempDir()
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
	dir := t.TempDir()
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
	for _, args := range [][]string{{canary}, {"status", "--password=" + canary}, {"status", "--profile", filepath.Join(t.TempDir(), "profile.json"), canary}, {"login", "--pin=" + canary}} {
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
	path := filepath.Join(t.TempDir(), "profile.json")
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
