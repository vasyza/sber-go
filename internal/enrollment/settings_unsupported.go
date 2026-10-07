//go:build !linux && !darwin

package enrollment

import "context"

func ReadPrivateFile(string, int64) ([]byte, error)            { return nil, ErrUnsupported }
func ReplacePrivateFile(context.Context, string, []byte) error { return ErrUnsupported }
