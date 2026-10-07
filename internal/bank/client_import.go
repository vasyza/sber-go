package bank

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// ClientFileOptions relaxes private-file permission checks only by explicit
// opt-in. Native no-follow, regular-file, Unicode and input-size bounds remain.
// This constructor uses the existing private source reader's 2 MiB file bound.
type ClientFileOptions struct{ AllowNonPrivateHAR, AllowNonPrivateCookies bool }

// NewSberClientFromFiles imports observed hosts/headers/cookies, not a browser
// process. It performs no login, warm-up or bank request. Cookies may complete a
// sanitized HAR. Unlike profile factories, imported source files are NOT updated.
func NewSberClientFromFiles(har, cookies string, o ClientOptions, imports ...ClientFileOptions) (*SberClient, error) {
	if len(imports) > 1 {
		return nil, &sdkErrs.MissingSession{Message: "invalid source file options"}
	}
	var options ClientFileOptions
	if len(imports) == 1 {
		options = imports[0]
	}
	raw, e := sdkSession.ReadSessionFile(har, !options.AllowNonPrivateHAR)
	if e != nil {
		return nil, e
	}
	doc, e := DecodeJSON(bytes.NewReader(raw))
	if e != nil {
		return nil, &sdkErrs.MissingSession{Message: "invalid HAR"}
	}
	b, e := clientBundleFromHAR(doc)
	if e != nil {
		return nil, e
	}
	if cookies != "" {
		raw, e = sdkSession.ReadSessionFile(cookies, !options.AllowNonPrivateCookies)
		if e != nil {
			return nil, e
		}
		extra, e := clientNetscapeCookies(raw)
		if e != nil {
			return nil, e
		}
		accumulator := newClientCookieImport()
		for _, r := range b.Cookies {
			if e = accumulator.apply(r, false); e != nil {
				return nil, e
			}
		}
		for _, r := range extra {
			if e = accumulator.apply(r, false); e != nil {
				return nil, e
			}
		}
		b.Cookies = accumulator.records()
	}
	b, e = sdkSession.NewSessionBundle(b)
	if e != nil {
		return nil, e
	}
	return NewSberClient(b, o)
}

// Import only the audited cookie routing fields. Applying historical Max-Age
// uses the HAR response timestamp, not the time the import is performed.
type clientCookieImport struct {
	recordsByKey map[sdkSession.CookieKey]sdkSession.CookieRecord
	order        []sdkSession.CookieKey
}

func newClientCookieImport() *clientCookieImport {
	return &clientCookieImport{recordsByKey: map[sdkSession.CookieKey]sdkSession.CookieRecord{}}
}
func (i *clientCookieImport) apply(record sdkSession.CookieRecord, deleted bool) error {
	expiry := record.Expires
	record.Expires = nil
	record, e := sdkSession.NewCookieRecord(record)
	if e != nil {
		return e
	}
	key := sdkSession.KeyForCookie(record)
	if deleted || expiry != nil && *expiry <= time.Now().Unix() {
		delete(i.recordsByKey, key)
		return nil
	}
	record.Expires = expiry
	if _, present := i.recordsByKey[key]; !present {
		already := false
		for _, prior := range i.order {
			if prior == key {
				already = true
				break
			}
		}
		if !already {
			i.order = append(i.order, key)
		}
	}
	i.recordsByKey[key] = record
	if len(i.recordsByKey) > sdkSession.MaxCookies {
		return &sdkErrs.MissingSession{Message: "too many imported cookies"}
	}
	return nil
}
func (i *clientCookieImport) records() []sdkSession.CookieRecord {
	result := make([]sdkSession.CookieRecord, 0, len(i.recordsByKey))
	for _, key := range i.order {
		if r, exists := i.recordsByKey[key]; exists {
			result = append(result, r)
		}
	}
	return result
}
func clientImportInteger(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		// Integer JSON tokens and finite source floats may exceed the native
		// bound. Keep their sign/lifetime, never reinterpret them as absent.
		if n, e := v.Int64(); e == nil || errors.Is(e, strconv.ErrRange) {
			return n, true
		}
		n, e := v.Float64()
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		if n >= float64(uint64(1)<<63) {
			return math.MaxInt64, true
		}
		if n < -float64(uint64(1)<<63) {
			return math.MinInt64, true
		}
		return int64(n), true
	case string:
		n, e := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n, e == nil || errors.Is(e, strconv.ErrRange)
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}
func clientImportTimestamp(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number, bool:
		return clientImportInteger(v)
	case string:
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
			if t, e := time.Parse(layout, v); e == nil {
				return t.Unix(), true
			}
		}
		if t, e := http.ParseTime(v); e == nil {
			return t.Unix(), true
		}
	}
	return 0, false
}
func clientImportString(m map[string]any, k string) string { s, _ := m[k].(string); return s }
func clientImportBool(m map[string]any, k string, fallback bool) bool {
	v, exists := m[k]
	if !exists {
		return fallback
	}
	b, ok := v.(bool)
	return ok && b
}
func clientImportHeaderOrder(v any) []string {
	items, _ := v.([]any)
	names := []string{}
	seen := map[string]bool{}
	for _, item := range items {
		m, _ := item.(map[string]any)
		name := strings.ToLower(clientImportString(m, "name"))
		value, ok := m["value"].(string)
		if name != "" && ok && !strings.ContainsAny(value, "\r\n") && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
func clientImportHeaders(v any) map[string]string {
	result := map[string]string{}
	items, _ := v.([]any)
	for _, item := range items {
		m, _ := item.(map[string]any)
		name := strings.ToLower(clientImportString(m, "name"))
		value, ok := m["value"].(string)
		if name != "" && ok && !strings.ContainsAny(value, "\r\n") {
			result[name] = value
		}
	}
	return result
}
func clientImportStructured(i *clientCookieImport, value any, origin *url.URL, path string, received int64) (int, error) {
	items, _ := value.([]any)
	accepted := 0
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		// The native importer does not coerce opaque identities or security
		// flags: unsupported scalar spellings fail before any transport exists.
		for _, key := range []string{"name", "value", "domain", "path"} {
			if value, exists := m[key]; exists && value != nil {
				if _, ok := value.(string); !ok {
					return 0, &sdkErrs.MissingSession{Message: "unsupported cookie scalar"}
				}
			}
		}
		for _, key := range []string{"secure", "httpOnly", "hostOnly"} {
			if value, exists := m[key]; exists {
				if _, ok := value.(bool); !ok {
					return 0, &sdkErrs.MissingSession{Message: "unsupported cookie scalar"}
				}
			}
		}
		domain := clientImportString(m, "domain")
		if domain == "" {
			domain = origin.Hostname()
		}
		domain = strings.TrimRight(strings.ToLower(strings.TrimLeft(domain, ".")), ".")
		host := strings.ToLower(origin.Hostname())
		if !sdkSession.HostAllowed(domain) || host != domain && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		cookiePath := clientImportString(m, "path")
		if cookiePath == "" {
			cookiePath = path
		}
		r := sdkSession.CookieRecord{Name: clientImportString(m, "name"), Value: clientImportString(m, "value"), Domain: domain, Path: cookiePath, Secure: clientImportBool(m, "secure", true), HTTPOnly: clientImportBool(m, "httpOnly", false), HostOnly: clientImportString(m, "domain") == "" || clientImportBool(m, "hostOnly", false)}
		if n, ok := clientImportTimestamp(m["expires"]); ok {
			r.Expires = &n
		} else if _, numeric := m["expires"].(json.Number); numeric {
			return 0, &sdkErrs.MissingSession{Message: "invalid cookie expiry"}
		}
		maxAge, exists := m["maxAge"]
		if !exists {
			maxAge, exists = m["max-age"]
		}
		if exists {
			if seconds, ok := clientImportInteger(maxAge); ok {
				if seconds > 0 && received > int64(^uint64(0)>>1)-seconds || seconds < 0 && received < (-int64(^uint64(0)>>1)-1)-seconds {
					return 0, &sdkErrs.MissingSession{Message: "invalid cookie expiry"}
				}
				expiry := received + seconds
				r.Expires = &expiry
			} else if _, numeric := maxAge.(json.Number); numeric {
				return 0, &sdkErrs.MissingSession{Message: "invalid cookie expiry"}
			}
		}
		if value, exists := m["sameSite"]; exists && value != nil {
			policy, ok := value.(string)
			if !ok {
				return 0, &sdkErrs.MissingSession{Message: "unsupported cookie metadata"}
			}
			r.SameSite = &policy
		}
		if partitioned, exists := m["partitioned"]; exists && partitioned != false && partitioned != nil {
			return 0, &sdkErrs.MissingSession{Message: "partitioned cookie unsupported"}
		}
		if e := i.apply(r, false); e != nil {
			return 0, e
		}
		accepted++
	}
	return accepted, nil
}

var clientImportWebHost = regexp.MustCompile(`^web[0-9]+\.online\.sberbank\.ru$`)

func clientBundleFromHAR(doc map[string]any) (sdkSession.SessionBundle, error) {
	log, _ := doc["log"].(map[string]any)
	entries, ok := log["entries"].([]any)
	if !ok {
		return sdkSession.SessionBundle{}, &sdkErrs.MissingSession{Message: "HAR entries required"}
	}
	accumulator := newClientCookieImport()
	b := sdkSession.SessionBundle{}
	headers := map[string]string{}
	headerOrder := []string{}
	for _, entry := range entries {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		request, _ := m["request"].(map[string]any)
		response, _ := m["response"].(map[string]any)
		target := clientImportString(request, "url")
		if !sdkSession.IsOnlineURL(target) {
			continue
		}
		origin, e := url.Parse(target)
		if e != nil {
			continue
		}
		received, ok := clientImportTimestamp(m["startedDateTime"])
		if !ok {
			received = time.Now().Unix()
		}
		observed := clientImportHeaders(request["headers"])
		if clientReadPath(origin.Path) {
			b.APIBase = origin.Scheme + "://" + origin.Host
			for _, name := range clientImportHeaderOrder(request["headers"]) {
				if sdkSession.ObservedHeaderNames[name] {
					if _, exists := headers[name]; !exists {
						headerOrder = append(headerOrder, name)
					}
					headers[name] = observed[name]
				}
			}
		}
		if origin.Path == "/main" {
			if clientImportWebHost.MatchString(origin.Hostname()) {
				b.WebBase = origin.Scheme + "://" + origin.Host
			}
			content, _ := response["content"].(map[string]any)
			if api, ok := sdkSession.APIBaseFromMainHTML(clientImportString(content, "text")); ok {
				b.APIBase = api
			}
		}
		if strings.EqualFold(clientImportString(request, "method"), "POST") && clientMutationPath(origin.Path) {
			if value := observed["rsa-antifraud-device-print"]; value != "" {
				b.AntifraudDeviceprint = sdkTransport.PtrString(value)
				if e := sdkSession.ValidateDeviceprint(b.AntifraudDeviceprint); e != nil {
					return sdkSession.SessionBundle{}, e
				}
			}
		}
		n, e := clientImportStructured(accumulator, request["cookies"], origin, "/", received)
		if e != nil {
			return sdkSession.SessionBundle{}, e
		}
		if n == 0 && observed["cookie"] != "" {
			r := http.Request{Header: http.Header{"Cookie": []string{observed["cookie"]}}}
			for _, cookie := range r.Cookies() {
				if e = accumulator.apply(sdkSession.CookieRecord{Name: cookie.Name, Value: cookie.Value, Domain: origin.Hostname(), Path: "/", Secure: true, HostOnly: true}, false); e != nil {
					return sdkSession.SessionBundle{}, e
				}
			}
		}
		if _, e = clientImportStructured(accumulator, response["cookies"], origin, sdkSession.DefaultCookiePath(origin.EscapedPath()), received); e != nil {
			return sdkSession.SessionBundle{}, e
		}
		items, _ := response["headers"].([]any)
		for _, item := range items {
			h, _ := item.(map[string]any)
			if !strings.EqualFold(clientImportString(h, "name"), "set-cookie") {
				continue
			}
			change, e := sdkSession.ParseResponseCookie(origin, clientImportString(h, "value"), received)
			if e != nil {
				return sdkSession.SessionBundle{}, e
			}
			if change != nil {
				if e = accumulator.apply(change.Record, change.Deleted); e != nil {
					return sdkSession.SessionBundle{}, e
				}
			}
		}
	}
	for _, name := range headerOrder {
		b.Browser.Headers = append(b.Browser.Headers, sdkSession.BrowserHeader{Name: name, Value: headers[name]})
	}
	b.Cookies = accumulator.records()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	b.CapturedAt = &now
	return b, nil
}
func clientNetscapeCookies(raw []byte) ([]sdkSession.CookieRecord, error) {
	if !utf8.Valid(raw) {
		return nil, &sdkErrs.MissingSession{Message: "invalid cookie source text"}
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")
	accumulator := newClientCookieImport()
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		httpOnly := strings.HasPrefix(line, "#HttpOnly_")
		if httpOnly {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		columns := strings.SplitN(line, "\t", 7)
		if len(columns) != 7 {
			return nil, &sdkErrs.MissingSession{Message: "invalid Netscape cookie row"}
		}
		domain, subdomains, path, secure, expiry, name, value := columns[0], strings.ToUpper(columns[1]), columns[2], strings.ToUpper(columns[3]), columns[4], columns[5], columns[6]
		if subdomains != "TRUE" && subdomains != "FALSE" || secure != "TRUE" && secure != "FALSE" || !strings.HasPrefix(path, "/") || sdkSession.HasControls(path) {
			return nil, &sdkErrs.MissingSession{Message: "invalid Netscape cookie metadata"}
		}
		seconds, e := strconv.ParseInt(expiry, 10, 64)
		if e != nil {
			return nil, &sdkErrs.MissingSession{Message: "invalid Netscape cookie expiry"}
		}
		if !sdkSession.HostAllowed(strings.TrimLeft(domain, ".")) {
			continue
		}
		r := sdkSession.CookieRecord{Name: name, Value: value, Domain: domain, Path: path, Secure: secure == "TRUE", HTTPOnly: httpOnly, HostOnly: subdomains == "FALSE" && !strings.HasPrefix(domain, ".")}
		if seconds > 0 {
			r.Expires = &seconds
		}
		if e = accumulator.apply(r, false); e != nil {
			return nil, e
		}
	}
	records := accumulator.records()
	if len(records) == 0 {
		return nil, &sdkErrs.MissingSession{Message: "no usable Netscape cookies"}
	}
	return records, nil
}
