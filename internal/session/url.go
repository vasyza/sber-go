package session

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

// MaxAuthRedirects is the audited auth navigation bound. A caller must count
// responses; these pure primitives do not navigate, replay requests or own state.
const MaxAuthRedirects = 10

// APIBaseFromMainHTML discovers only one unambiguous runtime UFS API origin.
// Matching repeated values are allowed; mixed types, hosts or unsafe URLs fail.
func APIBaseFromMainHTML(html string) (string, bool) {
	return runtimeOnlineOrigin(html, "ufs.block.root.url")
}

// UFSHostFromAppShell discovers the frontend host from a fetched /app/main shell.
func UFSHostFromAppShell(html string) (string, bool) { return runtimeOnlineOrigin(html, "ufsHost") }
func runtimeOnlineOrigin(text, key string) (string, bool) {
	if !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxFrontendHTMLCharacters {
		return "", false
	}
	// Inspect complete quoted tokens, then the property separator. The shared
	// scanner retains Python-equivalent whitespace and treats intervening
	// comments as source syntax, not as a way to hide a conflicting key.
	occurrences := regexp.MustCompile(`"(?:\\.|[^"\\])*"`).FindAllStringIndex(text, -1)
	result := ""
	for _, occurrence := range occurrences {
		separator, ok := skipFrontendSpace(text, occurrence[1])
		if !ok {
			return "", false
		}
		if separator >= len(text) || text[separator] != ':' {
			continue
		}
		// Compare decoded names so escaped spellings cannot hide conflicts.
		property, _, ok := frontendPropertyKey(text, occurrence[0])
		if !ok {
			return "", false
		}
		if property != key {
			continue
		}
		start, ok := skipFrontendSpace(text, separator+1)
		if !ok || start >= len(text) || text[start] != '"' {
			return "", false
		}
		literalEnd, ok := skipFrontendString(text, start)
		if !ok {
			return "", false
		}
		// The complete property value must be a literal, never merely the
		// allowlisted prefix of concatenation, indexing or a call expression.
		end, ok := skipFrontendSpace(text, literalEnd)
		if !ok || end < len(text) && text[end] != ',' && text[end] != '}' {
			return "", false
		}
		var value string
		if json.Unmarshal([]byte(text[start:literalEnd]), &value) != nil || !safeURLText(value) || !IsOnlineURL(value) {
			return "", false
		}
		parsed, err := url.Parse(value)
		if err != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(value, "#") {
			return "", false
		}
		value = strings.TrimRight(value, "/")
		if result != "" && result != value {
			return "", false
		}
		result = value
	}
	return result, result != ""
}
func safeURLText(value string) bool {
	return value != "" && utf8.ValidString(value) && value == strings.TrimSpace(value) && !HasControls(value) && !strings.Contains(value, "\\")
}

// pathWithoutTraversal closes encoded-dot/backslash/control ambiguities at
// config/endpoint/captcha boundaries. Unlike navigation, these paths must not
// normalize into a different security scope. Doubly encoded paths also fail.
func pathWithoutTraversal(path string) bool {
	for round := 0; round < 5; round++ {
		if HasControls(path) || strings.Contains(path, "\\") {
			return false
		}
		for _, piece := range strings.Split(path, "/") {
			if piece == "." || piece == ".." {
				return false
			}
		}
		if !strings.Contains(path, "%") {
			return true
		}
		decoded, err := url.PathUnescape(path)
		if err != nil {
			return false
		}
		if decoded == path {
			return true
		}
		path = decoded
	}
	return false
}

// NormalizeAuthBase validates a literal frontend auth base. Relative segments
// use AppOrigin; absolute bases must be HTTPS online hosts, without credentials,
// non-443 ports, queries, fragments or path traversal. No source value is logged.
func NormalizeAuthBase(value string) (string, error) {
	if !safeURLText(value) {
		return "", invalidFrontendConfig()
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(value, "#") || !pathWithoutTraversal(parsed.Path) {
		return "", invalidFrontendConfig()
	}
	if IsOnlineURL(value) {
		return strings.TrimRight(value, "/"), nil
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || strings.HasPrefix(value, "//") {
		return "", invalidFrontendConfig()
	}
	pieces := []string{}
	// Preserve the URL's escaped representation instead of converting %xx into
	// active separators while removing redundant slash segments.
	for _, piece := range strings.Split(parsed.EscapedPath(), "/") {
		if piece != "" {
			pieces = append(pieces, piece)
		}
	}
	if len(pieces) == 0 {
		return "", invalidFrontendConfig()
	}
	result := AppOrigin + "/" + strings.Join(pieces, "/")
	if !IsOnlineURL(result) {
		return "", invalidFrontendConfig()
	}
	return result, nil
}

// SafeOnlineURL resolves a navigation URL to HTTPS online hosts only. Like the
// Python redirect helper it preserves query tickets, resolves ordinary dot
// segments and discards the fragment (it is never sent in an HTTP request).
func SafeOnlineURL(value, base string) (string, error) {
	failure := func() (string, error) {
		return "", &sdkErrs.PinAuthError{Code: "unsafe_redirect", Message: "unsafe redirect"}
	}
	if !safeURLText(value) || !safeURLText(base) || !IsOnlineURL(base) {
		return failure()
	}
	b, err := url.Parse(base)
	if err != nil {
		return failure()
	}
	v, err := url.Parse(value)
	if err != nil {
		return failure()
	}
	resolved := b.ResolveReference(v)
	resolved.Fragment = ""
	resolved.RawFragment = ""
	result := resolved.String()
	if !IsOnlineURL(result) {
		return failure()
	}
	return result, nil
}
func IsAuthRedirectStatus(status int) bool {
	return status == 301 || status == 302 || status == 303 || status == 307 || status == 308
}

// ResolveAuthRedirect validates Location and preserves an existing empty-body
// POST only across 307/308. It never promotes GET to POST or follows a response.
func ResolveAuthRedirect(current string, status int, location string, post bool) (string, bool, error) {
	if !IsAuthRedirectStatus(status) || location == "" {
		return "", false, &sdkErrs.PinAuthError{Code: "invalid_redirect", Message: "invalid redirect"}
	}
	target, err := SafeOnlineURL(location, current)
	if err != nil {
		return "", false, err
	}
	return target, post && (status == 307 || status == 308), nil
}

// AuthEndpoint appends an auth resource path without urljoin's base-path reset.
// Queries/fragments and escaped traversal are not valid endpoint definitions.
func AuthEndpoint(base, path string) (string, error) {
	invalid := func() (string, error) {
		return "", &sdkErrs.PinAuthError{Code: "unsafe_endpoint", Message: "unsafe endpoint"}
	}
	normalized, err := NormalizeAuthBase(base)
	if err != nil {
		return invalid()
	}
	if !safeURLText(path) || strings.ContainsAny(path, "?#") || strings.Contains(path, ":") || strings.HasPrefix(path, "//") {
		return invalid()
	}
	parsed, err := url.Parse(path)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || !pathWithoutTraversal(parsed.Path) {
		return invalid()
	}
	result := normalized + "/" + strings.TrimLeft(path, "/")
	if !IsOnlineURL(result) {
		return invalid()
	}
	return result, nil
}

// SafeCaptchaURL constrains a resolved asset to the loaded auth base path.
// It does not retrieve or solve a CAPTCHA and does not generate cache-busters.
func SafeCaptchaURL(value, base string) (string, error) {
	invalid := func() (string, error) {
		return "", &sdkErrs.PinAuthError{Code: "unsafe_captcha_url", Message: "unsafe captcha URL"}
	}
	normalized, err := NormalizeAuthBase(base)
	if err != nil {
		return invalid()
	}
	raw, err := url.Parse(value)
	if err != nil || !pathWithoutTraversal(raw.Path) {
		return invalid()
	}
	result, err := SafeOnlineURL(value, normalized+"/")
	if err != nil {
		return "", err
	}
	target, err := url.Parse(result)
	if err != nil {
		return invalid()
	}
	auth, _ := url.Parse(normalized)
	if !strings.HasPrefix(target.Path, strings.TrimRight(auth.Path, "/")+"/") {
		return invalid()
	}
	return result, nil
}
