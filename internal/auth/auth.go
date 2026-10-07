package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

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
	BrowserBootstrap                  sdkTransport.BrowserBootstrapProvider
	BrowserBootstrapTimeout           time.Duration
	// BrowserFirst requires explicit selection; malformed HTML never causes fallback.
	BrowserFirst bool
}

func (AuthOptions) String() string               { return "AuthOptions(<redacted>)" }
func (o AuthOptions) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, o.String()) }
func (AuthOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

type authFlow struct {
	currentOwned               *authOwnedTransport
	primarySRP                 *srp.Client
	primaryToken, pinPublicKey string
	primaryOTPPending          bool
	otpPending                 bool
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
	if b.Deviceprint == nil {
		return nil, &sdkErrs.MissingSession{Message: "auth deviceprint required"}
	}
	if o.TransportOptions.Retry != 0 {
		return nil, &sdkErrs.TransportError{Code: "retry_forbidden"}
	}
	if o.BrowserFirst && o.BrowserBootstrap == nil {
		return nil, authFailure("browser_bootstrap_unavailable", nil)
	}
	if o.BrowserBootstrapTimeout == 0 {
		o.BrowserBootstrapTimeout = 30 * time.Second
	}
	if o.BrowserBootstrapTimeout < 0 || o.BrowserBootstrapTimeout > 120*time.Second {
		return nil, authFailure("invalid_auth_options", nil)
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
	return &authFlow{gate: make(chan struct{}, 1), root: root, cancel: cancel, bundle: b, transport: tr, primary: primary, options: o, owned: []*authOwnedTransport{owned}, currentOwned: owned}, nil
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
	current, err := f.bundle.WithCookieJar(f.transport.CookieJar())
	if err != nil {
		return sdkSession.FrontendConfig{}, authRequestError(err)
	}
	var rendered *sdkTransport.BrowserBootstrapResult
	var html string
	if !f.options.BrowserFirst {
		r, e := f.transport.Get(ctx, sdkTransport.PublicBootstrapURL, authDocumentHeaders("", sdkTransport.PublicBootstrapURL))
		if err = f.check(ctx); err != nil {
			return sdkSession.FrontendConfig{}, authRequestError(err)
		}
		if e != nil {
			return sdkSession.FrontendConfig{}, authRequestError(e)
		}
		if r == nil || r.StatusCode != 200 {
			return sdkSession.FrontendConfig{}, authFailure("bootstrap_failed", r)
		}
		html = r.Text()
		if sdkTransport.IsBrowserCheck(html) {
			if f.options.BrowserBootstrap == nil {
				return sdkSession.FrontendConfig{}, authFailure("browser_check_required", r)
			}
		} else {
			goto parse
		}
	}
	{
		child, cancel := context.WithTimeout(ctx, f.options.BrowserBootstrapTimeout)
		result, e := f.options.BrowserBootstrap.Bootstrap(child, current, sdkTransport.PublicBootstrapURL)
		deadline := child.Err()
		cancel()
		if err = f.check(ctx); err != nil {
			return sdkSession.FrontendConfig{}, authRequestError(err)
		}
		if deadline != nil {
			return sdkSession.FrontendConfig{}, authFailure("browser_bootstrap_timeout", nil)
		}
		if e != nil {
			code := "browser_bootstrap_failed"
			var ae *sdkErrs.PinAuthError
			if errors.As(e, &ae) {
				switch ae.Code {
				case "browser_owner_required", "unsupported_browser_state", "browser_bootstrap_unavailable", "unsafe_bootstrap_request", "browser_bootstrap_timeout":
					code = ae.Code
				}
			}
			return sdkSession.FrontendConfig{}, authFailure(code, nil)
		}
		result, e = sdkTransport.NewBrowserBootstrapResult(result)
		if e != nil {
			return sdkSession.FrontendConfig{}, authFailure("unsupported_browser_state", nil)
		}
		if result.URL != sdkTransport.PublicBootstrapURL || result.Browser.AsMap()["user-agent"] == "" {
			return sdkSession.FrontendConfig{}, authFailure("unsupported_browser_state", nil)
		}
		if sdkTransport.IsBrowserCheck(result.HTML) {
			return sdkSession.FrontendConfig{}, authFailure("browser_check_required", nil)
		}
		rendered = &result
		html = result.HTML
	}
parse:
	var c sdkSession.FrontendConfig
	if f.primary {
		c, err = sdkSession.ParsePrimaryConfig(html)
	} else {
		c, err = sdkSession.ParsePINConfig(html)
	}
	html = ""
	if err != nil {
		return sdkSession.FrontendConfig{}, authFailure("invalid_frontend_config", nil)
	}
	var replacement sdkTransport.Transport
	var replacementOwner *authOwnedTransport
	if rendered != nil {
		current.Cookies = rendered.Cookies
		current.Browser = rendered.Browser
		current, err = current.Clone()
		if err != nil {
			return sdkSession.FrontendConfig{}, authFailure("unsupported_browser_state", nil)
		}
		replacement, err = f.options.TransportFactory(current, f.options.TransportOptions)
		if replacement != nil {
			replacementOwner = f.trackTransport(replacement)
		}
		if err != nil {
			if replacementOwner != nil {
				_ = replacementOwner.close()
			}
			if e := f.check(ctx); e != nil {
				return sdkSession.FrontendConfig{}, e
			}
			return sdkSession.FrontendConfig{}, authFailure("browser_bootstrap_failed", nil)
		}
		if replacement == nil || replacement.CookieJar() == nil {
			if replacement != nil {
				_ = replacementOwner.close()
			}
			return sdkSession.FrontendConfig{}, authFailure("unsupported_browser_state", nil)
		}
	}
	f.mu.Lock()
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		if replacement != nil {
			_ = replacementOwner.close()
		}
		return sdkSession.FrontendConfig{}, f.check(ctx)
	}
	var old *authOwnedTransport
	if replacement != nil {
		old = f.currentOwned
		f.bundle = current
		f.transport = replacement
		f.currentOwned = replacementOwner
	}
	f.resetProcess()
	f.config = &c
	f.mu.Unlock()
	if old != nil {
		_ = old.close()
	}
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

// Track constructed transports even if close/cancel prevents adoption. A failed
// cleanup remains owned and retryable; successful cleanup stays idempotent.
func (f *authFlow) trackTransport(tr sdkTransport.Transport) *authOwnedTransport {
	owned := &authOwnedTransport{transport: tr}
	f.mu.Lock()
	f.owned = append(f.owned, owned)
	if f.closed {
		f.closeComplete = false
	}
	f.mu.Unlock()
	return owned
}
