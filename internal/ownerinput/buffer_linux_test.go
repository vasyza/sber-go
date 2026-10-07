//go:build linux

package ownerinput

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestSecretExplicitStringViewsAndNilSafety(t *testing.T) {
	s := &Secret{value: []byte("synthetic-explicit-format-only")}
	if s.String() != "[REDACTED]" || s.GoString() != "[REDACTED]" {
		t.Fatal("explicit string view disclosed buffer")
	}
	s.Clear()
	s.Clear()
	var absent *Secret
	absent.Clear()
	if absent.Bytes() != nil {
		t.Fatal("nil secret buffer not safe")
	}
}

func TestRealPTYPostReadFailureClearsAndRestores(t *testing.T) {
	for _, failure := range []string{"read-error", "canceled-after-read"} {
		t.Run(failure, func(t *testing.T) {
			master, slave := syntheticPTY(t)
			before := ttyState(t, slave)
			flags, e := unix.FcntlInt(slave.Fd(), unix.F_GETFL, 0)
			if e != nil {
				t.Fatal("synthetic flags unavailable")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var borrowed []byte
			calls := terminalCalls{readPassword: func(fd int) ([]byte, error) {
				value, e := term.ReadPassword(fd)
				borrowed = value
				if e != nil {
					return value, e
				}
				if failure == "read-error" {
					return value, errors.New("synthetic-do-not-report-error")
				}
				cancel()
				return value, nil
			}, restore: term.Restore}
			type result struct {
				s *Secret
				e error
			}
			done := make(chan result, 1)
			go func() { s, e := readSecret(ctx, slave, slave, Password, calls); done <- result{s, e} }()
			waitHidden(t, slave)
			master.Write([]byte("synthetic-post-read-canary\n"))
			var got result
			select {
			case got = <-done:
			case <-time.After(time.Second):
				t.Fatal("post-read failure blocked")
			}
			if got.s != nil || got.e == nil || strings.Contains(got.e.Error(), "synthetic") {
				if got.s != nil {
					got.s.Clear()
				}
				t.Fatal("post-read failure returned data or leaked error")
			}
			if len(borrowed) == 0 || !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
				t.Fatal("post-read failure left read buffer uncleared")
			}
			if !reflect.DeepEqual(before, ttyState(t, slave)) {
				t.Fatal("post-read failure did not restore termios")
			}
			after, e := unix.FcntlInt(slave.Fd(), unix.F_GETFL, 0)
			if e != nil || after != flags {
				t.Fatal("hidden input changed original stdin descriptor flags")
			}
			if bytes.Contains(transcript(t, master), []byte("synthetic-post-read-canary")) {
				t.Fatal("post-read failure echoed input")
			}
		})
	}
}
