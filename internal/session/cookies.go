package session

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

const MaxCookies = 256

// CookieRecord is one opaque cookie with complete routing metadata. SameSite is
// nil (unspecified), Strict, Lax or None. Partitioned cookies are not supported.
// Explicit field access is sensitive; normal formatting/JSON is redacted.
type CookieRecord struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Secure   bool
	HTTPOnly bool
	HostOnly bool
	Expires  *int64
	SameSite *string
}

var CookieNamePattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func normalizeCookie(c CookieRecord, bankOnly bool) (CookieRecord, error) {
	c.Domain = strings.TrimRight(strings.ToLower(strings.TrimLeft(c.Domain, ".")), ".")
	if !utf8.ValidString(c.Name) || !utf8.ValidString(c.Value) || !utf8.ValidString(c.Domain) || !utf8.ValidString(c.Path) || !CookieNamePattern.MatchString(c.Name) ||
		utf8.RuneCountInString(c.Name) > 256 || utf8.RuneCountInString(c.Value) > 4096 || len(c.Domain) > 253 || c.Domain == "" ||
		utf8.RuneCountInString(c.Path) > 2048 || !strings.HasPrefix(c.Path, "/") || HasControls(c.Path) || HasControls(c.Value) || strings.Contains(c.Value, ";") || (bankOnly && !HostAllowed(c.Domain)) {
		return CookieRecord{}, &sdkErrs.MissingSession{Message: "invalid cookie metadata"}
	}
	if c.Expires != nil {
		if *c.Expires <= 0 {
			return CookieRecord{}, &sdkErrs.MissingSession{Message: "invalid cookie expiry"}
		}
		v := *c.Expires
		c.Expires = &v
	}
	if c.SameSite != nil {
		v := *c.SameSite
		if v != "Strict" && v != "Lax" && v != "None" {
			return CookieRecord{}, &sdkErrs.MissingSession{Message: "invalid cookie same_site"}
		}
		c.SameSite = &v
	}
	if strings.HasPrefix(c.Name, "__Secure-") && !c.Secure || strings.HasPrefix(c.Name, "__Host-") && (!c.Secure || !c.HostOnly || c.Path != "/") {
		return CookieRecord{}, &sdkErrs.MissingSession{Message: "invalid cookie prefix"}
	}
	return c, nil
}

func NewCookieRecord(c CookieRecord) (CookieRecord, error) { return normalizeCookie(c, true) }
func (c CookieRecord) Validate() error                     { _, err := NewCookieRecord(c); return err }
func (c CookieRecord) String() string                      { return "CookieRecord(<redacted>)" }
func (c CookieRecord) GoString() string                    { return c.String() }
func (c CookieRecord) Format(f fmt.State, v rune)          { sdkErrs.FormatError(f, c.String()) }
func (c CookieRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"value": "<redacted>"})
}

func HasControls(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
func HostAllowed(host string) bool {
	host = strings.TrimRight(strings.ToLower(host), ".")
	return host == "sberbank.ru" || strings.HasSuffix(host, ".sberbank.ru")
}

type cookieEntry struct {
	record CookieRecord
	order  uint64
}
type CookieKey struct{ name, domain, path string }

// CookieJar owns complete cookie records. It deliberately does not delegate
// snapshots to net/http/cookiejar, which strips HttpOnly/SameSite/expiry metadata.
// Mutation and snapshots are concurrent-safe and return deep copies.
type CookieJar struct {
	mu       sync.Mutex
	entries  map[CookieKey]cookieEntry
	sequence uint64
}

func KeyForCookie(c CookieRecord) CookieKey { return CookieKey{c.Name, c.Domain, c.Path} }
func cloneCookie(c CookieRecord) CookieRecord {
	if c.Expires != nil {
		v := *c.Expires
		c.Expires = &v
	}
	c.SameSite = CloneString(c.SameSite)
	return c
}
func NewCookieJar(records []CookieRecord) (*CookieJar, error) {
	j := &CookieJar{entries: make(map[CookieKey]cookieEntry)}
	if len(records) > MaxCookies {
		return nil, &sdkErrs.MissingSession{}
	}
	for _, c := range records {
		r, err := normalizeCookie(c, false)
		if err != nil {
			return nil, err
		}
		key := KeyForCookie(r)
		if _, exists := j.entries[key]; exists {
			return nil, &sdkErrs.MissingSession{}
		}
		if r.Expires != nil && *r.Expires <= time.Now().Unix() {
			continue
		}
		j.sequence++
		j.entries[key] = cookieEntry{r, j.sequence}
	}
	return j, nil
}
func (j *CookieJar) clearExpiredLocked(now int64) {
	for k, e := range j.entries {
		if e.record.Expires != nil && *e.record.Expires <= now {
			delete(j.entries, k)
		}
	}
}
func (j *CookieJar) Snapshot() []CookieRecord {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.clearExpiredLocked(time.Now().Unix())
	entries := make([]cookieEntry, 0, len(j.entries))
	for _, e := range j.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, k int) bool { return entries[i].order < entries[k].order })
	out := make([]CookieRecord, 0, len(entries))
	for _, e := range entries {
		out = append(out, cloneCookie(e.record))
	}
	return out
}
func cookieMatches(c CookieRecord, u *url.URL, now int64) bool {
	if u == nil {
		return false
	}
	host := strings.TrimRight(strings.ToLower(u.Hostname()), ".")
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return (host == c.Domain || !c.HostOnly && strings.HasSuffix(host, "."+c.Domain)) &&
		(path == c.Path || strings.HasPrefix(path, c.Path) && (strings.HasSuffix(c.Path, "/") || strings.HasPrefix(path[len(c.Path):], "/"))) &&
		(!c.Secure || u.Scheme == "https") && (c.Expires == nil || *c.Expires > now)
}
func (j *CookieJar) RecordsForURL(u *url.URL) []CookieRecord {
	records := j.Snapshot()
	out := make([]CookieRecord, 0, len(records))
	for _, c := range records {
		if cookieMatches(c, u, time.Now().Unix()) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, k int) bool { return len(out[i].Path) > len(out[k].Path) })
	return out
}

// CookieHeader returns sensitive wire data. It never formats CookieRecord with
// net/http.Cookie.String (which can silently strip bytes and log cookie values).
func (j *CookieJar) CookieHeader(u *url.URL) string {
	records := j.RecordsForURL(u)
	parts := make([]string, 0, len(records))
	for _, c := range records {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}
func (j *CookieJar) String() string             { return "CookieJar(<redacted>)" }
func (j *CookieJar) GoString() string           { return j.String() }
func (j *CookieJar) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, j.String()) }
func (j *CookieJar) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"cookies": "<redacted>"})
}

func DefaultCookiePath(path string) string {
	if !strings.HasPrefix(path, "/") || strings.Count(path, "/") <= 1 {
		return "/"
	}
	return path[:strings.LastIndex(path, "/")]
}

type cookieChange struct {
	Record  CookieRecord
	Deleted bool
}

func ParseResponseCookie(u *url.URL, header string, now int64) (*cookieChange, error) {
	parts := strings.Split(header, ";")
	var policy *string
	for _, part := range parts[1:] {
		name, value, has := strings.Cut(strings.TrimSpace(part), "=")
		switch strings.ToLower(name) {
		case "partitioned":
			return nil, &sdkErrs.MissingSession{Message: "partitioned cookie unsupported"}
		case "samesite":
			if !has {
				return nil, &sdkErrs.MissingSession{}
			}
			v := strings.Trim(strings.TrimSpace(value), "\"")
			switch strings.ToLower(v) {
			case "strict":
				v = "Strict"
			case "lax":
				v = "Lax"
			case "none":
				v = "None"
			default:
				return nil, &sdkErrs.MissingSession{Message: "unsupported same_site"}
			}
			policy = &v
		}
	}
	// curl accepts observed valueless protection cookies. Normalize only the
	// first pair, then let the standard parser retain every attribute.
	if !strings.Contains(parts[0], "=") && CookieNamePattern.MatchString(strings.TrimSpace(parts[0])) {
		header = strings.TrimSpace(parts[0]) + "="
		if len(parts) > 1 {
			header += ";" + strings.Join(parts[1:], ";")
		}
	}
	c, err := http.ParseSetCookie(header)
	if err != nil {
		return nil, nil
	}
	host := strings.TrimRight(strings.ToLower(u.Hostname()), ".")
	domain := strings.TrimRight(strings.ToLower(strings.TrimLeft(c.Domain, ".")), ".")
	hostOnly := c.Domain == ""
	if hostOnly {
		domain = host
	}
	if domain != host && (!strings.HasSuffix(host, "."+domain) || !HostAllowed(domain)) {
		return nil, nil
	}
	if HostAllowed(host) && !HostAllowed(domain) {
		return nil, nil
	}
	path := c.Path
	if !strings.HasPrefix(path, "/") {
		path = DefaultCookiePath(u.EscapedPath())
	}
	r := CookieRecord{Name: c.Name, Value: c.Value, Domain: domain, Path: path, Secure: c.Secure, HTTPOnly: c.HttpOnly, HostOnly: hostOnly, SameSite: policy}
	deleted := c.MaxAge < 0
	if c.MaxAge > 0 {
		if int64(c.MaxAge) > int64(^uint64(0)>>1)-now {
			return nil, &sdkErrs.MissingSession{}
		}
		n := now + int64(c.MaxAge)
		r.Expires = &n
	} else if c.MaxAge == 0 && !c.Expires.IsZero() {
		n := c.Expires.Unix()
		deleted = n <= now
		if !deleted {
			r.Expires = &n
		}
	}
	r, err = normalizeCookie(r, false)
	if err != nil {
		return nil, nil
	}
	if r.Secure && u.Scheme != "https" {
		return nil, nil
	}
	return &cookieChange{r, deleted}, nil
}

// ApplySetCookie applies only authoritative scoped changes and deletion
// directives. SameSite casing is normalized; absence explicitly removes the
// prior policy even when the value is unchanged. Unsupported metadata fails the
// whole response atomically; unrelated in-flight cookies are never pruned.
func (j *CookieJar) ApplySetCookie(u *url.URL, headers []string) error {
	if u == nil || u.Hostname() == "" {
		return &sdkErrs.MissingSession{}
	}
	now := time.Now().Unix()
	changes := make([]cookieChange, 0, len(headers))
	for _, h := range headers {
		c, err := ParseResponseCookie(u, h, now)
		if err != nil {
			return err
		}
		if c != nil {
			changes = append(changes, *c)
		}
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.clearExpiredLocked(now)
	entries := make(map[CookieKey]cookieEntry, len(j.entries))
	for k, e := range j.entries {
		entries[k] = e
	}
	sequence := j.sequence
	for _, c := range changes {
		k := KeyForCookie(c.Record)
		if c.Deleted {
			delete(entries, k)
			continue
		}
		order := entries[k].order
		if order == 0 {
			sequence++
			order = sequence
		}
		entries[k] = cookieEntry{c.Record, order}
	}
	if len(entries) > MaxCookies {
		return &sdkErrs.MissingSession{}
	}
	j.entries = entries
	j.sequence = sequence
	return nil
}
