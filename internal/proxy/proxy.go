// Package proxy validates explicit proxy settings. Environment variables never
// select a proxy. Credentials are available only through explicit field access.
package proxy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vasyza/sber-sdk/internal/errs"
)

// Options selects an HTTP, HTTPS, or SOCKS5 proxy. URL is an address without
// credentials. The zero value selects a direct connection.
type Options struct {
	URL      string
	Username string
	Password string
}

func (o Options) String() string               { return "ProxyOptions(<redacted>)" }
func (o Options) Format(f fmt.State, _ rune)   { errs.FormatError(f, o.String()) }
func (o Options) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

func invalid() error { return &errs.TransportError{Code: "invalid_proxy"} }

// Parse accepts [scheme://]host:port[:username:password]. An omitted scheme
// selects HTTP. Brackets delimit IPv6 hosts; passwords may contain colons.
func Parse(input string) (Options, error) {
	if input == "" || len(input) > 4096 || !safeText(input) {
		return Options{}, invalid()
	}
	scheme, authority := "http", input
	if prefix, rest, ok := strings.Cut(input, "://"); ok && !strings.Contains(prefix, ":") {
		scheme, authority = strings.ToLower(prefix), rest
	}
	var host, rest string
	if strings.HasPrefix(authority, "[") {
		end := strings.IndexByte(authority, ']')
		if end < 0 || len(authority) <= end+1 || authority[end+1] != ':' {
			return Options{}, invalid()
		}
		host, rest = authority[:end+1], authority[end+2:]
	} else {
		var ok bool
		host, rest, ok = strings.Cut(authority, ":")
		if !ok {
			return Options{}, invalid()
		}
	}
	fields := strings.SplitN(rest, ":", 3)
	if len(fields) == 2 {
		return Options{}, invalid()
	}
	o := Options{URL: scheme + "://" + host + ":" + fields[0]}
	if len(fields) == 3 {
		o.Username, o.Password = fields[1], fields[2]
	}
	return o.Normalize()
}

// Normalize validates an SDK value and returns a canonical, immutable copy.
// Errors contain no address or credentials.
func (o Options) Normalize() (Options, error) {
	if o.URL == "" {
		if o.Username != "" || o.Password != "" {
			return Options{}, invalid()
		}
		return Options{}, nil
	}
	if len(o.URL) > 2048 || !safeText(o.URL) || !safeText(o.Username) || !safeText(o.Password) || len(o.Username) > 1024 || len(o.Password) > 1024 || o.Username == "" && o.Password != "" {
		return Options{}, invalid()
	}
	u, err := url.Parse(o.URL)
	if err != nil || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" {
		return Options{}, invalid()
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" {
		return Options{}, invalid()
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 || !validHost(host) {
		return Options{}, invalid()
	}
	if u.Scheme == "socks5" && (len(o.Username) > 255 || len(o.Password) > 255 || o.Username != "" && o.Password == "") {
		return Options{}, invalid()
	}
	o.URL = u.Scheme + "://" + net.JoinHostPort(strings.ToLower(host), strconv.Itoa(port))
	return o, nil
}

// Endpoint returns a validated URL with credentials for the standard transport.
// Callers must never format or log the returned URL.
func (o Options) Endpoint() (*url.URL, error) {
	o, err := o.Normalize()
	if err != nil || o.URL == "" {
		return nil, err
	}
	u, _ := url.Parse(o.URL)
	if o.Username != "" {
		u.User = url.UserPassword(o.Username, o.Password)
	}
	return u, nil
}

func safeText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, unicode.IsControl)
}

func validHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(strings.TrimSuffix(host, "."), ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if c != '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				return false
			}
		}
	}
	return true
}
