package sber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func stringPtr(s string) *string { return &s }
func int64Ptr(v int64) *int64    { return &v }
func syntheticBundle() SessionBundle {
	return SessionBundle{APIBase: "https://web-node2.online.sberbank.ru", WebBase: "https://web2.online.sberbank.ru", SchemaVersion: 4,
		Cookies:     []CookieRecord{{Name: "UFS-SESSION", Value: "synthetic-session-secret", Domain: ".online.sberbank.ru", Path: "/", Secure: true, HTTPOnly: true, HostOnly: false, Expires: int64Ptr(4102444800), SameSite: stringPtr("Strict")}},
		Browser:     BrowserProfile{Headers: []BrowserHeader{{Name: "User-Agent", Value: "synthetic-agent-secret"}}},
		Deviceprint: stringPtr("synthetic-device-secret"), AntifraudDeviceprint: stringPtr("synthetic-antifraud-secret"), CapturedAt: stringPtr("2026-01-01T00:00:00+00:00"),
	}
}
func TestSessionValidationAndRedaction(t *testing.T) {
	b, err := NewSessionBundle(syntheticBundle())
	if err != nil {
		t.Fatal(err)
	}
	if b.Cookies[0].Domain != "online.sberbank.ru" || b.Browser.Headers[0].Name != "user-agent" {
		t.Fatal("source normalization missing")
	}
	vals := []any{b, b.Cookies[0], b.Browser, BrowserHeader{Name: "referer", Value: "synthetic-process-secret"}}
	for _, v := range vals {
		j, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{string(j), fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v)} {
			for _, secret := range []string{"synthetic-session-secret", "synthetic-agent-secret", "synthetic-device-secret", "synthetic-antifraud-secret", "synthetic-process-secret"} {
				if strings.Contains(s, secret) {
					t.Fatal("representation leaked secret")
				}
			}
		}
	}
	// Normal JSON is deliberately redacted, not an implicit export API.
	j, _ := json.Marshal(b)
	if !bytes.Contains(j, []byte("<redacted>")) && !bytes.Contains(j, []byte("\\u003credacted\\u003e")) {
		t.Fatal("redaction marker absent")
	}
	for _, bad := range []string{"http://online.sberbank.ru", "https://outside.invalid", "https://online.sberbank.ru:444", "https://user:pass@online.sberbank.ru"} {
		c := syntheticBundle()
		c.APIBase = bad
		if _, err := NewSessionBundle(c); err == nil {
			t.Fatal("unsafe session base accepted")
		}
	}
	c := syntheticBundle()
	c.Cookies = append(c.Cookies, c.Cookies[0])
	if _, err := NewSessionBundle(c); err == nil {
		t.Fatal("duplicate scope accepted")
	}
	c = syntheticBundle()
	c.Deviceprint = stringPtr("synthetic\nsecret")
	if _, err := NewSessionBundle(c); err == nil {
		t.Fatal("control deviceprint accepted")
	}
	c = syntheticBundle()
	c.Browser.Headers = append(c.Browser.Headers, BrowserHeader{Name: "Cookie", Value: "opaque"})
	if _, err := NewSessionBundle(c); err == nil {
		t.Fatal("unsafe observed header accepted")
	}
	for _, mutate := range []func(*CookieRecord){
		func(c *CookieRecord) { c.Value = "opaque;injected" }, func(c *CookieRecord) { c.Name = "bad name" }, func(c *CookieRecord) { c.Domain = "evil.invalid" },
		func(c *CookieRecord) { c.Path = "bad" }, func(c *CookieRecord) { c.Expires = int64Ptr(0) }, func(c *CookieRecord) { c.SameSite = stringPtr("STRICT") },
		func(c *CookieRecord) { c.Name = "__Secure-fixture"; c.Secure = false }, func(c *CookieRecord) { c.Name = "__Host-fixture"; c.HostOnly = false },
	} {
		c := syntheticBundle()
		mutate(&c.Cookies[0])
		if _, err := NewSessionBundle(c); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
}

func TestSessionPrivatePersistenceAndLegacy(t *testing.T) {
	original, err := NewSessionBundle(syntheticBundle())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic-state.json")
	if err := original.Save(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("save was not private")
	}
	restored, err := LoadSessionBundle(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatal("roundtrip lost metadata")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 9 || raw["schema"] != SessionSchema || raw["version"] != float64(4) {
		t.Fatal("Python v4 top-level contract differs")
	}
	cookies := raw["cookies"].([]any)
	if len(cookies[0].(map[string]any)) != 9 {
		t.Fatal("Python cookie field contract differs")
	}
	for _, version := range []int{1, 2, 3} {
		var legacy map[string]any
		json.Unmarshal(data, &legacy)
		legacy["version"] = version
		if version == 1 {
			delete(legacy, "deviceprint")
		}
		if version < 3 {
			delete(legacy, "antifraud_deviceprint")
		}
		for _, c := range legacy["cookies"].([]any) {
			delete(c.(map[string]any), "same_site")
		}
		old, _ := json.Marshal(legacy)
		if err := os.WriteFile(path, old, 0600); err != nil {
			t.Fatal(err)
		}
		migrated, err := LoadSessionBundle(path)
		if err != nil {
			t.Fatalf("legacy %d: %v", version, err)
		}
		if migrated.SchemaVersion != 4 || migrated.Cookies[0].SameSite != nil {
			t.Fatal("legacy metadata/version not migrated")
		}
		if version == 1 && migrated.Deviceprint != nil || version < 3 && migrated.AntifraudDeviceprint != nil {
			t.Fatal("legacy fields invented")
		}
		if err := migrated.Save(path); err != nil {
			t.Fatal(err)
		}
	}
}
