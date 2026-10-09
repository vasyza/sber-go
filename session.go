// Public session API; implementation lives in internal/session.
package sber

import (
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
)

const MaxFrontendHTMLCharacters = sdkSession.MaxFrontendHTMLCharacters

type FrontendConfig = sdkSession.FrontendConfig

// NewFrontendConfig retains every supplied value verbatim, including partial
// synthetic configs. It performs no parsing, validation, network access or SRP
// defaulting. All-empty arguments return the native zero value.
func NewFrontendConfig(baseURL, processID string, pinLength int, nHex, gHex string, seamlessWeb, redirectPost bool) FrontendConfig {
	return sdkSession.NewFrontendConfig(baseURL, processID, pinLength, nHex, gHex, seamlessWeb, redirectPost)
}

// ParsePINConfig accepts only the remembered-browser PIN mode, with PIN enabled
// and already enrolled. Every security-relevant field must be a literal.
func ParsePINConfig(html string) (FrontendConfig, error) { return sdkSession.ParsePINConfig(html) }

// ParsePrimaryConfig accepts only cold-browser primary SRP and new PIN
// enrollment. It never falls back to PIN parameters or synthesizes a group.
func ParsePrimaryConfig(html string) (FrontendConfig, error) {
	return sdkSession.ParsePrimaryConfig(html)
}

const MaxCookies = sdkSession.MaxCookies

type CookieRecord = sdkSession.CookieRecord

func NewCookieRecord(c CookieRecord) (CookieRecord, error) { return sdkSession.NewCookieRecord(c) }

type CookieJar = sdkSession.CookieJar

func NewCookieJar(records []CookieRecord) (*CookieJar, error) {
	return sdkSession.NewCookieJar(records)
}

type Deviceprint = sdkSession.Deviceprint

// GenerateDeviceprint reproduces the audited Python deviceprint.py browser
// layout and ordered fields. These are synthetic data, not observed browser
// headers, protection cookies, a supported-browser claim or bank verification.
// Generate once and explicitly retain the identity when reuse is intended.
func GenerateDeviceprint() (Deviceprint, error) { return sdkSession.GenerateDeviceprint() }

// GenerateAntifraudDeviceprint implements urllib.parse.quote(source,safe="").
// Omitted input mints an independent identity; explicit empty input stays empty.
// Captured arrays/objects/hash extras are preserved byte-for-byte, not parsed.
// No policy for sending financial mutations is granted by generating this value.
func GenerateAntifraudDeviceprint(source ...string) (Deviceprint, error) {
	return sdkSession.GenerateAntifraudDeviceprint(source...)
}

const SessionSchema = sdkSession.SessionSchema
const SessionSchemaVersion = sdkSession.SessionSchemaVersion
const MaxSessionFileBytes = sdkSession.MaxSessionFileBytes
const MaxBrowserHeaders = sdkSession.MaxBrowserHeaders
const AppOrigin = sdkSession.AppOrigin
const AuthCookieDomain = sdkSession.AuthCookieDomain

type BrowserHeader = sdkSession.BrowserHeader
type BrowserProfile = sdkSession.BrowserProfile

func NewBrowserProfile(p BrowserProfile) (BrowserProfile, error) {
	return sdkSession.NewBrowserProfile(p)
}

type SessionBundle = sdkSession.SessionBundle

func IsOnlineURL(value string) bool { return sdkSession.IsOnlineURL(value) }

func NewSessionBundle(b SessionBundle) (SessionBundle, error) { return sdkSession.NewSessionBundle(b) }

type SessionLoadOptions = sdkSession.SessionLoadOptions

// LoadSessionBundle strictly imports the exact Python v1-v4 field contracts and
// normalizes them to v4; no unknown fields, duplicate keys or lossy coercions.
func LoadSessionBundle(path string, options ...SessionLoadOptions) (SessionBundle, error) {
	return sdkSession.LoadSessionBundle(path, options...)
}

// DecodeSessionBundle accepts sensitive input in memory, with the same strict
// schema/size validation used by the private-file loader.
func DecodeSessionBundle(raw []byte) (SessionBundle, error) {
	return sdkSession.DecodeSessionBundle(raw)
}

type SberCredentials = sdkSession.SberCredentials

func NewSberCredentials(session, token string) (SberCredentials, error) {
	return sdkSession.NewSberCredentials(session, token)
}

func CredentialsFromBundle(b SessionBundle) (SberCredentials, error) {
	return sdkSession.CredentialsFromBundle(b)
}

type CredentialsBundleOptions = sdkSession.CredentialsBundleOptions
type SessionSeed = sdkSession.SessionSeed

const MaxAuthRedirects = sdkSession.MaxAuthRedirects

// APIBaseFromMainHTML discovers only one unambiguous runtime UFS API origin.
// Matching repeated values are allowed; mixed types, hosts or unsafe URLs fail.
func APIBaseFromMainHTML(html string) (string, bool) { return sdkSession.APIBaseFromMainHTML(html) }

// UFSHostFromAppShell discovers the frontend host from a fetched /app/main shell.
func UFSHostFromAppShell(html string) (string, bool) { return sdkSession.UFSHostFromAppShell(html) }

// NormalizeAuthBase validates a literal frontend auth base. Relative segments
// use AppOrigin; absolute bases must be HTTPS online hosts, without credentials,
// non-443 ports, queries, fragments or path traversal. No source value is logged.
func NormalizeAuthBase(value string) (string, error) { return sdkSession.NormalizeAuthBase(value) }

// SafeOnlineURL resolves a navigation URL to HTTPS online hosts only. Like the
// Python redirect helper it preserves query tickets, resolves ordinary dot
// segments and discards the fragment (it is never sent in an HTTP request).
func SafeOnlineURL(value, base string) (string, error) { return sdkSession.SafeOnlineURL(value, base) }

func IsAuthRedirectStatus(status int) bool { return sdkSession.IsAuthRedirectStatus(status) }

// ResolveAuthRedirect validates Location and preserves an existing empty-body
// POST only across 307/308. It never promotes GET to POST or follows a response.
func ResolveAuthRedirect(current string, status int, location string, post bool) (string, bool, error) {
	return sdkSession.ResolveAuthRedirect(current, status, location, post)
}

// AuthEndpoint appends an auth resource path without urljoin's base-path reset.
// Queries/fragments and escaped traversal are not valid endpoint definitions.
func AuthEndpoint(base, path string) (string, error) { return sdkSession.AuthEndpoint(base, path) }

// SafeCaptchaURL constrains a resolved asset to the loaded auth base path.
// It does not retrieve or solve a CAPTCHA and does not generate cache-busters.
func SafeCaptchaURL(value, base string) (string, error) {
	return sdkSession.SafeCaptchaURL(value, base)
}
