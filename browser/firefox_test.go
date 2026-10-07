package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	pw "github.com/mxschmitt/playwright-go"
	sber "github.com/vasyza/sber-go"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublicBootstrapRequestAllowlist(t *testing.T) {
	target := sber.PublicBootstrapURL
	for _, tt := range []struct {
		url, method, kind string
		main, allowed     bool
	}{
		{target, "GET", "document", true, true}, {target, "POST", "document", true, false},
		{"https://online.sberbank.ru/TSPD/synthetic", "GET", "document", false, true},
		{"https://online.sberbank.ru/CSAFront/synthetic.js?v=1", "GET", "script", false, true},
		{"https://online.sberbank.ru/CSAFront/authMainJson.do.js", "GET", "script", false, false},
		{"https://online.sberbank.ru/CSAFront/%61pi/v1/pin/begin", "GET", "fetch", false, false},
		{"https://online.sberbank.ru/captcha.png", "GET", "image", false, false},
		{"https://evil.invalid/static.js", "GET", "script", false, false},
		{"http://online.sberbank.ru/TSPD/synthetic.js", "GET", "script", false, false},
		{target, "GET", "document", false, false},
		{"https://online.sberbank.ru/TSPD/synthetic", "GET", "document", true, false},
	} {
		if got := publicBootstrapRequest(tt.url, tt.method, tt.kind, target, tt.main); got != tt.allowed {
			t.Fatal("unsafe/incompatible route decision")
		}
	}
}
func TestPlaywrightCookieRoundtripAndPartitionRejection(t *testing.T) {
	ss := pw.SameSiteAttribute("Strict")
	raw := pw.Cookie{Name: "synthetic", Value: "synthetic-secret", Domain: ".online.sberbank.ru", Path: "/CSAFront", Expires: 4102444800, HttpOnly: true, Secure: true, SameSite: &ss}
	record, err := cookieFromPlaywright(raw)
	if err != nil {
		t.Fatal(err)
	}
	if record.HostOnly || record.Domain != "online.sberbank.ru" || !record.HTTPOnly || *record.SameSite != "Strict" || *record.Expires != 4102444800 {
		t.Fatal("browser cookie metadata lost")
	}
	exported := cookieToPlaywright(record)
	if *exported.Domain != ".online.sberbank.ru" || *exported.SameSite != ss || *exported.Expires != raw.Expires || !*exported.HttpOnly {
		t.Fatal("seed metadata lost")
	}
	for _, edit := range []func(*pw.Cookie){func(c *pw.Cookie) { x := "https://partition.invalid"; c.PartitionKey = &x }, func(c *pw.Cookie) { c.Expires = math.NaN() }, func(c *pw.Cookie) { c.Expires = math.Inf(1) }, func(c *pw.Cookie) { c.Expires = 0 }, func(c *pw.Cookie) { s := pw.SameSiteAttribute("invalid"); c.SameSite = &s }} {
		bad := raw
		edit(&bad)
		_, err := cookieFromPlaywright(bad)
		var pin *sber.PinAuthError
		if !errors.As(err, &pin) || pin.Code != "unsupported_browser_state" {
			t.Fatal("unsupported metadata accepted")
		}
	}
	raw.Expires = -1
	raw.Domain = "online.sberbank.ru"
	raw.SameSite = nil
	r, err := cookieFromPlaywright(raw)
	if err != nil || r.Expires != nil || !r.HostOnly {
		t.Fatal("session/host-only metadata changed")
	}
}
func TestFirefoxSetupAndSafeEnvironment(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	provider, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Bootstrap(context.Background(), sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin}, "https://evil.invalid/")
	var pin *sber.PinAuthError
	if !errors.As(err, &pin) || pin.Code != "unsafe_bootstrap_request" {
		t.Fatal("unsafe target not rejected")
	}
	if err := os.Chmod(profile, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile}); err == nil {
		t.Fatal("public profile accepted")
	}
	os.Chmod(profile, 0700)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(profile, link); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: link}); err == nil {
		t.Fatal("profile symlink accepted")
	}
	for _, timeout := range []time.Duration{-time.Second, 121 * time.Second} {
		if _, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile, Timeout: timeout}); err == nil {
			t.Fatal("unbounded timeout accepted")
		}
	}
	env := safeBrowserEnvironment([]string{"DISPLAY=:99", "HTTPS_PROXY=synthetic", "all_proxy=synthetic", "MOZ_DISABLE_CONTENT_SANDBOX=1", "DEBUGP=1", "DEBUG=pw:api", "PWDEBUG=1", "SSLKEYLOGFILE=synthetic", "MOZ_LOG=synthetic", "PLAYWRIGHT_FIREFOX_POLICIES_JSON=synthetic"})
	if len(env) != 1 || env["DISPLAY"] != ":99" {
		t.Fatal("dangerous environment propagated")
	}
	raw, _ := json.Marshal(provider)
	if strings.Contains(string(raw), profile) {
		t.Fatal("provider serialized internal profile")
	}
}

type fakeFrame struct {
	pw.Frame
	parent pw.Frame
}

func (f *fakeFrame) ParentFrame() pw.Frame { return f.parent }

type fakeRequest struct {
	pw.Request
	u, method, kind string
	frame           pw.Frame
}

func (r *fakeRequest) URL() string          { return r.u }
func (r *fakeRequest) Method() string       { return r.method }
func (r *fakeRequest) ResourceType() string { return r.kind }
func (r *fakeRequest) Frame() pw.Frame      { return r.frame }
func (r *fakeRequest) AllHeaders() (map[string]string, error) {
	return map[string]string{"user-agent": "SyntheticFirefox/1.0", "accept-language": "ru-RU", "cookie": "synthetic-secret"}, nil
}

type fakeRoute struct {
	pw.Route
	request            pw.Request
	continued, aborted bool
}

func (r *fakeRoute) Request() pw.Request                       { return r.request }
func (r *fakeRoute) Continue(...pw.RouteContinueOptions) error { r.continued = true; return nil }
func (r *fakeRoute) Abort(...string) error                     { r.aborted = true; return nil }

type fakeResponse struct {
	pw.Response
	r      pw.Request
	status int
}

func (r *fakeResponse) URL() string         { return r.r.URL() }
func (r *fakeResponse) Request() pw.Request { return r.r }
func (r *fakeResponse) Status() int         { return r.status }

type embeddedLocator = pw.Locator

type fakeLocator struct {
	embeddedLocator
	visible bool
}

func (l *fakeLocator) Count() (int, error)                                   { return 2, nil }
func (l *fakeLocator) Nth(i int) pw.Locator                                  { return &fakeLocator{visible: i == 1 && l.visible} }
func (l *fakeLocator) IsVisible(...pw.LocatorIsVisibleOptions) (bool, error) { return l.visible, nil }

type fakeBrowserContext struct {
	pw.BrowserContext
	mu                  sync.Mutex
	once                sync.Once
	closed              chan struct{}
	cleared, pageClosed bool
	seeded              []pw.OptionalCookie
	page                *fakePage
	handler             func(pw.Route)
	response            func(pw.Response)
	websocket           bool
	raw                 []pw.Cookie
}

func (c *fakeBrowserContext) ClearCookies(...pw.BrowserContextClearCookiesOptions) error {
	c.cleared = true
	return nil
}
func (c *fakeBrowserContext) AddCookies(v []pw.OptionalCookie) error {
	if !c.cleared {
		return errors.New("fixture not cleared")
	}
	c.seeded = v
	return nil
}
func (c *fakeBrowserContext) Route(_ any, h func(pw.Route), _ ...int) error {
	c.handler = h
	return nil
}
func (c *fakeBrowserContext) RouteWebSocket(_ any, _ func(pw.WebSocketRoute)) error {
	c.websocket = true
	return nil
}
func (c *fakeBrowserContext) OnResponse(h func(pw.Response)) { c.response = h }
func (c *fakeBrowserContext) NewPage() (pw.Page, error)      { return c.page, nil }
func (c *fakeBrowserContext) Cookies(...string) ([]pw.Cookie, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pageClosed {
		return nil, errors.New("must freeze scripts before snapshot")
	}
	return c.raw, nil
}
func (c *fakeBrowserContext) Close(...pw.BrowserContextCloseOptions) error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

type fakePage struct {
	pw.Page
	c                           *fakeBrowserContext
	frame                       pw.Frame
	html                        string
	captcha, blocking, executed bool
	readErr                     error
}

func (p *fakePage) MainFrame() pw.Frame { return p.frame }
func (p *fakePage) URL() string         { return sber.PublicBootstrapURL }
func (p *fakePage) Goto(u string, _ ...pw.PageGotoOptions) (pw.Response, error) {
	r := &fakeRequest{u: u, method: "GET", kind: "document", frame: p.frame}
	route := &fakeRoute{request: r}
	p.c.handler(route)
	if !route.continued || route.aborted {
		return nil, errors.New("fixture route aborted")
	}
	res := &fakeResponse{r: r, status: 200}
	p.c.response(res)
	return res, nil
}
func (p *fakePage) Content() (string, error) {
	if p.blocking {
		<-p.c.closed
		return "", errors.New("synthetic-secret closed")
	}
	if p.readErr != nil {
		return "", p.readErr
	}
	return p.html, nil
}
func (p *fakePage) Locator(string, ...pw.PageLocatorOptions) pw.Locator {
	return &fakeLocator{visible: p.captcha}
}
func (p *fakePage) Evaluate(string, ...any) (any, error) { return p.executed, nil }
func (p *fakePage) Close(...pw.PageCloseOptions) error {
	p.c.mu.Lock()
	p.c.pageClosed = true
	p.c.mu.Unlock()
	return nil
}
func fakeProvider(t *testing.T) (*FirefoxBootstrap, *fakeBrowserContext, *pw.BrowserTypeLaunchPersistentContextOptions) {
	t.Helper()
	profile := filepath.Join(t.TempDir(), "firefox")
	os.Mkdir(profile, 0700)
	provider, err := NewFirefoxBootstrap(FirefoxOptions{ProfileDir: profile, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	c := &fakeBrowserContext{closed: make(chan struct{}), raw: []pw.Cookie{{Name: "synthetic", Value: "synthetic-secret", Domain: "online.sberbank.ru", Path: "/", Secure: true, HttpOnly: true, Expires: -1}}}
	p := &fakePage{c: c, frame: &fakeFrame{}, html: `<script>window.config = {processId:"synthetic"};</script>`}
	c.page = p
	options := new(pw.BrowserTypeLaunchPersistentContextOptions)
	provider.launch = func(o pw.BrowserTypeLaunchPersistentContextOptions) (pw.BrowserContext, func(), error) {
		*options = o
		return c, func() {}, nil
	}
	return provider, c, options
}
func TestFirefoxOrdinaryRenderAndFrozenSnapshot(t *testing.T) {
	provider, c, o := fakeProvider(t)
	b := sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin, Cookies: []sber.CookieRecord{{Name: "synthetic", Value: "synthetic-seed", Domain: "online.sberbank.ru", Path: "/CSAFront", Secure: true, HostOnly: true}}}
	result, err := provider.Bootstrap(context.Background(), b, sber.PublicBootstrapURL)
	if err != nil {
		t.Fatal(err)
	}
	if result.HTML != c.page.html || result.URL != sber.PublicBootstrapURL || result.Browser.AsMap()["user-agent"] != "SyntheticFirefox/1.0" || len(result.Cookies) != 1 {
		t.Fatal("snapshot not bound to rendered state")
	}
	if !c.cleared || !c.pageClosed || !c.websocket || len(c.seeded) != 1 {
		t.Fatal("lifecycle or socket guard missing")
	}
	select {
	case <-c.closed:
	default:
		t.Fatal("context leaked")
	}
	if o.IgnoreHttpsErrors == nil || *o.IgnoreHttpsErrors || o.JavaScriptEnabled == nil || !*o.JavaScriptEnabled || o.AcceptDownloads == nil || *o.AcceptDownloads || o.ServiceWorkers == nil || *o.ServiceWorkers != "block" || len(o.Args) > 0 || o.UserAgent != nil || o.Proxy != nil || o.HttpCredentials != nil || o.RecordHarPath != nil {
		t.Fatal("ordinary verified browser invariants weakened")
	}
	if o.FirefoxUserPrefs["network.proxy.type"] != 0 || o.FirefoxUserPrefs["network.http.redirection-limit"] != 0 {
		t.Fatal("redirect/proxy policy missing")
	}
	otherRoot := &fakeRoute{request: &fakeRequest{u: sber.PublicBootstrapURL, method: "GET", kind: "document", frame: &fakeFrame{}}}
	c.handler(otherRoot)
	if !otherRoot.aborted {
		t.Fatal("popup created unrelated auth process")
	}
	child := &fakeRoute{request: &fakeRequest{u: sber.AppOrigin + "/TSPD/synthetic", method: "GET", kind: "document", frame: &fakeFrame{parent: c.page.frame}}}
	c.handler(child)
	if !child.continued {
		t.Fatal("ordinary first-party TSPD iframe blocked")
	}
}
func TestFirefoxFailuresAreBoundedRedactedAndCleaned(t *testing.T) {
	for _, kind := range []string{"captcha", "timeout", "exception", "cancel", "executed_config"} {
		t.Run(kind, func(t *testing.T) {
			provider, c, _ := fakeProvider(t)
			provider.options.Timeout = 30 * time.Millisecond
			switch kind {
			case "captcha":
				c.page.captcha = true
			case "timeout", "cancel":
				c.page.blocking = true
			case "exception":
				c.page.readErr = errors.New("synthetic-cookie-html-secret")
			case "executed_config":
				c.page.html = "<html>synthetic dynamic config</html>"
				c.page.executed = true
			}
			ctx := context.Background()
			if kind == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, err := provider.Bootstrap(ctx, sber.SessionBundle{APIBase: sber.AppOrigin, WebBase: sber.AppOrigin}, sber.PublicBootstrapURL)
			if kind == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation not propagated")
				}
				return
			}
			want := map[string]string{"captcha": "browser_owner_required", "timeout": "browser_bootstrap_timeout", "exception": "browser_bootstrap_failed", "executed_config": "unsupported_browser_state"}[kind]
			var pin *sber.PinAuthError
			if !errors.As(err, &pin) || pin.Code != want {
				t.Fatal("safe typed failure missing")
			}
			select {
			case <-c.closed:
			case <-time.After(time.Second):
				t.Fatal("context not cleaned")
			}
		})
	}
}

func TestFoundationPublicPathTraversal(t *testing.T) {
	const traversal = "/TSPD/%2e%2e%2fuoh-bh/v1/operations/list"
	wire := make(chan string, 1)
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire <- r.RequestURI
		fmt.Fprint(w, path.Clean(r.URL.Path))
	}))
	defer origin.Close()
	// Only localhost is contacted. This is an ordinary server's routing
	// normalization, not an assertion about any bank's implementation.
	res, err := origin.Client().Get(origin.URL + traversal)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if got := <-wire; got != traversal {
		t.Fatalf("wire fixture changed: %q", got)
	}
	if string(normalized) != "/uoh-bh/v1/operations/list" {
		t.Fatalf("wire normalization fixture invalid: %q", normalized)
	}
	for _, p := range []string{
		traversal, "/TSPD/../uoh-bh/v1/operations/list", "/TSPD/%2e%2e/uoh-bh/v1/operations/list",
		"/TSPD/%252e%252e%252fuoh-bh/v1/operations/list", "/TSPD/%25252e%25252e%25252fuoh-bh/v1/operations/list",
		"/TSPD/foo%2fbar", "/TSPD/foo%5cbar", "/TSPD/%255c..%255cauthMainJson.do",
		"/TSPD/./synthetic", "/TSPD//synthetic", "/CSAFront/../private.js",
	} {
		for _, resource := range []string{"fetch", "document", "script"} {
			if publicBootstrapRequest(sber.AppOrigin+p, "GET", resource, sber.PublicBootstrapURL, false) {
				t.Errorf("ambiguous/traversing path allowed: %s (%s)", p, resource)
			}
		}
	}
	for _, p := range []string{"/TSPD/synthetic", "/TSPD/synthetic.js?cache=1", "/CSAFront/synthetic.css", "/CSAFront/synthetic%20asset.js", "/CSAFront/%73ynthetic.js"} {
		if !publicBootstrapRequest(sber.AppOrigin+p, "GET", "script", sber.PublicBootstrapURL, false) {
			t.Errorf("ordinary public path rejected: %s", p)
		}
	}
	if !publicBootstrapRequest(sber.PublicBootstrapURL, "GET", "document", sber.PublicBootstrapURL, true) {
		t.Error("ordinary main bootstrap rejected")
	}
}
