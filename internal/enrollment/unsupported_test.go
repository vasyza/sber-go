//go:build !linux && !darwin

package enrollment

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestUnsupportedEnrollmentFailsClosed(t *testing.T) {
	called := false
	e := Enroll(context.Background(), "synthetic-unused-path", func(context.Context) (CandidateWriter, error) { called = true; return nil, nil })
	if called || !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported OS did not reject enrollment")
	}
	exists, e := SafeProfileExists("synthetic-unused-path")
	if runtime.GOOS != "darwin" && (exists || !errors.Is(e, ErrUnsupported)) {
		t.Fatal("unsupported metadata query accepted")
	}
}
