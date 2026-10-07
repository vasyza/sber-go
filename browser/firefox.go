// Package browser contains opt-in ordinary browser rendering. Importing sber
// does not launch a browser, install a driver, or require a Python runtime.
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	pw "github.com/mxschmitt/playwright-go"
	sber "github.com/vasyza/sber-go"
	sdkProxy "github.com/vasyza/sber-go/internal/proxy"
)

// FirefoxOptions requires a dedicated private profile. Its NSS trust must be
// provisioned/verified by the caller. DriverDir + FirefoxExecutable explicitly
// select a matching preinstalled Playwright driver/browser; no implicit install,
// global cache discovery, owner profile lookup, or TLS bypass.
type FirefoxOptions struct {
	ProfileDir        string
	DriverDir         string
	FirefoxExecutable string
	Headed            bool
	Timeout           time.Duration
	MaxRequests       int
	Proxy             sber.ProxyOptions
}

func (o FirefoxOptions) String() string               { return "FirefoxOptions(<redacted>)" }
func (o FirefoxOptions) Format(f fmt.State, _ rune)   { _, _ = f.Write([]byte(o.String())) }
func (o FirefoxOptions) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

type FirefoxBootstrap struct {
	options FirefoxOptions
	launch  func(pw.BrowserTypeLaunchPersistentContextOptions) (pw.BrowserContext, func(), error)
}

var _ sber.BrowserBootstrapProvider = (*FirefoxBootstrap)(nil)

// The binding mutates a package-global logger on startup, so serialize complete
// renders without changing process-wide environment or log configuration.
var driverGate = make(chan struct{}, 1)

func NewFirefoxBootstrap(o FirefoxOptions) (*FirefoxBootstrap, error) {
	proxy, err := o.Proxy.Normalize()
	if err != nil {
		return nil, browserError("unsupported_browser_state")
	}
	o.Proxy = proxy
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	if o.Timeout < 0 || o.Timeout > 120*time.Second {
		return nil, browserError("unsupported_browser_state")
	}
	if o.MaxRequests == 0 {
		o.MaxRequests = 128
	}
	if o.MaxRequests < 1 || o.MaxRequests > 512 {
		return nil, browserError("unsupported_browser_state")
	}
	if err := privateProfile(o.ProfileDir); err != nil {
		return nil, err
	}
	p := &FirefoxBootstrap{options: o}
	p.launch = func(options pw.BrowserTypeLaunchPersistentContextOptions) (pw.BrowserContext, func(), error) {
		if o.DriverDir == "" || o.FirefoxExecutable == "" || !filepath.IsAbs(o.DriverDir) || !filepath.IsAbs(o.FirefoxExecutable) {
			return nil, nil, browserError("browser_bootstrap_unavailable")
		}
		// Reject binding debug/raw-protocol logging and alternate executables instead
		// of temporarily modifying environment shared by other goroutines.
		for _, name := range []string{"DEBUG", "DEBUGP", "PWDEBUG", "PLAYWRIGHT_CLI_PATH", "PLAYWRIGHT_NODEJS_PATH", "SSLKEYLOGFILE"} {
			if os.Getenv(name) != "" {
				return nil, nil, browserError("unsupported_browser_state")
			}
		}
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		runtime, err := pw.Run(&pw.RunOptions{DriverDirectory: o.DriverDir, Verbose: false, Stdout: io.Discard, Stderr: io.Discard, Logger: logger})
		if err != nil {
			return nil, nil, browserError("browser_bootstrap_unavailable")
		}
		cleanup := func() { _ = runtime.Stop() }
		c, err := runtime.Firefox.LaunchPersistentContext(o.ProfileDir, options)
		if err != nil {
			cleanup()
			return nil, nil, browserError("browser_bootstrap_failed")
		}
		return c, cleanup, nil
	}
	return p, nil
}
func privateProfile(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return browserError("unsupported_browser_state")
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Getuid()) {
		return browserError("unsupported_browser_state")
	}
	return nil
}
func browserError(code string) *sber.PinAuthError        { return &sber.PinAuthError{Code: code} }
func (p *FirefoxBootstrap) String() string               { return "FirefoxBootstrap(<redacted>)" }
func (p *FirefoxBootstrap) Format(f fmt.State, v rune)   { _, _ = f.Write([]byte(p.String())) }
func (p *FirefoxBootstrap) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

func safeBrowserEnvironment(values []string) map[string]string {
	env := map[string]string{}
	for _, value := range values {
		name, v, ok := strings.Cut(value, "=")
		if !ok {
			continue
		}
		if strings.HasSuffix(strings.ToLower(name), "_proxy") || strings.HasPrefix(name, "MOZ_DISABLE_") {
			continue
		}
		switch name {
		case "DEBUG", "DEBUGP", "PWDEBUG", "SSLKEYLOGFILE", "MOZ_LOG", "MOZ_LOG_FILE", "PLAYWRIGHT_FIREFOX_POLICIES_JSON":
			continue
		}
		env[name] = v
	}
	return env
}
func (p *FirefoxBootstrap) launchOptions() pw.BrowserTypeLaunchPersistentContextOptions {
	options := pw.BrowserTypeLaunchPersistentContextOptions{Headless: pw.Bool(!p.options.Headed), Timeout: pw.Float(float64(p.options.Timeout / time.Millisecond)),
		JavaScriptEnabled: pw.Bool(true), IgnoreHttpsErrors: pw.Bool(false), AcceptDownloads: pw.Bool(false), ServiceWorkers: pw.ServiceWorkerPolicyBlock,
		Env: safeBrowserEnvironment(os.Environ()), FirefoxUserPrefs: map[string]any{"network.proxy.type": 0, "network.http.redirection-limit": 0},
	}
	if p.options.Proxy.URL != "" {
		delete(options.FirefoxUserPrefs, "network.proxy.type")
		options.Proxy = playwrightProxy(p.options.Proxy)
	}
	if p.options.FirefoxExecutable != "" {
		options.ExecutablePath = pw.String(p.options.FirefoxExecutable)
	}
	return options
}

func playwrightProxy(o sber.ProxyOptions) *pw.Proxy {
	proxy := &pw.Proxy{Server: o.URL}
	if o.Username != "" {
		proxy.Username, proxy.Password = pw.String(o.Username), pw.String(o.Password)
	}
	return proxy
}

func (p *FirefoxBootstrap) browserLaunchOptions(ctx context.Context) (pw.BrowserTypeLaunchPersistentContextOptions, func(), error) {
	options := p.launchOptions()
	if strings.HasPrefix(p.options.Proxy.URL, "socks5://") && p.options.Proxy.Username != "" {
		bridge, err := sdkProxy.StartBridge(ctx, p.options.Proxy, p.options.Timeout, 32)
		if err != nil {
			return options, nil, browserError("browser_bootstrap_failed")
		}
		options.Proxy = playwrightProxy(bridge.Options())
		return options, bridge.Close, nil
	}
	return options, func() {}, nil
}
func (p *FirefoxBootstrap) Bootstrap(ctx context.Context, bundle sber.SessionBundle, target string) (sber.BrowserBootstrapResult, error) {
	if target != sber.PublicBootstrapURL {
		return sber.BrowserBootstrapResult{}, browserError("unsafe_bootstrap_request")
	}
	if ctx == nil {
		return sber.BrowserBootstrapResult{}, browserError("unsupported_browser_state")
	}
	b, err := sber.NewSessionBundle(bundle)
	if err != nil {
		return sber.BrowserBootstrapResult{}, browserError("unsupported_browser_state")
	}
	if err := privateProfile(p.options.ProfileDir); err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	bounded, cancel := context.WithTimeout(ctx, p.options.Timeout)
	defer cancel()
	select {
	case driverGate <- struct{}{}:
	case <-bounded.Done():
		return sber.BrowserBootstrapResult{}, bootstrapContextError(bounded)
	}
	// The worker owns/relinquishes the gate; if a driver method returns late, its
	// canceled context prevents navigation and its state is never adopted.
	type outcome struct {
		r   sber.BrowserBootstrapResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() { <-driverGate }()
		r, err := p.render(bounded, b, target)
		done <- outcome{r, err}
	}()
	select {
	case <-bounded.Done():
		return sber.BrowserBootstrapResult{}, bootstrapContextError(bounded)
	case result := <-done:
		if bounded.Err() != nil {
			return sber.BrowserBootstrapResult{}, bootstrapContextError(bounded)
		}
		if result.err != nil {
			var pin *sber.PinAuthError
			if errors.As(result.err, &pin) {
				switch pin.Code {
				case "browser_owner_required", "unsupported_browser_state", "unsafe_bootstrap_request", "browser_bootstrap_unavailable":
					return sber.BrowserBootstrapResult{}, browserError(pin.Code)
				}
			}
			return sber.BrowserBootstrapResult{}, browserError("browser_bootstrap_failed")
		}
		return result.r, nil
	}
}
func bootstrapContextError(ctx context.Context) error {
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	return browserError("browser_bootstrap_timeout")
}

var configPattern = regexp.MustCompile(`\bwindow\s*\.\s*config\s*=`)
var staticPattern = regexp.MustCompile(`\.(?:js|css|png|jpg|jpeg|gif|svg|ico|woff2?|ttf)$`)

func publicBootstrapRequest(value, method, resource, target string, main bool) bool {
	if method != "GET" || !sber.IsOnlineURL(value) || strings.Contains(value, "\\") {
		return false
	}
	for _, r := range value {
		if r < 0x20 {
			return false
		}
	}
	u, err := url.Parse(value)
	if err != nil {
		return false
	}
	escaped := strings.ToLower(u.EscapedPath())
	// Different routers decode/clean paths in different orders. Do not grant a
	// public prefix to dot segments, encoded separators or another escape layer.
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") {
		return false
	}
	path, err := url.PathUnescape(escaped)
	if err != nil || strings.ContainsAny(path, "%\\") || strings.Contains(path, "//") {
		return false
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return false
		}
	}
	for _, r := range path {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	path = strings.ToLower(path)
	if strings.Contains(path, "/api/") || strings.Contains(path, "authmainjson.do") || strings.Contains(path, "captcha") {
		return false
	}
	if resource == "document" {
		if main {
			return value == target
		}
		return strings.HasPrefix(path, "/tspd/")
	}
	return strings.HasPrefix(path, "/tspd/") || staticPattern.MatchString(path)
}
func cookieToPlaywright(c sber.CookieRecord) pw.OptionalCookie {
	domain := c.Domain
	if !c.HostOnly {
		domain = "." + domain
	}
	r := pw.OptionalCookie{Name: c.Name, Value: c.Value, Domain: pw.String(domain), Path: pw.String(c.Path), Secure: pw.Bool(c.Secure), HttpOnly: pw.Bool(c.HTTPOnly)}
	if c.Expires != nil {
		r.Expires = pw.Float(float64(*c.Expires))
	}
	if c.SameSite != nil {
		ss := pw.SameSiteAttribute(*c.SameSite)
		r.SameSite = &ss
	}
	return r
}
func cookieFromPlaywright(c pw.Cookie) (sber.CookieRecord, error) {
	if c.PartitionKey != nil || math.IsNaN(c.Expires) || math.IsInf(c.Expires, 0) || (c.Expires != -1 && c.Expires <= 0) || c.Expires >= float64(math.MaxInt64) {
		return sber.CookieRecord{}, browserError("unsupported_browser_state")
	}
	r := sber.CookieRecord{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HttpOnly, HostOnly: !strings.HasPrefix(c.Domain, ".")}
	if c.Expires != -1 {
		expiry := int64(c.Expires)
		r.Expires = &expiry
	}
	if c.SameSite != nil {
		ss := string(*c.SameSite)
		r.SameSite = &ss
	}
	r, err := sber.NewCookieRecord(r)
	if err != nil {
		return sber.CookieRecord{}, browserError("unsupported_browser_state")
	}
	return r, nil
}
func (p *FirefoxBootstrap) render(ctx context.Context, bundle sber.SessionBundle, target string) (sber.BrowserBootstrapResult, error) {
	if ctx.Err() != nil {
		return sber.BrowserBootstrapResult{}, ctx.Err()
	}
	options, closeProxy, err := p.browserLaunchOptions(ctx)
	if err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	defer closeProxy()
	browser, cleanup, err := p.launch(options)
	if err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	defer cleanup()
	var closeOnce sync.Once
	closeContext := func() { closeOnce.Do(func() { _ = browser.Close() }) }
	defer closeContext()
	stop := context.AfterFunc(ctx, closeContext)
	defer stop()
	if ctx.Err() != nil {
		return sber.BrowserBootstrapResult{}, ctx.Err()
	}
	if err := browser.ClearCookies(); err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	cookies := make([]pw.OptionalCookie, 0, len(bundle.Cookies))
	for _, c := range bundle.Cookies {
		if c.Expires == nil || *c.Expires > time.Now().Unix() {
			cookies = append(cookies, cookieToPlaywright(c))
		}
	}
	if len(cookies) > 0 {
		if err := browser.AddCookies(cookies); err != nil {
			return sber.BrowserBootstrapResult{}, err
		}
	}
	var mu sync.Mutex
	var root pw.Frame
	var status int
	observed := map[string]string{}
	requests := 0
	budgetExceeded := false
	err = browser.Route("**/*", func(route pw.Route) {
		request := route.Request()
		frame := request.Frame()
		main := request.ResourceType() == "document" && frame != nil && frame.ParentFrame() == nil
		mu.Lock()
		owned := root
		requests++
		if requests > p.options.MaxRequests {
			budgetExceeded = true
		}
		exceeded := budgetExceeded
		mu.Unlock()
		if exceeded || main && frame != owned || !publicBootstrapRequest(request.URL(), request.Method(), request.ResourceType(), target, main) {
			_ = route.Abort()
			return
		}
		if request.ResourceType() == "document" && request.URL() == target && frame == owned {
			headers, err := request.AllHeaders()
			if err != nil {
				_ = route.Abort()
				return
			}
			mu.Lock()
			clear(observed)
			for _, name := range []string{"user-agent", "accept-language", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"} {
				if v, ok := headers[name]; ok {
					observed[name] = v
				}
			}
			mu.Unlock()
		}
		_ = route.Continue()
	})
	if err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	if err := browser.RouteWebSocket("**/*", func(socket pw.WebSocketRoute) { socket.Close() }); err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	browser.OnResponse(func(r pw.Response) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL() == target && r.Request().ResourceType() == "document" && r.Request().Frame() == root {
			status = r.Status()
		}
	})
	page, err := browser.NewPage()
	if err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	defer page.Close()
	mu.Lock()
	root = page.MainFrame()
	mu.Unlock()
	if _, err := page.Goto(target, pw.PageGotoOptions{WaitUntil: pw.WaitUntilStateDomcontentloaded, Timeout: pw.Float(float64(p.options.Timeout / time.Millisecond))}); err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	html := ""
	for {
		if ctx.Err() != nil {
			return sber.BrowserBootstrapResult{}, ctx.Err()
		}
		mu.Lock()
		exceeded := budgetExceeded
		mu.Unlock()
		if exceeded {
			return sber.BrowserBootstrapResult{}, browserError("browser_bootstrap_failed")
		}
		captcha := page.Locator(`iframe[src*="captcha" i], input[name*="captcha" i], img[src*="captcha" i]`)
		count, err := captcha.Count()
		if err == nil {
			for i := 0; i < count; i++ {
				visible, err := captcha.Nth(i).IsVisible()
				if err == nil && visible {
					return sber.BrowserBootstrapResult{}, browserError("browser_owner_required")
				}
			}
		}
		html, err = page.Content()
		if err == nil {
			if configPattern.MatchString(html) {
				break
			}
			executed, err := page.Evaluate(`() => typeof window.config === "object" && window.config !== null`)
			if err == nil && executed == true {
				return sber.BrowserBootstrapResult{}, browserError("unsupported_browser_state")
			}
		} else {
			var driverError *pw.Error
			if !errors.As(err, &driverError) {
				return sber.BrowserBootstrapResult{}, err
			}
		}
		select {
		case <-ctx.Done():
			return sber.BrowserBootstrapResult{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	finalURL := page.URL()
	if err := page.Close(); err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	raw, err := browser.Cookies()
	if err != nil {
		return sber.BrowserBootstrapResult{}, err
	}
	if len(raw) > sber.MaxCookies {
		return sber.BrowserBootstrapResult{}, browserError("unsupported_browser_state")
	}
	records := make([]sber.CookieRecord, 0, len(raw))
	for _, c := range raw {
		r, err := cookieFromPlaywright(c)
		if err != nil {
			return sber.BrowserBootstrapResult{}, err
		}
		records = append(records, r)
	}
	mu.Lock()
	currentStatus := status
	headers := make([]sber.BrowserHeader, 0, len(observed))
	for _, name := range []string{"user-agent", "accept-language", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"} {
		if value, ok := observed[name]; ok {
			headers = append(headers, sber.BrowserHeader{Name: name, Value: value})
		}
	}
	mu.Unlock()
	if currentStatus != 200 || finalURL != target {
		return sber.BrowserBootstrapResult{}, browserError("browser_bootstrap_failed")
	}
	return sber.NewBrowserBootstrapResult(sber.BrowserBootstrapResult{HTML: html, Cookies: records, Browser: sber.BrowserProfile{Headers: headers}, URL: finalURL})
}
