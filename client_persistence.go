package sber

import "context"

func (c *SberClient) saveRotatedSession(ctx context.Context) error {
	if c.core().sessionPath == "" {
		return nil
	}
	b, e := c.ExportSession()
	if e != nil {
		return e
	}
	if c.core().credentialsOnly {
		credentials, e := CredentialsFromBundle(b)
		if e != nil {
			return e
		}
		b, e = credentials.ToBundle(CredentialsBundleOptions{APIBase: b.APIBase, WebBase: b.WebBase, Browser: b.Browser, Deviceprint: b.Deviceprint, AntifraudDeviceprint: b.AntifraudDeviceprint})
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
		return ErrClosed
	}
	if ctx.Err() != nil {
		return transportFailure(ctx.Err())
	}
	c.core().bundle = b
	return nil
}
