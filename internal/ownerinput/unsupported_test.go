//go:build !linux

package ownerinput

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedTerminalFailsClosed(t *testing.T) {
	s, e := ReadSecret(context.Background(), nil, nil, Password)
	if s != nil || !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported OS did not reject terminal input")
	}
	s, e = ReadOwnerSecret(context.Background(), Password)
	if s != nil || !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported owner input accepted")
	}
}
