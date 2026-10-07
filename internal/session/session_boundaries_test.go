package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

func TestSessionStrictSchemaAndFileBoundaries(t *testing.T) {
	b, _ := NewSessionBundle(syntheticBundle())
	raw, _ := json.Marshal(b.secretPayload())
	for _, modify := range []func(map[string]any){
		func(m map[string]any) { m["unexpected"] = true }, func(m map[string]any) { delete(m, "captured_at") }, func(m map[string]any) { m["version"] = true },
		func(m map[string]any) { m["version"] = 4.0 + 0.1 }, func(m map[string]any) { m["deviceprint"] = true }, func(m map[string]any) { m["browser"] = map[string]any{"headers": nil} },
		func(m map[string]any) { m["cookies"].([]any)[0].(map[string]any)["secure"] = "true" },
		func(m map[string]any) { delete(m["cookies"].([]any)[0].(map[string]any), "same_site") },
		func(m map[string]any) { m["cookies"].([]any)[0].(map[string]any)["expires"] = 1.5 },
	} {
		var m map[string]any
		json.Unmarshal(raw, &m)
		modify(m)
		s, _ := json.Marshal(m)
		if _, err := DecodeSessionBundle(s); err == nil {
			t.Fatal("invalid field set/type accepted")
		}
	}
	for _, bad := range [][]byte{[]byte(`{"schema":"sber-unofficial-session","schema":"synthetic"}`), []byte(`{"browser":{"headers":[],"headers":[]}}`), []byte(`null`), []byte(string(raw) + ` {}`), append(raw, 0xff), []byte(strings.Repeat(" ", MaxSessionFileBytes+1))} {
		if _, err := DecodeSessionBundle(bad); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	dir := testPrivateDir(t)
	path := filepath.Join(dir, "state.json")
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadSessionBundle(path)
	var insecure *sdkErrs.InsecureSessionFile
	if !errors.As(err, &insecure) {
		t.Fatal("public file accepted")
	}
	if _, err := LoadSessionBundle(path, SessionLoadOptions{AllowNonPrivate: true}); err != nil {
		t.Fatal("explicit unsafe load not honored")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{link, dir} {
		if _, err := LoadSessionBundle(p, SessionLoadOptions{AllowNonPrivate: true}); !errors.As(err, &insecure) {
			t.Fatal("symlink/nonregular accepted")
		}
	}
	if err := b.Save(link); !errors.As(err, &insecure) {
		t.Fatal("save followed symlink")
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSessionBundle(fifo); !errors.As(err, &insecure) {
		t.Fatal("FIFO accepted")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", MaxSessionFileBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	os.Chmod(path, 0600)
	var missing *sdkErrs.MissingSession
	if _, err := LoadSessionBundle(path); !errors.As(err, &missing) {
		t.Fatal("oversize accepted")
	}
	// Owner checks are exercised through actual fstat metadata and a deterministic
	// wrong-owner stat clone when the test process cannot chown files.
	small := filepath.Join(dir, "small.json")
	if err := os.WriteFile(small, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(small)
	st := *info.Sys().(*syscall.Stat_t)
	st.Uid++
	if err := validateSessionStat(statOverride{FileInfo: info, stat: st}, true); !errors.As(err, &insecure) {
		t.Fatal("wrong owner accepted")
	}
}

type statOverride struct {
	os.FileInfo
	stat syscall.Stat_t
}

func (s statOverride) Sys() any { return &s.stat }
