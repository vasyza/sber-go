package transport

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkProxy "github.com/vasyza/sber-sdk/internal/proxy"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
)

// HeaderOverrides uses nil to remove a default header, just like Python None.
// The map is sensitive and redacted in fmt/JSON. Header values are copied.
type HeaderOverrides map[string]*string

func (h HeaderOverrides) String() string               { return "HeaderOverrides(<redacted>)" }
func (h HeaderOverrides) Format(f fmt.State, v rune)   { sdkErrs.FormatError(f, h.String()) }
func (h HeaderOverrides) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

type RequestOptions struct {
	AcceptEncoding string
	Headers        HeaderOverrides
}

func (o RequestOptions) String() string               { return "RequestOptions(<redacted>)" }
func (o RequestOptions) Format(f fmt.State, v rune)   { sdkErrs.FormatError(f, o.String()) }
func (o RequestOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

// Response owns decoded bytes. Accessors are explicit sensitive reads; normal
// formatting, logs and JSON never expose HTML, cookies or response identifiers.
type Response struct {
	StatusCode int
	Headers    http.Header
	Content    []byte
}

func (r Response) Text() string { return strings.ToValidUTF8(string(r.Content), "\ufffd") }
func (r Response) DecodeJSON(dst any) error {
	if err := json.Unmarshal(r.Content, dst); err != nil {
		return &sdkErrs.APIError{StatusCode: r.StatusCode}
	}
	return nil
}
func (r Response) JSON() (any, error) { var value any; err := r.DecodeJSON(&value); return value, err }
func (r Response) String() string {
	return fmt.Sprintf("Response(status=%d, <redacted>)", r.StatusCode)
}
func (r Response) GoString() string           { return r.String() }
func (r Response) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, r.String()) }
func (r Response) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"status_code": r.StatusCode, "body": "<redacted>"})
}

type ProxyOptions = sdkProxy.Options

// TransportOptions uses verified TLS. Default trust combines the
// bundled bank root with a canonical system PEM bundle, when available. An
// explicit CA file replaces that trust for this instance, never global trust.
// Retry must be zero. Timeout and capacity apply to all requests.
type TransportOptions struct {
	CABundle         string
	AllowUnready     bool
	MaxClients       int
	Retry            int
	Timeout          time.Duration
	MaxResponseBytes int64
	Proxy            ProxyOptions
}

func (o TransportOptions) String() string               { return "TransportOptions(<redacted>)" }
func (o TransportOptions) Format(f fmt.State, _ rune)   { sdkErrs.FormatError(f, o.String()) }
func (o TransportOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

// HTTPTransport uses ordinary verified TLS and HTTP/1.1. Business transports
// use one request per connection. Authentication transports reuse connections,
// reject replay-enabling POST headers and never provide a rewindable POST body.
type HTTPTransport struct {
	mu               sync.Mutex
	closed           bool
	root             context.Context
	cancel           context.CancelFunc
	wire             *http.Transport
	client           *http.Client
	jar              *sdkSession.CookieJar
	headers          http.Header
	slots            chan struct{}
	maxResponseBytes int64
	authConnections  bool
	proxyEnabled     bool
}

func NewHTTPTransport(b sdkSession.SessionBundle, o TransportOptions) (*HTTPTransport, error) {
	return newHTTPTransport(b, o, false)
}

// NewAuthenticationTransport retains ordinary verified connections throughout
// the auth handshake. POST bodies cannot rewind and replay-enabling headers are
// rejected. The business constructor keeps its single-use connection boundary.
func NewAuthenticationTransport(b sdkSession.SessionBundle, o TransportOptions) (*HTTPTransport, error) {
	return newHTTPTransport(b, o, true)
}

func newHTTPTransport(b sdkSession.SessionBundle, o TransportOptions, authConnections bool) (*HTTPTransport, error) {
	if o.Retry != 0 {
		return nil, &sdkErrs.TransportError{Code: "retry_forbidden"}
	}
	if o.MaxClients == 0 {
		o.MaxClients = 10
	}
	if o.MaxClients < 1 || o.MaxClients > 100 {
		return nil, &sdkErrs.TransportError{Code: "invalid_options"}
	}
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.Timeout < 0 {
		return nil, &sdkErrs.TransportError{Code: "invalid_options"}
	}
	if o.MaxResponseBytes == 0 {
		o.MaxResponseBytes = 16 * 1024 * 1024
	}
	if o.MaxResponseBytes < 1 || o.MaxResponseBytes > 64*1024*1024 {
		return nil, &sdkErrs.TransportError{Code: "invalid_options"}
	}
	proxyURL, err := o.Proxy.Endpoint()
	if err != nil {
		return nil, err
	}
	seed, err := b.ToSeed(!o.AllowUnready)
	if err != nil {
		return nil, err
	}
	roots, err := applicationRoots(o.CABundle)
	if err != nil {
		return nil, err
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	// The client deadline covers DNS, connection, verified TLS, headers and
	// body together. Do not shorten configured budgets with hidden phase caps.
	wire := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: o.Timeout}).DialContext,
		TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}}, TLSHandshakeTimeout: o.Timeout,
		DisableCompression: true, DisableKeepAlives: !authConnections, MaxConnsPerHost: o.MaxClients, ResponseHeaderTimeout: o.Timeout,
		IdleConnTimeout:        o.Timeout,
		MaxResponseHeaderBytes: 1 * 1024 * 1024, Protocols: protocols,
	}
	wire.DialTLSContext = verifiedTLSDialer(wire.TLSClientConfig, o.Timeout)
	if proxyURL != nil {
		wire.Proxy = http.ProxyURL(proxyURL)
		// net/http verifies both HTTPS proxy TLS and destination TLS. It also
		// owns the CONNECT/SOCKS handshake within the request deadline.
		wire.DialTLSContext = nil
		wire.OnProxyConnectResponse = func(_ context.Context, _ *url.URL, _ *http.Request, response *http.Response) error {
			if response.StatusCode == http.StatusProxyAuthRequired {
				return &sdkErrs.TransportError{Code: "proxy_authentication"}
			}
			if response.StatusCode != http.StatusOK {
				return &sdkErrs.TransportError{Code: "proxy_connect"}
			}
			return nil
		}
	}
	root, cancel := context.WithCancel(context.Background())
	tr := &HTTPTransport{root: root, cancel: cancel, wire: wire, jar: seed.Cookies, headers: browserHeaders(seed.ObservedHeaders), slots: make(chan struct{}, o.MaxClients), maxResponseBytes: o.MaxResponseBytes, authConnections: authConnections, proxyEnabled: proxyURL != nil}
	tr.client = &http.Client{Transport: wire, Timeout: o.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return tr, nil
}
func browserHeaders(observed map[string]string) http.Header {
	h := http.Header{}
	for n, v := range observed {
		h.Set(n, v)
	}
	for n, v := range map[string]string{"Accept": "application/json, text/plain, */*", "Content-Type": "application/json;charset=UTF-8", "Origin": sdkSession.AppOrigin, "Referer": sdkSession.AppOrigin + "/", "Sec-Fetch-Dest": "empty", "Sec-Fetch-Mode": "cors", "Sec-Fetch-Site": "same-site", "Priority": "u=1, i", "X-Requested-With": "XMLHttpRequest"} {
		if h.Get(n) == "" {
			if _, exists := h[http.CanonicalHeaderKey(n)]; !exists {
				h.Set(n, v)
			}
		}
	}
	return h
}
func (tr *HTTPTransport) CookieJar() *sdkSession.CookieJar { return tr.jar }
func (tr *HTTPTransport) String() string                   { return "HTTPTransport(<redacted>)" }
func (tr *HTTPTransport) GoString() string                 { return tr.String() }
func (tr *HTTPTransport) Format(f fmt.State, v rune)       { sdkErrs.FormatError(f, tr.String()) }
func (tr *HTTPTransport) MarshalJSON() ([]byte, error)     { return json.Marshal("<redacted>") }
func (tr *HTTPTransport) Close() error {
	tr.mu.Lock()
	if !tr.closed {
		tr.closed = true
		tr.cancel()
	}
	tr.mu.Unlock()
	tr.wire.CloseIdleConnections()
	return nil
}
func (tr *HTTPTransport) Get(ctx context.Context, target string, o RequestOptions) (*Response, error) {
	defaults := HeaderOverrides{"Accept": PtrString("*/*"), "Content-Type": PtrString("application/json; charset=utf-8"), "X-Requested-With": nil}
	owner := HeaderOverrides{}
	for name, value := range o.Headers {
		canonical := http.CanonicalHeaderKey(name)
		if prior, exists := owner[canonical]; exists && (prior == nil && value != nil || prior != nil && (value == nil || *prior != *value)) {
			// Preserve request's validation/error ordering for conflicting input.
			return tr.request(ctx, http.MethodGet, target, nil, "", o)
		}
		owner[canonical] = value
	}
	for name, value := range owner {
		defaults[name] = value
	}
	o.Headers = defaults
	return tr.request(ctx, http.MethodGet, target, nil, "", o)
}
func PtrString(s string) *string { return &s }

// Transport is the common native context-aware transport contract. SDK methods
// do not retry; pooled authentication may recover an idle connection for a GET
// but never replay a transmitted POST. CookieJar snapshots are complete and
// concurrent-safe.
type Transport interface {
	Get(context.Context, string, RequestOptions) (*Response, error)
	Post(context.Context, string, map[string]any, RequestOptions) (*Response, error)
	PostForm(context.Context, string, map[string]string, RequestOptions) (*Response, error)
	CookieJar() *sdkSession.CookieJar
	Close() error
}

var _ Transport = (*HTTPTransport)(nil)

func (tr *HTTPTransport) Post(ctx context.Context, target string, body map[string]any, o RequestOptions) (*Response, error) {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, &sdkErrs.TransportError{Code: "invalid_json_body"}
		}
		reader = bytes.NewReader(raw)
	}
	return tr.request(ctx, http.MethodPost, target, reader, "", o)
}
func (tr *HTTPTransport) PostForm(ctx context.Context, target string, data map[string]string, o RequestOptions) (*Response, error) {
	form := url.Values{}
	for n, v := range data {
		form.Set(n, v)
	}
	return tr.request(ctx, http.MethodPost, target, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", o)
}

func (tr *HTTPTransport) request(ctx context.Context, method, target string, body io.Reader, contentType string, o RequestOptions) (*Response, error) {
	tr.mu.Lock()
	closed := tr.closed
	tr.mu.Unlock()
	if closed {
		return nil, sdkErrs.ErrClosed
	}
	if ctx == nil {
		return nil, &sdkErrs.TransportError{Code: "invalid_context"}
	}
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(tr.root, cancel)
	defer stop()
	defer cancel()
	select {
	case tr.slots <- struct{}{}:
		defer func() { <-tr.slots }()
	case <-child.Done():
		return nil, TransportFailure(child.Err())
	}
	tr.mu.Lock()
	closed = tr.closed
	tr.mu.Unlock()
	if closed {
		return nil, sdkErrs.ErrClosed
	}
	requestContext, requestCancel := context.WithTimeout(child, tr.client.Timeout)
	defer requestCancel()
	requestContext = context.WithValue(requestContext, tlsRequestContextKey{}, requestContext)
	// Only a handshake closure can identify this phase. An EOF after successful
	// TLS must retain its HTTP failure classification; trace hooks can run concurrently.
	var proxyTLSClosed atomic.Bool
	if tr.proxyEnabled {
		requestContext = httptrace.WithClientTrace(requestContext, &httptrace.ClientTrace{
			TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
				proxyTLSClosed.Store(retryableTLSClosure(err))
			},
		})
	}
	req, err := http.NewRequestWithContext(requestContext, method, target, body)
	if err != nil {
		return nil, &sdkErrs.TransportError{Code: "unsafe_request"}
	}
	if req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Hostname() == "" {
		return nil, &sdkErrs.TransportError{Code: "unsafe_request"}
	}
	req.GetBody = nil
	req.Header = tr.headers.Clone()
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if o.AcceptEncoding != "" {
		req.Header.Set("Accept-Encoding", o.AcceptEncoding)
	} else {
		req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	}
	seen := map[string]bool{}
	for name, v := range o.Headers {
		canonical := http.CanonicalHeaderKey(name)
		if !sdkSession.CookieNamePattern.MatchString(name) || seen[canonical] {
			return nil, &sdkErrs.TransportError{Code: "invalid_headers"}
		}
		seen[canonical] = true
		if v == nil {
			req.Header.Del(canonical)
			if canonical == "User-Agent" {
				req.Header[canonical] = []string{""}
			}
			continue
		}
		if strings.ContainsAny(*v, "\r\n") {
			return nil, &sdkErrs.TransportError{Code: "invalid_headers"}
		}
		req.Header.Set(canonical, *v)
	}
	if tr.authConnections && method == http.MethodPost {
		// Presence, including an empty value, enables replay in net/http. Auth
		// callers must not attach either header to pooled POST requests.
		for _, name := range []string{"Idempotency-Key", "X-Idempotency-Key"} {
			if _, present := req.Header[name]; present {
				return nil, &sdkErrs.TransportError{Code: "replay_forbidden"}
			}
		}
	}
	if cookie := tr.jar.CookieHeader(req.URL); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	res, err := tr.client.Do(req)
	if err != nil {
		failure := TransportFailure(err)
		if tr.proxyEnabled && failure.Code == "request_failed" {
			failure.Code = "proxy_failed"
			if proxyTLSClosed.Load() {
				failure.Code = "proxy_tls_closed"
			}
		}
		return nil, failure
	}
	defer res.Body.Close()
	tr.mu.Lock()
	if tr.closed {
		tr.mu.Unlock()
		return nil, sdkErrs.ErrClosed
	}
	err = tr.jar.ApplySetCookie(req.URL, res.Header.Values("Set-Cookie"))
	tr.mu.Unlock()
	if err != nil {
		return nil, &sdkErrs.TransportError{Code: "unsupported_cookie_metadata"}
	}
	reader := io.Reader(res.Body)
	switch strings.ToLower(strings.TrimSpace(res.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, &sdkErrs.TransportError{Code: "invalid_encoding"}
		}
		defer gz.Close()
		reader = gz
	case "deflate":
		z, err := zlib.NewReader(res.Body)
		if err != nil {
			return nil, &sdkErrs.TransportError{Code: "invalid_encoding"}
		}
		defer z.Close()
		reader = z
	case "br":
		reader = brotli.NewReader(res.Body)
	case "zstd":
		z, err := zstd.NewReader(res.Body, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(64*1024*1024), zstd.WithDecoderMaxWindow(64*1024*1024))
		if err != nil {
			return nil, &sdkErrs.TransportError{Code: "invalid_encoding"}
		}
		defer z.Close()
		reader = z
	default:
		return nil, &sdkErrs.TransportError{Code: "unsupported_encoding"}
	}
	content, err := io.ReadAll(io.LimitReader(reader, tr.maxResponseBytes+1))
	if err != nil {
		return nil, TransportFailure(err)
	}
	if int64(len(content)) > tr.maxResponseBytes {
		return nil, &sdkErrs.TransportError{Code: "response_too_large"}
	}
	tr.mu.Lock()
	closed = tr.closed
	tr.mu.Unlock()
	if closed {
		return nil, sdkErrs.ErrClosed
	}
	return &Response{StatusCode: res.StatusCode, Headers: res.Header.Clone(), Content: content}, nil
}
func TransportFailure(err error) *sdkErrs.TransportError {
	if errors.Is(err, context.Canceled) {
		return sdkErrs.NewTransportError("canceled", context.Canceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return sdkErrs.NewTransportError("timeout", context.DeadlineExceeded)
	}
	var proxyError *sdkErrs.TransportError
	if errors.As(err, &proxyError) && (proxyError.Code == "proxy_authentication" || proxyError.Code == "proxy_connect") {
		return &sdkErrs.TransportError{Code: proxyError.Code}
	}
	var hostname x509.HostnameError
	if errors.As(err, &hostname) {
		return &sdkErrs.TransportError{Code: "tls_hostname"}
	}
	var unknown x509.UnknownAuthorityError
	if errors.As(err, &unknown) {
		return &sdkErrs.TransportError{Code: "tls_untrusted"}
	}
	var invalid x509.CertificateInvalidError
	if errors.As(err, &invalid) {
		if invalid.Reason == x509.Expired {
			return &sdkErrs.TransportError{Code: "tls_expired"}
		}
		return &sdkErrs.TransportError{Code: "tls_invalid"}
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return sdkErrs.NewTransportError("timeout", context.DeadlineExceeded)
	}
	return &sdkErrs.TransportError{Code: "request_failed"}
}

const PublicBootstrapURL = sdkSession.AppOrigin + "/CSAFront/index.do"

var configAssignmentPattern = regexp.MustCompile(`\bwindow\s*\.\s*config\s*=`)

// IsLoginPageRejected identifies the bank's connection rejection page. A real
// configuration can contain the same text as a translation, so it takes priority.
// This classification makes no claim about the bank's reason for rejection.
func IsLoginPageRejected(html string) bool {
	if utf8.RuneCountInString(html) > 4*1024*1024 || configAssignmentPattern.MatchString(html) {
		return false
	}
	return strings.Contains(strings.ToLower(html), "нельзя войти в сбербанк онлайн в этом браузере")
}

func IsBrowserCheck(html string) bool {
	if utf8.RuneCountInString(html) > 4*1024*1024 {
		return false
	}
	lower := strings.ToLower(html)
	return strings.Contains(lower, "/tspd/") && strings.Contains(lower, "<script") && strings.Contains(lower, "enable javascript") && !configAssignmentPattern.MatchString(html)
}
