//go:build linux || darwin

package ownerinput

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestPTYMasterNotOwnerInputFailsBeforeEchoChanges(t *testing.T) {
	master, slave := syntheticPTY(t)
	before := ttyState(t, slave)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s, e := ReadSecret(ctx, master, slave, Password)
	if s != nil || !errors.Is(e, ErrTerminal) {
		if s != nil {
			s.Clear()
		}
		t.Error("PTY master accepted as owner stdin")
	}
	if !reflect.DeepEqual(before, ttyState(t, slave)) {
		t.Error("rejected PTY master changed slave echo")
	}
	if len(transcript(t, master)) != 0 {
		t.Error("rejected PTY master wrote a prompt")
	}
}

func TestDeadlineRestoresLivePTY(t *testing.T) {
	master, slave := syntheticPTY(t)
	before := ttyState(t, slave)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		s, e := ReadSecret(ctx, slave, slave, Password)
		if s != nil {
			s.Clear()
		}
		done <- e
	}()
	waitHidden(t, slave)
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("deadline not propagated")
		}
	case <-time.After(time.Second):
		t.Fatal("deadline left input blocked")
	}
	if !reflect.DeepEqual(before, ttyState(t, slave)) {
		t.Fatal("deadline did not restore termios")
	}
	transcript(t, master)
}
