//go:build !linux && !darwin

package enrollment

// SafeProfileExists fails closed where private metadata cannot be verified.
func SafeProfileExists(string) (bool, error) { return false, ErrUnsupported }
