//go:build !linux && !darwin

package enrollment

import "context"

// Enroll deliberately fails closed on unsupported operating systems.
func Enroll(context.Context, string, Prepare) error { return ErrUnsupported }
