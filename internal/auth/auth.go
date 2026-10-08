package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	"github.com/vasyza/sber-go/internal/srp"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// AuthOptions injects a transport for tests or caller-owned verified transport.
// An auth object owns and closes every transport it accepts or constructs.
type AuthTransportFactory func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error)
type AuthOptions struct {
	Deviceprint, AntifraudDeviceprint *string
	Browser                           sdkSession.BrowserProfile
	IsPWA                             bool
	Transport                         sdkTransport.Transport
	TransportFactory                  AuthTransportFactory
	TransportOptions                  sdkTransport.TransportOptions
}

func (AuthOptions) String() string               { return "AuthOptions(<redacted>)" }
func (o AuthOptions) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, o.String()) }
func (AuthOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

type authFlow struct {
	primarySRP                 *srp.Client
	primaryToken, pinPublicKey string
	primaryOTPPending          bool
	otpPending                 bool
	qrPending                  bool
	srp                        *srp.Client
	csrf                       string
	authenticated              bool
	gate                       chan struct{}
	mu                         sync.Mutex
	closeMu                    sync.Mutex
	root                       context.Context
	cancel                     context.CancelFunc
	closed, closeComplete      bool
	bundle                     sdkSession.SessionBundle
	transport                  sdkTransport.Transport
	config                     *sdkSession.FrontendConfig
	primary                    bool
	options                    AuthOptions
	owned                      []*authOwnedTransport
}

// PINAuth is a serialized remembered-browser authentication process. Neither
// PINs nor OTPs are stored on the object. A profile is not authenticated merely
// because its anonymous document says that PIN login is enabled.
type PINAuth struct{ *authFlow }

func NewPINAuth(bundle sdkSession.SessionBundle, o AuthOptions) (*PINAuth, error) {
	f, err := newAuthFlow(bundle, o, false)
	if err != nil {
		return nil, err
	}
	return &PINAuth{f}, nil
}
func newAuthFlow(bundle sdkSession.SessionBundle, o AuthOptions, primary bool) (*authFlow, error) {
	if o.Deviceprint != nil {
		bundle.Deviceprint = o.Deviceprint
	}
	if o.AntifraudDeviceprint != nil {
		bundle.AntifraudDeviceprint = o.AntifraudDeviceprint
	}
	b, err := bundle.Clone()
	if err != nil {
		return nil, err
	}
	// The bank's web-session handoff requires the Mozilla compatibility prefix.
	// Declare the native SDK rather than a browser engine. Explicit observed
	// headers remain authoritative; no browser process or security cookie is made.
	if _, present := b.Browser.AsMap()["user-agent"]; !present {
		b.Browser.Headers = append(b.Browser.Headers, sdkSession.BrowserHeader{Name: "user-agent", Value: "Mozilla/5.0 (compatible; sber-go/1.0; +https://github.com/vasyza/sber-go)"})
	}
	if b.Deviceprint == nil {
		return nil, &sdkErrs.MissingSession{Message: "auth deviceprint required"}
	}
	if o.TransportOptions.Retry != 0 {
		return nil, &sdkErrs.TransportError{Code: "retry_forbidden"}
	}
	o.TransportOptions.AllowUnready = true
	if o.TransportFactory == nil {
		o.TransportFactory = func(b sdkSession.SessionBundle, o sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			return sdkTransport.NewAuthenticationTransport(b, o)
		}
	}
	tr := o.Transport
	if tr == nil {
		tr, err = o.TransportFactory(b, o.TransportOptions)
		if err != nil {
			return nil, err
		}
	}
	if tr == nil || tr.CookieJar() == nil {
		return nil, &sdkErrs.MissingSession{}
	}
	root, cancel := context.WithCancel(context.Background())
	owned := &authOwnedTransport{transport: tr}
	return &authFlow{gate: make(chan struct{}, 1), root: root, cancel: cancel, bundle: b, transport: tr, primary: primary, options: o, owned: []*authOwnedTransport{owned}}, nil
}
func (f *authFlow) String() string               { return "Auth(<redacted>)" }
func (f *authFlow) GoString() string             { return f.String() }
func (f *authFlow) Format(s fmt.State, v rune)   { sdkErrs.FormatError(s, f.String()) }
func (f *authFlow) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }
func (f *authFlow) check(ctx context.Context) error {
	f.mu.Lock()
	closed := f.closed
	f.mu.Unlock()
	if closed {
		return sdkErrs.ErrClosed
	}
	if ctx != nil && ctx.Err() != nil {
		return sdkTransport.TransportFailure(ctx.Err())
	}
	return nil
}
func (f *authFlow) enter(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, &sdkErrs.TransportError{Code: "invalid_context"}
	}
	if err := f.check(ctx); err != nil {
		return nil, nil, err
	}
	select {
	case f.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, sdkTransport.TransportFailure(ctx.Err())
	case <-f.root.Done():
		return nil, nil, sdkErrs.ErrClosed
	}
	child, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(f.root, cancel)
	done := func() { stop(); cancel(); <-f.gate }
	if err := f.check(child); err != nil {
		done()
		return nil, nil, err
	}
	return child, done, nil
}

type authOwnedTransport struct {
	mu        sync.Mutex
	transport sdkTransport.Transport
	closed    bool
}

func (o *authOwnedTransport) close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	if e := o.transport.Close(); e != nil {
		return &sdkErrs.TransportError{Code: "close_failed"}
	}
	o.closed = true
	return nil
}
func (f *authFlow) Close() error {
	// Lifetime is closed immediately, even if another cleanup is blocked/failed.
	f.mu.Lock()
	f.closed = true
	f.cancel()
	f.mu.Unlock()
	f.closeMu.Lock()
	defer f.closeMu.Unlock()
	f.mu.Lock()
	if f.closeComplete {
		f.mu.Unlock()
		return nil
	}
	owned := append([]*authOwnedTransport(nil), f.owned...)
	f.mu.Unlock()
	var failed bool
	for i := len(owned) - 1; i >= 0; i-- {
		if e := owned[i].close(); e != nil {
			failed = true
		}
	}
	if failed {
		return &sdkErrs.TransportError{Code: "close_failed"}
	}
	f.mu.Lock()
	f.closeComplete = true
	f.mu.Unlock()
	return nil
}

// ExportSession is an explicit sensitive snapshot, also available before login
// for remembered-device continuation. It does not assert authenticated readiness.
func (f *authFlow) ExportSession() (sdkSession.SessionBundle, error) {
	_, done, err := f.enter(context.Background())
	if err != nil {
		return sdkSession.SessionBundle{}, err
	}
	defer done()
	return f.bundle.WithCookieJar(f.transport.CookieJar())
}
func (f *authFlow) LoadConfig(ctx context.Context) (sdkSession.FrontendConfig, error) {
	ctx, done, err := f.enter(ctx)
	if err != nil {
		return sdkSession.FrontendConfig{}, authRequestError(err)
	}
	defer done()
	return f.loadConfig(ctx)
}
func (f *authFlow) loadConfig(ctx context.Context) (sdkSession.FrontendConfig, error) {
	if f.config != nil {
		return *f.config, nil
	}
	r, e := f.transport.Get(ctx, sdkTransport.PublicBootstrapURL, authDocumentHeaders("", sdkTransport.PublicBootstrapURL))
	if err := f.check(ctx); err != nil {
		return sdkSession.FrontendConfig{}, authRequestError(err)
	}
	if e != nil {
		return sdkSession.FrontendConfig{}, authRequestError(e)
	}
	if r == nil || r.StatusCode != 200 {
		return sdkSession.FrontendConfig{}, authFailure("bootstrap_failed", r)
	}
	html := r.Text()
	if sdkTransport.IsBrowserCheck(html) {
		return sdkSession.FrontendConfig{}, authFailure("browser_check_required", r)
	}
	var c sdkSession.FrontendConfig
	var err error
	if f.primary {
		c, err = sdkSession.ParsePrimaryConfig(html)
	} else {
		c, err = sdkSession.ParsePINConfig(html)
	}
	if err != nil {
		return sdkSession.FrontendConfig{}, authFailure("invalid_frontend_config", nil)
	}
	f.mu.Lock()
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		return sdkSession.FrontendConfig{}, f.check(ctx)
	}
	f.resetProcess()
	f.config = &c
	f.mu.Unlock()
	if err = f.check(ctx); err != nil {
		return sdkSession.FrontendConfig{}, authRequestError(err)
	}
	return c, nil
}

func authFailure(code string, r *sdkTransport.Response) *sdkErrs.PinAuthError {
	e := &sdkErrs.PinAuthError{Code: code}
	if r != nil {
		e.StatusCode = r.StatusCode
	}
	return e
}
func authDocumentHeaders(referer, target string) sdkTransport.RequestOptions {
	h := sdkTransport.HeaderOverrides{"Accept": sdkTransport.PtrString("text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"), "Content-Type": nil, "Origin": nil, "Referer": nil, "Sec-Fetch-Dest": sdkTransport.PtrString("document"), "Sec-Fetch-Mode": sdkTransport.PtrString("navigate"), "Sec-Fetch-Site": sdkTransport.PtrString("none"), "Sec-Fetch-User": sdkTransport.PtrString("?1"), "Upgrade-Insecure-Requests": sdkTransport.PtrString("1"), "X-Requested-With": nil}
	if referer != "" {
		h["Referer"] = sdkTransport.PtrString(referer)
		h["Sec-Fetch-Site"] = sdkTransport.PtrString(authFetchSite(referer, target))
	}
	return sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd", Headers: h}
}
func authOrigin(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
func authFetchSite(source, target string) string { // all navigation targets were validated before use
	a, _ := sdkSession.SafeOnlineURL(source, sdkSession.AppOrigin+"/")
	b, _ := sdkSession.SafeOnlineURL(target, sdkSession.AppOrigin+"/")
	if authOrigin(a) == authOrigin(b) {
		return "same-origin"
	}
	return "same-site"
}
