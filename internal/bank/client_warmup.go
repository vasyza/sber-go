package bank

import (
	"context"
	"time"

	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

const clientWarmUpDebounce = 60 * time.Second

// WarmUp is explicit. force bypasses the 60-second debounce; no business read
// depends on or triggers this legacy session keepalive endpoint.
func (c *SberClient) WarmUp(ctx context.Context, force bool) error {
	return c.withReadRenewal(ctx, func(ctx context.Context) error { return c.warmUpOnce(ctx, force) })
}
func (c *SberClient) warmUpOnce(ctx context.Context, force bool) error {
	if e := c.check(ctx); e != nil {
		return e
	}
	if !force && c.core().lastWarmup != nil && c.core().options.Monotonic().Sub(*c.core().lastWarmup) < clientWarmUpDebounce {
		return nil
	}
	if _, e := c.resolveAPIBase(ctx); e != nil {
		return e
	}
	r, e := c.core().transport.Post(ctx, c.core().bundle.WebBase+clientWarmUpPath, map[string]any{}, sdkTransport.RequestOptions{AcceptEncoding: "gzip, deflate, br, zstd"})
	if e != nil {
		return clientSafeError(e)
	}
	if e = c.check(ctx); e != nil {
		return e
	}
	if _, e = clientDecodeResponse(r, clientWarmUpPath); e != nil {
		return e
	}
	if e = c.saveRotatedSession(ctx); e != nil {
		return e
	}
	now := c.core().options.Monotonic()
	c.core().lastWarmup = &now
	return nil
}
