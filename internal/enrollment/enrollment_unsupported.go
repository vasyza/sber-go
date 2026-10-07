//go:build !linux

package enrollment

import "context"

// Enroll deliberately fails closed on unsupported operating systems.
func Enroll(context.Context, string, Prepare) error { return ErrUnsupported }

// SafeProfileExists never inspects owner state on unsupported systems.
func SafeProfileExists(string) (bool, error) { return false, ErrUnsupported }
