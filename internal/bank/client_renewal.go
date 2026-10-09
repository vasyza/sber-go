package bank

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"

	sdkAuth "github.com/vasyza/sber-sdk/internal/auth"
	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

func clientNormalizeOptions(o ClientOptions) (ClientOptions, error) {
	if o.TransportOptions.Retry != 0 || o.AuthOptions.TransportOptions.Retry != 0 {
		return o, &sdkErrs.TransportError{Code: "retry_forbidden"}
	}
	o.AuthOptions.Deviceprint = sdkSession.CloneString(o.AuthOptions.Deviceprint)
	o.AuthOptions.AntifraudDeviceprint = sdkSession.CloneString(o.AuthOptions.AntifraudDeviceprint)
	o.AuthOptions.Browser.Headers = append([]sdkSession.BrowserHeader(nil), o.AuthOptions.Browser.Headers...)
	if o.Monotonic == nil {
		o.Monotonic = time.Now
	}
	o.TransportOptions.AllowUnready = false
	defaultTransportFactory := o.TransportFactory == nil
	if defaultTransportFactory {
		o.TransportFactory = func(b sdkSession.SessionBundle, to sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			return sdkTransport.NewHTTPTransport(b, to)
		}
	}
	if o.Renewal == nil {
		o.Renewal = clientPINLogin
	}
	o.AuthOptions.TransportOptions = o.TransportOptions
	o.AuthOptions.TransportOptions.AllowUnready = true
	if o.AuthOptions.TransportFactory == nil {
		if defaultTransportFactory {
			o.AuthOptions.TransportFactory = func(b sdkSession.SessionBundle, to sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return sdkTransport.NewAuthenticationTransport(b, to)
			}
		} else {
			// Preserve deliberately injected caller factories. Native defaults
			// use auth's connection policy rather than the business policy.
			o.AuthOptions.TransportFactory = sdkAuth.AuthTransportFactory(o.TransportFactory)
		}
	}
	return o, nil
}
func clientPINLogin(ctx context.Context, b sdkSession.SessionBundle, provider sdkAuth.PINProvider, o sdkAuth.AuthOptions) (bundle sdkSession.SessionBundle, err error) {
	if provider == nil {
		return bundle, &sdkErrs.MissingSession{Message: "PIN provider required"}
	}
	if ctx == nil {
		return bundle, &sdkErrs.TransportError{Code: "invalid_context"}
	}
	if ctx.Err() != nil {
		return bundle, sdkTransport.TransportFailure(ctx.Err())
	}
	pin, e := provider(ctx)
	if e != nil {
		return bundle, clientSafeError(e)
	}
	defer func() { pin = "" }()
	if ctx.Err() != nil {
		return bundle, sdkTransport.TransportFailure(ctx.Err())
	}
	if pin == "" {
		return bundle, &sdkErrs.MissingSession{Message: "PIN provider returned no PIN"}
	}
	auth, e := sdkAuth.NewPINAuth(b, o)
	if e != nil {
		return bundle, clientSafeError(e)
	}
	defer func() {
		if e := auth.Close(); e != nil {
			primary := err
			if primary == nil {
				primary = clientSafeError(e)
			}
			bundle = sdkSession.SessionBundle{}
			err = &ClientCleanupError{cause: primary, cleanup: auth.Close}
		}
	}()
	return auth.Login(ctx, pin, sdkAuth.CaptchaAnswer{})
}

// NewSberClientFromPINProfile keeps a full remembered browser profile. A ready
// profile is opened without login; an unready profile renews once and is saved
// before its first business transport exists. All paths are caller-explicit.
func NewSberClientFromPINProfile(ctx context.Context, path string, provider sdkAuth.PINProvider, o ClientOptions, load ...sdkSession.SessionLoadOptions) (*SberClient, error) {
	if ctx == nil {
		return nil, &sdkErrs.TransportError{Code: "invalid_context"}
	}
	if ctx.Err() != nil {
		return nil, sdkTransport.TransportFailure(ctx.Err())
	}
	b, e := sdkSession.LoadSessionBundle(path, load...)
	if e != nil {
		return nil, e
	}
	if b.Deviceprint == nil || provider == nil {
		return nil, &sdkErrs.MissingSession{Message: "PIN profile device identity and provider required"}
	}
	defaultPIN := o.Renewal == nil
	o, e = clientNormalizeOptions(o)
	if e != nil {
		return nil, e
	}
	o.SessionPath = path
	var initial *clientInitialOwners
	if _, e = b.ToSeed(true); e != nil {
		authOptions := o.AuthOptions
		if defaultPIN {
			initial = clientNewInitialOwners(o.Transport)
			authOptions = clientGuardAuthOptions(authOptions, initial.claim)
		}
		refreshed, e := o.Renewal(ctx, b, provider, authOptions)
		if e != nil {
			return nil, clientSafeError(e)
		}
		if ctx.Err() != nil {
			return nil, sdkTransport.TransportFailure(ctx.Err())
		}
		refreshed, e = refreshed.Clone()
		if e != nil {
			return nil, e
		}
		if _, e = refreshed.ToSeed(true); e != nil {
			return nil, e
		}
		if e = refreshed.Save(path); e != nil {
			return nil, e
		}
		if ctx.Err() != nil {
			return nil, sdkTransport.TransportFailure(ctx.Err())
		}
		b = refreshed
		o.Transport = nil
	}
	c, e := clientNewSberClient(b, o, initial)
	if e != nil {
		return nil, e
	}
	if e = c.check(ctx); e != nil {
		if cleanup := c.Close(); cleanup != nil {
			return nil, &ClientCleanupError{cause: e, cleanup: c.Close}
		}
		return nil, e
	}
	c.core().pinProvider = provider
	c.core().defaultPINRenewal = defaultPIN
	c.core().credentialsOnly = false
	return c, nil
}

func clientTransportPresent(tr sdkTransport.Transport) bool {
	if tr == nil {
		return false
	}
	value := reflect.ValueOf(tr)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !value.IsNil()
	}
	return true
}

type clientOwnedTransport struct {
	mu        sync.Mutex
	transport sdkTransport.Transport
	identity  sdkTransport.Transport
	closed    bool
}

func (o *clientOwnedTransport) close() error {
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
func (c *SberClient) track(tr sdkTransport.Transport) *clientOwnedTransport {
	owned := clientNewOwnedTransport(tr)
	c.core().mu.Lock()
	c.core().owned = append(c.core().owned, owned)
	c.core().mu.Unlock()
	return owned
}
func (c *SberClient) alreadyOwns(tr sdkTransport.Transport) bool {
	// A separately closing candidate must have verifiable stable identity.
	// Reuse or uncertainty is rejected before acquiring cleanup ownership.
	identity := clientClosingIdentity(tr)
	if identity == nil {
		return true
	}
	c.core().mu.Lock()
	defer c.core().mu.Unlock()
	for _, owned := range c.core().owned {
		if owned.identity == nil || owned.identity == identity || clientSameTransport(owned.transport, tr) {
			return true
		}
	}
	return false
}
func (c *SberClient) renewProfile(ctx context.Context) error {
	if e := c.check(ctx); e != nil {
		return e
	}
	c.core().mu.Lock()
	provider := c.core().pinProvider
	c.core().mu.Unlock()
	if provider == nil || c.core().sessionPath == "" {
		return &sdkErrs.AuthenticationExpired{}
	}
	b, e := c.ExportSession()
	if e != nil {
		return e
	}
	authOptions := c.core().options.AuthOptions
	if c.core().defaultPINRenewal {
		authOptions = clientGuardAuthOptions(authOptions, c.claimAuthTransport)
	}
	refreshed, e := c.core().options.Renewal(ctx, b, provider, authOptions)
	if e != nil {
		return clientSafeError(e)
	}
	if e = c.check(ctx); e != nil {
		return e
	}
	refreshed, e = refreshed.Clone()
	if e != nil {
		return e
	}
	if _, e = refreshed.ToSeed(true); e != nil {
		return e
	}
	factoryBundle, _ := refreshed.Clone()
	replacement, e := c.core().options.TransportFactory(factoryBundle, c.core().options.TransportOptions)
	// A bad injectable factory cannot trick retirement into closing the current
	// committed generation or reopen an already-retired owned instance.
	if clientTransportPresent(replacement) && c.alreadyOwns(replacement) {
		return &sdkErrs.TransportError{Code: "reused_transport"}
	}
	var owner *clientOwnedTransport
	if clientTransportPresent(replacement) {
		owner = c.track(replacement)
	}
	discard := func() {
		if owner != nil {
			_ = owner.close()
		}
	}
	if e != nil {
		discard()
		return clientSafeError(e)
	}
	if !clientTransportPresent(replacement) || replacement.CookieJar() == nil {
		discard()
		return &sdkErrs.MissingSession{Message: "renewal transport cookie jar required"}
	}
	if e = c.check(ctx); e != nil {
		discard()
		return e
	}
	if e = refreshed.Save(c.core().sessionPath); e != nil {
		discard()
		return e
	}
	c.core().mu.Lock()
	if c.core().closed || ctx.Err() != nil {
		c.core().mu.Unlock()
		discard()
		return c.check(ctx)
	}
	previous := c.core().currentOwned
	c.core().bundle = refreshed
	c.core().transport = replacement
	c.core().currentOwned = owner
	c.core().lastWarmup = nil
	c.core().apiResolved = refreshed.APIBase != sdkSession.AppOrigin
	c.core().mu.Unlock()
	// A committed generation is never rolled back because retiring the old
	// transport failed. No business read retries cleanup; only Close does.
	_ = previous.close()
	return c.check(ctx)
}

// The arrival epoch changes only after the entire renewal + read retry. This
// includes failed renewal, and suppresses a waiter arriving during the retry.
func (c *SberClient) withReadRenewal(ctx context.Context, attempt func(context.Context) error) error {
	c.core().mu.Lock()
	arrival := c.core().reauthEpoch
	c.core().mu.Unlock()
	ctx, done, e := c.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	e = attempt(ctx)
	var expired *sdkErrs.AuthenticationExpired
	if !errors.As(e, &expired) {
		return e
	}
	c.core().mu.Lock()
	canRenew := c.core().pinProvider != nil && arrival == c.core().reauthEpoch
	c.core().mu.Unlock()
	if !canRenew {
		return e
	}
	defer func() { c.core().mu.Lock(); c.core().reauthEpoch++; c.core().mu.Unlock() }()
	if e = c.renewProfile(ctx); e != nil {
		return e
	}
	return attempt(ctx)
}
