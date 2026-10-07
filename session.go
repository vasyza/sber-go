package sber

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const (
	SessionSchema        = "sber-unofficial-session"
	SessionSchemaVersion = 4
	MaxSessionFileBytes  = 2 * 1024 * 1024
	MaxBrowserHeaders    = 32
	AppOrigin            = "https://online.sberbank.ru"
	AuthCookieDomain     = "online.sberbank.ru"
)

// BrowserHeader and BrowserProfile carry only allowlisted observed headers.
// Values are used exactly, not fabricated to impersonate another browser.
type BrowserHeader struct{ Name, Value string }
type BrowserProfile struct{ Headers []BrowserHeader }

var observedHeaderNames = map[string]bool{
	"accept": true, "accept-language": true, "content-type": true, "origin": true, "priority": true, "referer": true, "user-agent": true,
	"sec-ch-ua": true, "sec-ch-ua-mobile": true, "sec-ch-ua-platform": true, "sec-fetch-dest": true, "sec-fetch-mode": true, "sec-fetch-site": true, "x-requested-with": true,
}

func NewBrowserProfile(p BrowserProfile) (BrowserProfile, error) {
	if len(p.Headers) > MaxBrowserHeaders {
		return BrowserProfile{}, &MissingSession{Message: "too many browser headers"}
	}
	result := BrowserProfile{Headers: make([]BrowserHeader, 0, len(p.Headers))}
	seen := map[string]bool{}
	for _, h := range p.Headers {
		h.Name = strings.ToLower(h.Name)
		if !observedHeaderNames[h.Name] || seen[h.Name] || strings.ContainsAny(h.Value, "\r\n") || utf8.RuneCountInString(h.Value) > 8192 || !utf8.ValidString(h.Value) {
			return BrowserProfile{}, &MissingSession{Message: "invalid browser header"}
		}
		seen[h.Name] = true
		result.Headers = append(result.Headers, h)
	}
	return result, nil
}
func (p BrowserProfile) AsMap() map[string]string {
	m := make(map[string]string, len(p.Headers))
	for _, h := range p.Headers {
		m[h.Name] = h.Value
	}
	return m
}
func (p BrowserProfile) Validate() error            { _, err := NewBrowserProfile(p); return err }
func (p BrowserProfile) String() string             { return "BrowserProfile(<redacted>)" }
func (p BrowserProfile) GoString() string           { return p.String() }
func (p BrowserProfile) Format(f fmt.State, v rune) { formatError(f, p.String()) }
func (p BrowserProfile) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"headers": "<redacted>"})
}
func (h BrowserHeader) String() string             { return "BrowserHeader(<redacted>)" }
func (h BrowserHeader) GoString() string           { return h.String() }
func (h BrowserHeader) Format(f fmt.State, v rune) { formatError(f, h.String()) }
func (h BrowserHeader) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"value": "<redacted>"})
}

// SessionBundle is a native v4-compatible portable profile. Constructors and
// boundary methods copy slices and pointer values to avoid implicit sharing.
// JSON marshaling is REDACTED; Save is the only secret persistence operation.
type SessionBundle struct {
	APIBase, WebBase                              string
	Cookies                                       []CookieRecord
	Browser                                       BrowserProfile
	Deviceprint, AntifraudDeviceprint, CapturedAt *string
	SchemaVersion                                 int
}

func IsOnlineURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return false
	}
	port := u.Port()
	if port != "" && port != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "online.sberbank.ru" || strings.HasSuffix(host, ".online.sberbank.ru")
}
func cloneString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}
func validateDeviceprint(p *string) error {
	if p != nil && (!utf8.ValidString(*p) || utf8.RuneCountInString(*p) < 1 || utf8.RuneCountInString(*p) > 64*1024 || hasControls(*p)) {
		return &MissingSession{Message: "invalid deviceprint"}
	}
	return nil
}
func NewSessionBundle(b SessionBundle) (SessionBundle, error) {
	if b.SchemaVersion == 0 {
		b.SchemaVersion = SessionSchemaVersion
	}
	if b.SchemaVersion != SessionSchemaVersion || !IsOnlineURL(b.APIBase) || !IsOnlineURL(b.WebBase) || len(b.Cookies) > MaxCookies {
		return SessionBundle{}, &MissingSession{Message: "invalid session metadata"}
	}
	if err := validateDeviceprint(b.Deviceprint); err != nil {
		return SessionBundle{}, err
	}
	if err := validateDeviceprint(b.AntifraudDeviceprint); err != nil {
		return SessionBundle{}, err
	}
	p, err := NewBrowserProfile(b.Browser)
	if err != nil {
		return SessionBundle{}, err
	}
	b.Browser = p
	records := make([]CookieRecord, 0, len(b.Cookies))
	seen := map[string]bool{}
	for _, c := range b.Cookies {
		c, err = NewCookieRecord(c)
		if err != nil {
			return SessionBundle{}, err
		}
		key := c.Name + "\x00" + c.Domain + "\x00" + c.Path
		if seen[key] {
			return SessionBundle{}, &MissingSession{Message: "duplicate cookie scope"}
		}
		seen[key] = true
		records = append(records, c)
	}
	b.Cookies = records
	b.APIBase = strings.TrimRight(b.APIBase, "/")
	b.WebBase = strings.TrimRight(b.WebBase, "/")
	b.Deviceprint = cloneString(b.Deviceprint)
	b.AntifraudDeviceprint = cloneString(b.AntifraudDeviceprint)
	b.CapturedAt = cloneString(b.CapturedAt)
	return b, nil
}
func (b SessionBundle) Validate() error               { _, err := NewSessionBundle(b); return err }
func (b SessionBundle) Clone() (SessionBundle, error) { return NewSessionBundle(b) }
func (b SessionBundle) String() string {
	return fmt.Sprintf("SessionBundle(cookies=%d, <redacted>)", len(b.Cookies))
}
func (b SessionBundle) GoString() string             { return b.String() }
func (b SessionBundle) Format(f fmt.State, v rune)   { formatError(f, b.String()) }
func (b SessionBundle) MarshalJSON() ([]byte, error) { return json.Marshal(b.Redacted()) }
func (b SessionBundle) Redacted() map[string]any {
	present := func(v *string) any {
		if v == nil {
			return nil
		}
		return "<present>"
	}
	return map[string]any{"schema": SessionSchema, "version": SessionSchemaVersion, "cookies": len(b.Cookies), "values": "<redacted>", "browser": "<redacted>", "deviceprint": present(b.Deviceprint), "antifraud_deviceprint": present(b.AntifraudDeviceprint)}
}

// SessionLoadOptions is explicitly opt-in for non-private files; symlinks,
// non-regular files, malformed/oversized JSON are always rejected.
type SessionLoadOptions struct{ AllowNonPrivate bool }

func validateSessionStat(info os.FileInfo, private bool) error {
	if !info.Mode().IsRegular() {
		return &InsecureSessionFile{}
	}
	if info.Size() > MaxSessionFileBytes {
		return &MissingSession{}
	}
	if private {
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || uint32(os.Getuid()) != st.Uid || info.Mode().Perm()&0077 != 0 {
			return &InsecureSessionFile{}
		}
	}
	return nil
}

func readSessionFile(path string, private bool) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errorsIsNotExist(err) {
			return nil, &MissingSession{}
		}
		return nil, &InsecureSessionFile{}
	}
	f := os.NewFile(uintptr(fd), "private-session")
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, &InsecureSessionFile{}
	}
	if err = validateSessionStat(info, private); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxSessionFileBytes+1))
	if err != nil || len(raw) > MaxSessionFileBytes {
		return nil, &MissingSession{}
	}
	return raw, nil
}
func errorsIsNotExist(err error) bool { return os.IsNotExist(err) }

// LoadSessionBundle strictly imports the exact Python v1-v4 field contracts and
// normalizes them to v4; no unknown fields, duplicate keys or lossy coercions.
func LoadSessionBundle(path string, options ...SessionLoadOptions) (SessionBundle, error) {
	if len(options) > 1 {
		return SessionBundle{}, &MissingSession{}
	}
	private := true
	if len(options) == 1 {
		private = !options[0].AllowNonPrivate
	}
	raw, err := readSessionFile(path, private)
	if err != nil {
		return SessionBundle{}, err
	}
	return DecodeSessionBundle(raw)
}

// DecodeSessionBundle accepts sensitive input in memory, with the same strict
// schema/size validation used by the private-file loader.
func DecodeSessionBundle(raw []byte) (SessionBundle, error) {
	invalid := func() (SessionBundle, error) { return SessionBundle{}, &MissingSession{} }
	if len(raw) > MaxSessionFileBytes || !utf8.Valid(raw) {
		return invalid()
	}
	value, err := decodeUniqueJSON(raw)
	if err != nil {
		return invalid()
	}
	p, ok := value.(map[string]any)
	if !ok || p["schema"] != SessionSchema {
		return invalid()
	}
	version, ok := jsonInteger(p["version"])
	if !ok || version < 1 || version > 4 {
		return invalid()
	}
	keys := []string{"schema", "version", "api_base", "web_base", "captured_at", "browser", "cookies"}
	if version >= 2 {
		keys = append(keys, "deviceprint")
	}
	if version >= 3 {
		keys = append(keys, "antifraud_deviceprint")
	}
	if !exactJSONKeys(p, keys...) {
		return invalid()
	}
	b := SessionBundle{SchemaVersion: 4}
	if b.APIBase, ok = p["api_base"].(string); !ok {
		return invalid()
	}
	if b.WebBase, ok = p["web_base"].(string); !ok {
		return invalid()
	}
	if b.CapturedAt, ok = jsonNullableString(p["captured_at"]); !ok {
		return invalid()
	}
	if version >= 2 {
		if b.Deviceprint, ok = jsonNullableString(p["deviceprint"]); !ok {
			return invalid()
		}
	}
	if version >= 3 {
		if b.AntifraudDeviceprint, ok = jsonNullableString(p["antifraud_deviceprint"]); !ok {
			return invalid()
		}
	}
	browser, ok := p["browser"].(map[string]any)
	if !ok || !exactJSONKeys(browser, "headers") {
		return invalid()
	}
	headers, ok := browser["headers"].([]any)
	if !ok {
		return invalid()
	}
	for _, value := range headers {
		h, ok := value.(map[string]any)
		if !ok || !exactJSONKeys(h, "name", "value") {
			return invalid()
		}
		name, ok := h["name"].(string)
		if !ok {
			return invalid()
		}
		v, ok := h["value"].(string)
		if !ok {
			return invalid()
		}
		b.Browser.Headers = append(b.Browser.Headers, BrowserHeader{name, v})
	}
	cookies, ok := p["cookies"].([]any)
	if !ok {
		return invalid()
	}
	for _, value := range cookies {
		c, ok := value.(map[string]any)
		ck := []string{"name", "value", "domain", "path", "secure", "http_only", "host_only", "expires"}
		if version == 4 {
			ck = append(ck, "same_site")
		}
		if !ok || !exactJSONKeys(c, ck...) {
			return invalid()
		}
		r := CookieRecord{}
		if r.Name, ok = c["name"].(string); !ok {
			return invalid()
		}
		if r.Value, ok = c["value"].(string); !ok {
			return invalid()
		}
		if r.Domain, ok = c["domain"].(string); !ok {
			return invalid()
		}
		if r.Path, ok = c["path"].(string); !ok {
			return invalid()
		}
		if r.Secure, ok = c["secure"].(bool); !ok {
			return invalid()
		}
		if r.HTTPOnly, ok = c["http_only"].(bool); !ok {
			return invalid()
		}
		if r.HostOnly, ok = c["host_only"].(bool); !ok {
			return invalid()
		}
		if c["expires"] != nil {
			n, ok := jsonInteger(c["expires"])
			if !ok {
				return invalid()
			}
			r.Expires = &n
		}
		if version == 4 {
			if r.SameSite, ok = jsonNullableString(c["same_site"]); !ok {
				return invalid()
			}
		}
		b.Cookies = append(b.Cookies, r)
	}
	return NewSessionBundle(b)
}

func exactJSONKeys(p map[string]any, keys ...string) bool {
	if len(p) != len(keys) {
		return false
	}
	for _, k := range keys {
		if _, ok := p[k]; !ok {
			return false
		}
	}
	return true
}
func jsonNullableString(value any) (*string, bool) {
	if value == nil {
		return nil, true
	}
	s, ok := value.(string)
	return &s, ok
}
func jsonInteger(value any) (int64, bool) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(string(n), 10, 64)
	return i, err == nil
}

// validSessionJSONEscapes checks UTF-16 escapes before encoding/json can replace
// unpaired surrogates with U+FFFD. Escaped backslashes are consumed as a pair,
// so a literal backslash-u sequence is not mistaken for a Unicode escape.
func validSessionJSONEscapes(raw []byte) bool {
	inString := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+5 > len(raw) {
			return false
		}
		unit, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil || unit >= 0xdc00 && unit <= 0xdfff {
			return false
		}
		i += 4
		if unit < 0xd800 || unit > 0xdbff {
			continue
		}
		if i+7 > len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}

func decodeUniqueJSON(raw []byte) (any, error) {
	if !validSessionJSONEscapes(raw) {
		return nil, &MissingSession{}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := jsonValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, &MissingSession{}
	}
	return value, nil
}
func jsonValue(d *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, &MissingSession{}
	}
	tok, err := d.Token()
	if err != nil {
		return nil, &MissingSession{}
	}
	switch delim := tok.(type) {
	case json.Delim:
		switch delim {
		case '{':
			obj := map[string]any{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return nil, &MissingSession{}
				}
				s, ok := k.(string)
				if !ok {
					return nil, &MissingSession{}
				}
				if _, exists := obj[s]; exists {
					return nil, &MissingSession{}
				}
				v, err := jsonValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				obj[s] = v
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return nil, &MissingSession{}
			}
			return obj, nil
		case '[':
			list := make([]any, 0)
			for d.More() {
				v, err := jsonValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				list = append(list, v)
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return nil, &MissingSession{}
			}
			return list, nil
		default:
			return nil, &MissingSession{}
		}
	default:
		return tok, nil
	}
}

func (b SessionBundle) secretPayload() map[string]any {
	headers := make([]any, 0, len(b.Browser.Headers))
	for _, h := range b.Browser.Headers {
		headers = append(headers, map[string]any{"name": h.Name, "value": h.Value})
	}
	cookies := make([]any, 0, len(b.Cookies))
	for _, c := range b.Cookies {
		cookies = append(cookies, map[string]any{"name": c.Name, "value": c.Value, "domain": c.Domain, "path": c.Path, "secure": c.Secure, "http_only": c.HTTPOnly, "host_only": c.HostOnly, "expires": c.Expires, "same_site": c.SameSite})
	}
	return map[string]any{"schema": SessionSchema, "version": 4, "api_base": b.APIBase, "web_base": b.WebBase, "captured_at": b.CapturedAt, "browser": map[string]any{"headers": headers}, "deviceprint": b.Deviceprint, "antifraud_deviceprint": b.AntifraudDeviceprint, "cookies": cookies}
}

// Save atomically writes the exact v4 secret payload with mode 0600 and fsync.
// The parent must be an owner-owned private directory, not a symlink. Its open
// descriptor anchors all directory operations. Linux procfs is required to link
// the validated open inode into a private publication directory before rename.
// Directory privacy is a trust boundary, not isolation from hostile same-UID
// processes (which can also read memory/change permissions). Such writers must
// not mutate the private publication directory. Existing regular profiles can
// still be overwritten/renewed; symlink/non-regular targets are never followed.
func (b SessionBundle) Save(path string) error {
	b, err := NewSessionBundle(b)
	if err != nil {
		return err
	}
	dfd, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &InsecureSessionFile{}
	}
	parent := os.NewFile(uintptr(dfd), "private-session-directory")
	defer parent.Close()
	dirInfo, err := parent.Stat()
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm()&0077 != 0 {
		return &InsecureSessionFile{}
	}
	owner, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		return &InsecureSessionFile{}
	}
	name := filepath.Base(path)
	checkTarget := func() error {
		var target unix.Stat_t
		err := unix.Fstatat(dfd, name, &target, unix.AT_SYMLINK_NOFOLLOW)
		if err == unix.ENOENT {
			return nil
		}
		if err != nil || target.Mode&unix.S_IFMT != unix.S_IFREG {
			return &InsecureSessionFile{}
		}
		return nil
	}
	if err := checkTarget(); err != nil {
		return err
	}
	anchored := "/proc/self/fd/" + strconv.Itoa(dfd)
	f, err := os.CreateTemp(anchored, ".sber-session-*.tmp")
	if err != nil {
		return &InsecureSessionFile{}
	}
	tmp := filepath.Base(f.Name())
	defer unix.Unlinkat(dfd, tmp, 0)
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return &InsecureSessionFile{}
	}
	raw, err := json.MarshalIndent(b.secretPayload(), "", "  ")
	if err != nil || len(raw)+1 > MaxSessionFileBytes {
		return &MissingSession{}
	}
	raw = append(raw, '\n')
	if _, err = f.Write(raw); err != nil {
		return &InsecureSessionFile{}
	}
	if err = f.Sync(); err != nil {
		return &InsecureSessionFile{}
	}
	info, err := f.Stat()
	if err != nil {
		return &InsecureSessionFile{}
	}
	if err = validateSessionStat(info, true); err != nil {
		return err
	}
	validated, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return &InsecureSessionFile{}
	}
	// Reject an observed source exchange. This check is NOT the publication
	// guarantee: the subsequent link uses the open descriptor, never this name.
	var source unix.Stat_t
	if err = unix.Fstatat(dfd, tmp, &source, unix.AT_SYMLINK_NOFOLLOW); err != nil || source.Mode&unix.S_IFMT != unix.S_IFREG || source.Dev != validated.Dev || source.Ino != validated.Ino {
		return &InsecureSessionFile{}
	}
	stagePath, err := os.MkdirTemp(anchored, ".sber-publish-")
	if err != nil {
		return &InsecureSessionFile{}
	}
	stageName := filepath.Base(stagePath)
	defer unix.Unlinkat(dfd, stageName, unix.AT_REMOVEDIR)
	stageFD, err := unix.Openat(dfd, stageName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &InsecureSessionFile{}
	}
	stage := os.NewFile(uintptr(stageFD), "private-session-publication")
	defer stage.Close()
	defer unix.Unlinkat(stageFD, "payload", 0)
	stageInfo, err := stage.Stat()
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode().Perm()&0077 != 0 {
		return &InsecureSessionFile{}
	}
	stageOwner, ok := stageInfo.Sys().(*syscall.Stat_t)
	if !ok || stageOwner.Uid != uint32(os.Getuid()) {
		return &InsecureSessionFile{}
	}
	if err = unix.Linkat(unix.AT_FDCWD, "/proc/self/fd/"+strconv.Itoa(int(f.Fd())), stageFD, "payload", unix.AT_SYMLINK_FOLLOW); err != nil {
		return &InsecureSessionFile{}
	}
	var linked unix.Stat_t
	if err = unix.Fstatat(stageFD, "payload", &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil || linked.Mode&unix.S_IFMT != unix.S_IFREG || linked.Dev != validated.Dev || linked.Ino != validated.Ino {
		return &InsecureSessionFile{}
	}
	if err = unix.Unlinkat(dfd, tmp, 0); err != nil {
		return &InsecureSessionFile{}
	}
	if err = f.Close(); err != nil {
		return &InsecureSessionFile{}
	}
	if err = stage.Sync(); err != nil {
		return &InsecureSessionFile{}
	}
	if err = checkTarget(); err != nil {
		return err
	}
	if err = unix.Renameat(stageFD, "payload", dfd, name); err != nil {
		return &InsecureSessionFile{}
	}
	if err = unix.Unlinkat(dfd, stageName, unix.AT_REMOVEDIR); err != nil {
		return &InsecureSessionFile{}
	}
	if err = parent.Sync(); err != nil {
		return &InsecureSessionFile{}
	}
	return nil
}

// SberCredentials is the observed minimum pair, not an OAuth token. It has no
// implicit JSON secret export and never expands the underlying privilege scope.
type SberCredentials struct{ UFSSession, UFSToken string }

func NewSberCredentials(session, token string) (SberCredentials, error) {
	c := SberCredentials{session, token}
	for _, item := range []struct{ name, value string }{{"UFS-SESSION", session}, {"UFS-TOKEN", token}} {
		if item.value == "" {
			return SberCredentials{}, &MissingSession{}
		}
		if _, err := NewCookieRecord(CookieRecord{Name: item.name, Value: item.value, Domain: AuthCookieDomain, Path: "/", Secure: true, HTTPOnly: true}); err != nil {
			return SberCredentials{}, err
		}
	}
	return c, nil
}
func CredentialsFromBundle(b SessionBundle) (SberCredentials, error) {
	b, err := NewSessionBundle(b)
	if err != nil {
		return SberCredentials{}, err
	}
	values := map[string]string{}
	for _, c := range b.Cookies {
		if (c.Name == "UFS-SESSION" || c.Name == "UFS-TOKEN") && c.Domain == AuthCookieDomain && c.Path == "/" && c.Secure && !c.HostOnly {
			if _, exists := values[c.Name]; exists {
				return SberCredentials{}, &MissingSession{}
			}
			values[c.Name] = c.Value
		}
	}
	return NewSberCredentials(values["UFS-SESSION"], values["UFS-TOKEN"])
}

type CredentialsBundleOptions struct {
	APIBase, WebBase                  string
	Browser                           BrowserProfile
	Deviceprint, AntifraudDeviceprint *string
}

func (c SberCredentials) ToBundle(o CredentialsBundleOptions) (SessionBundle, error) {
	if _, err := NewSberCredentials(c.UFSSession, c.UFSToken); err != nil {
		return SessionBundle{}, err
	}
	if o.APIBase == "" {
		o.APIBase = AppOrigin
	}
	if o.WebBase == "" {
		o.WebBase = AppOrigin
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return NewSessionBundle(SessionBundle{APIBase: o.APIBase, WebBase: o.WebBase, Browser: o.Browser, Deviceprint: o.Deviceprint, AntifraudDeviceprint: o.AntifraudDeviceprint, CapturedAt: &now,
		Cookies: []CookieRecord{{Name: "UFS-SESSION", Value: c.UFSSession, Domain: AuthCookieDomain, Path: "/", Secure: true, HTTPOnly: true, HostOnly: false}, {Name: "UFS-TOKEN", Value: c.UFSToken, Domain: AuthCookieDomain, Path: "/", Secure: true, HTTPOnly: true, HostOnly: false}}})
}
func (c SberCredentials) String() string             { return "SberCredentials(<redacted>)" }
func (c SberCredentials) GoString() string           { return c.String() }
func (c SberCredentials) Format(f fmt.State, v rune) { formatError(f, c.String()) }
func (c SberCredentials) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"ufs_session": "<redacted>", "ufs_token": "<redacted>"})
}

type SessionSeed struct {
	APIBase, WebBase string
	Cookies          *CookieJar
	ObservedHeaders  map[string]string
}

func (s SessionSeed) String() string             { return "SessionSeed(<redacted>)" }
func (s SessionSeed) GoString() string           { return s.String() }
func (s SessionSeed) Format(f fmt.State, v rune) { formatError(f, s.String()) }
func (s SessionSeed) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"session_seed": "<redacted>"})
}
func (b SessionBundle) ToSeed(requireReady bool) (SessionSeed, error) {
	b, err := NewSessionBundle(b)
	if err != nil {
		return SessionSeed{}, err
	}
	j, err := NewCookieJar(b.Cookies)
	if err != nil {
		return SessionSeed{}, err
	}
	if requireReady {
		for _, target := range []string{b.WebBase + "/api/warmUpSession", b.APIBase + "/main-screen/rest/v2/m1/web/section/meta", b.APIBase + "/uoh-bh/v1/operations/list"} {
			u, err := url.Parse(target)
			if err != nil || len(j.RecordsForURL(u)) == 0 {
				return SessionSeed{}, &MissingSession{}
			}
		}
	}
	return SessionSeed{b.APIBase, b.WebBase, j, b.Browser.AsMap()}, nil
}
func (b SessionBundle) WithCookieJar(jar *CookieJar) (SessionBundle, error) {
	if jar == nil {
		return SessionBundle{}, &MissingSession{}
	}
	b, err := NewSessionBundle(b)
	if err != nil {
		return SessionBundle{}, err
	}
	previous := map[cookieKey]CookieRecord{}
	for _, c := range b.Cookies {
		previous[keyForCookie(c)] = c
	}
	records := jar.Snapshot()
	for i, c := range records {
		if old, ok := previous[keyForCookie(c)]; ok && old.HTTPOnly && !c.HTTPOnly {
			records[i].HTTPOnly = true
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	b.Cookies = records
	b.CapturedAt = &now
	return NewSessionBundle(b)
}
