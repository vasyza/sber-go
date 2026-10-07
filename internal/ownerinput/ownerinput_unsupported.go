//go:build !linux

package ownerinput

import (
	"context"
	"os"
)

// ReadSecret deliberately fails closed on unsupported operating systems.
func ReadSecret(context.Context, *os.File, *os.File, Prompt) (*Secret, error) {
	return nil, ErrUnsupported
}
