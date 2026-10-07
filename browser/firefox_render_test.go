package browser

// Fakes below model only driver calls. They are not live-bank evidence.
import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pw "github.com/mxschmitt/playwright-go"
	sber "github.com/vasyza/sber-go"
)

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
