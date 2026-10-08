package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func TestCredentialsAndCookieSeedReadiness(t *testing.T) {
	credentials, err := NewSberCredentials("synthetic-ufs-session", "synthetic-ufs-token")
	if err != nil {
		t.Fatal(err)
	}
	b, err := credentials.ToBundle(CredentialsBundleOptions{APIBase: "https://web-node2.online.sberbank.ru", WebBase: "https://web2.online.sberbank.ru"})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := b.ToSeed(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed.Cookies.Snapshot()) != 2 {
		t.Fatal("two minimum UFS cookies not created")
	}
	for _, c := range b.Cookies {
		if !c.Secure || !c.HTTPOnly || c.HostOnly || c.Path != "/" || c.Domain != AuthCookieDomain {
			t.Fatal("credentials cookie metadata changed")
		}
	}
	got, err := CredentialsFromBundle(b)
	if err != nil || got.UFSSession != credentials.UFSSession || got.UFSToken != credentials.UFSToken {
		t.Fatal("auth cookies not extracted")
	}
	for _, value := range []any{credentials, seed} {
		raw, _ := json.Marshal(value)
		for _, s := range []string{fmt.Sprintf("%#v", value), string(raw)} {
			if strings.Contains(s, "synthetic-ufs") {
				t.Fatal("credential or seed leaked")
			}
		}
	}
	empty := SessionBundle{APIBase: AppOrigin, WebBase: AppOrigin}
	if _, err := empty.ToSeed(false); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.ToSeed(true); err == nil {
		t.Fatal("empty ready seed accepted")
	}
	b.Cookies[0].HostOnly = true
	b.Cookies[1].HostOnly = true
	if _, err := b.ToSeed(true); err == nil {
		t.Fatal("hostonly cookies incorrectly ready for subdomains")
	}
	if _, err := CredentialsFromBundle(b); err == nil {
		t.Fatal("hostonly cookies treated as cross-subdomain credentials")
	}
	b.Cookies[0].HostOnly = false
	b.Cookies[1].HostOnly = false
	b.Cookies[0].Path = "/api"
	b.Cookies[1].Path = "/api"
	if _, err := b.ToSeed(true); err == nil {
		t.Fatal("warmup-only cookies counted as ready")
	}
}
func TestWithCookieJarPreservesSourceHttpOnlyAndDeletion(t *testing.T) {
	b, _ := NewSessionBundle(syntheticBundle())
	jar, _ := b.ToSeed(false)
	u := mustURL(t, "https://online.sberbank.ru/CSAFront/index.do")
	if err := jar.Cookies.ApplySetCookie(u, []string{"UFS-SESSION=rotated; Domain=online.sberbank.ru; Path=/; Secure; SameSite=Lax"}); err != nil {
		t.Fatal(err)
	}
	updated, err := b.WithCookieJar(jar.Cookies)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Cookies) != 1 || !updated.Cookies[0].HTTPOnly || *updated.Cookies[0].SameSite != "Lax" || updated.CapturedAt == nil {
		t.Fatal("source bundle reconciliation changed")
	}
	if err := jar.Cookies.ApplySetCookie(u, []string{"UFS-SESSION=deleted; Domain=online.sberbank.ru; Path=/; Max-Age=0; Secure"}); err != nil {
		t.Fatal(err)
	}
	updated, err = b.WithCookieJar(jar.Cookies)
	if err != nil || len(updated.Cookies) != 0 {
		t.Fatal("deleted cookie resurrected")
	}
}

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
	path := filepath.Join(testPrivateDir(t), "synthetic-state.json")
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

func TestFoundationSessionEscapedSurrogates(t *testing.T) {
	b, err := NewSessionBundle(syntheticBundle())
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := json.Marshal(b.secretPayload())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSessionBundle(baseline); err != nil {
		t.Fatalf("invalid base fixture: %v", err)
	}
	for _, field := range []string{"synthetic-session-secret", "synthetic-agent-secret", "synthetic-device-secret", "synthetic-antifraud-secret", "2026-01-01T00:00:00+00:00"} {
		for _, escaped := range []string{`\ud800`, `\udfff`, `\ud800x`, `\ud800\ud800`, `\udc00\ud800`, `\ud83d\u0041`} {
			raw := bytes.Replace(baseline, []byte(field), []byte(escaped), 1)
			if !json.Valid(raw) {
				t.Fatal("invalid JSON fixture, not a surrogate repro")
			}
			if _, err := DecodeSessionBundle(raw); err == nil {
				t.Errorf("escaped unpaired surrogate silently rewritten in %s", field)
			}
		}
	}
	for _, raw := range []string{`{"\ud800":"value"}`, `{"unknown":{"key":"\ud800"}}`, `{"nested":[{"key":"\udfff"}]}`} {
		if _, err := decodeUniqueJSON([]byte(raw)); err == nil {
			t.Errorf("private decoder rewrote surrogate in key/nested/unknown data: %s", raw)
		}
	}
	for _, tt := range []struct{ escaped, exact string }{
		{`\ud83d\ude00`, "😀"}, {`\\ud800`, `\ud800`}, {`\\uDFFF`, `\uDFFF`}, {`\ufffd`, "�"},
	} {
		raw := bytes.Replace(baseline, []byte("synthetic-session-secret"), []byte(tt.escaped), 1)
		decoded, err := DecodeSessionBundle(raw)
		if err != nil || len(decoded.Cookies) != 1 || decoded.Cookies[0].Value != tt.exact {
			t.Errorf("valid/literal Unicode did not remain exact: %s (%v)", tt.escaped, err)
		}
	}
	for _, raw := range []string{`{"same":1,"same":2}`, `{"\u0073ame":1,"same":2}`, strings.Repeat("[", 130) + "0" + strings.Repeat("]", 130)} {
		if _, err := decodeUniqueJSON([]byte(raw)); err == nil {
			t.Error("existing duplicate/depth guard weakened")
		}
	}
}

// Profile fixtures must be private independently of the developer's umask.
func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
