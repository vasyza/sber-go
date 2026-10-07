package sber

import (
	"context"
	"unicode/utf8"
)

// The operation gate coalesces discovery, and protects the host pair against
// concurrent renewal and mutation sequences. Neither half is adopted on error.
func (c *SberClient) resolveAPIBase(ctx context.Context) (string, error) {
	if e := c.check(ctx); e != nil {
		return "", e
	}
	if c.core().apiResolved {
		return c.core().bundle.APIBase, nil
	}
	shell, e := c.getHTML(ctx, AppOrigin+"/app/main")
	if e != nil {
		return "", e
	}
	web, ok := UFSHostFromAppShell(shell)
	if !ok {
		return "", &APIError{Message: "invalid runtime web origin"}
	}
	main, e := c.getHTML(ctx, web+"/main")
	if e != nil {
		return "", e
	}
	api, ok := APIBaseFromMainHTML(main)
	if !ok {
		return "", &APIError{Message: "invalid runtime API origin"}
	}
	c.core().mu.Lock()
	defer c.core().mu.Unlock()
	if c.core().closed {
		return "", ErrClosed
	}
	if ctx.Err() != nil {
		return "", transportFailure(ctx.Err())
	}
	c.core().bundle.APIBase = api
	c.core().bundle.WebBase = web
	c.core().apiResolved = true
	return api, nil
}
func (c *SberClient) getHTML(ctx context.Context, target string) (string, error) {
	if e := c.check(ctx); e != nil {
		return "", e
	}
	r, e := c.core().transport.Get(ctx, target, RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd"})
	if e != nil {
		return "", clientSafeError(e)
	}
	if e = c.check(ctx); e != nil {
		return "", e
	}
	if r == nil {
		return "", &APIError{Message: "missing discovery response"}
	}
	switch r.StatusCode {
	case 301, 302, 303, 307, 308, 401, 403:
		return "", &AuthenticationExpired{}
	}
	if r.StatusCode != 200 {
		return "", &APIError{StatusCode: r.StatusCode}
	}
	if !utf8.Valid(r.Content) {
		return "", &APIError{Message: "invalid discovery text"}
	}
	return string(r.Content), nil
}
