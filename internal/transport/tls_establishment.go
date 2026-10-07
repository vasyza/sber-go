package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"syscall"
	"time"
)

type tlsRequestContextKey struct{}

// Recovery is confined to TLS establishment. The HTTP transport receives only
// a fully verified connection, so none of these attempts sends a request body.
// Certificate failures, protocol failures and transmitted HTTP requests do not
// enter this recovery path. All attempts share the original request deadline.
func verifiedTLSDialer(configuration *tls.Config, timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: configuration}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		owner, _ := ctx.Value(tlsRequestContextKey{}).(context.Context)
		deadline := time.Now().Add(timeout)
		if owner != nil {
			if earlier, ok := owner.Deadline(); ok && earlier.Before(deadline) {
				deadline = earlier
			}
		}
		budget, cancel := context.WithDeadline(ctx, deadline)
		defer cancel()
		// net/http retains values but detaches dialing from request cancellation.
		// A failed or closed owner request must not establish another connection.
		if owner != nil {
			stop := context.AfterFunc(owner, cancel)
			defer stop()
		}
		for attempt := 0; ; attempt++ {
			if owner != nil && owner.Err() != nil {
				return nil, owner.Err()
			}
			if err := budget.Err(); err != nil {
				return nil, err
			}
			connection, err := dialer.DialContext(budget, network, address)
			if err == nil {
				return connection, nil
			}
			if owner != nil && owner.Err() != nil {
				return nil, owner.Err()
			}
			if attempt == 2 || !retryableTLSClosure(err) {
				return nil, err
			}
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-budget.Done():
				timer.Stop()
				if owner != nil && owner.Err() != nil {
					return nil, owner.Err()
				}
				return nil, budget.Err()
			case <-timer.C:
			}
		}
	}
}

func retryableTLSClosure(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var hostname x509.HostnameError
	var unknown x509.UnknownAuthorityError
	var invalid x509.CertificateInvalidError
	if errors.As(err, &hostname) || errors.As(err, &unknown) || errors.As(err, &invalid) {
		return false
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) || errors.Is(err, syscall.EPIPE)
}
