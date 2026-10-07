package cli

import (
	"context"

	sber "github.com/vasyza/sber-go"
)

// Native authentication loads only public configuration here, before any
// credential prompt. Login reuses that validated configuration. Synthetic
// authenticators may omit the optional configuration interface.
func prepareAuthentication(ctx context.Context, auth any) error {
	if loader, ok := auth.(interface {
		LoadConfig(context.Context) (sber.FrontendConfig, error)
	}); ok {
		_, err := loader.LoadConfig(ctx)
		return err
	}
	return nil
}

type loginPhaseError struct {
	phase string
	cause error
}

func (*loginPhaseError) Error() string   { return "authentication failed" }
func (e *loginPhaseError) Unwrap() error { return e.cause }
