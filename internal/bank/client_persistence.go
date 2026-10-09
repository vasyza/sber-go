package bank

import (
	"context"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
	sdkSession "github.com/vasyza/sber-sdk/internal/session"
	sdkTransport "github.com/vasyza/sber-sdk/internal/transport"
)

func (c *SberClient) saveRotatedSession(ctx context.Context) error {
	if c.core().sessionPath == "" {
		return nil
	}
	b, e := c.ExportSession()
	if e != nil {
		return e
	}
	if c.core().credentialsOnly {
		credentials, e := sdkSession.CredentialsFromBundle(b)
		if e != nil {
			return e
		}
		b, e = credentials.ToBundle(sdkSession.CredentialsBundleOptions{APIBase: b.APIBase, WebBase: b.WebBase, Browser: b.Browser, Deviceprint: b.Deviceprint, AntifraudDeviceprint: b.AntifraudDeviceprint})
		if e != nil {
			return e
		}
	}
	if e = c.check(ctx); e != nil {
		return e
	}
	if e = b.Save(c.core().sessionPath); e != nil {
		return e
	}
	c.core().mu.Lock()
	defer c.core().mu.Unlock()
	if c.core().closed {
		return sdkErrs.ErrClosed
	}
	if ctx.Err() != nil {
		return sdkTransport.TransportFailure(ctx.Err())
	}
	c.core().bundle = b
	return nil
}
