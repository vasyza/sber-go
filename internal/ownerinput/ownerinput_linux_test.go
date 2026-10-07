//go:build linux

package ownerinput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func syntheticPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal("synthetic PTY unavailable")
	}
	t.Cleanup(func() { master.Close(); slave.Close() })
	return master, slave
}

func ttyState(t *testing.T, f *os.File) unix.Termios {
	t.Helper()
	state, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatal("cannot inspect synthetic terminal metadata")
	}
	return *state
}

func waitHidden(t *testing.T, f *os.File) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := ttyState(t, f)
		if state.Lflag&(unix.ECHO|unix.ECHONL) == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("synthetic terminal never disabled echo")
}

func transcript(t *testing.T, f *os.File) []byte {
	t.Helper()
	var output []byte
	for {
		polls := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
		n, err := unix.Poll(polls, 30)
		if err != nil {
			t.Fatal("synthetic transcript poll failed")
		}
		if n == 0 || polls[0].Revents&unix.POLLIN == 0 {
			return output
		}
		buf := make([]byte, 512)
		n, err = unix.Read(int(f.Fd()), buf)
		if err != nil {
			t.Fatal("synthetic transcript read failed")
		}
		output = append(output, buf[:n]...)
	}
}

func TestRequiresBothTerminalDescriptorsBeforeConsuming(t *testing.T) {
	for _, broken := range []string{"stdin", "stderr", "nil-stdin", "nil-stderr", "closed"} {
		t.Run(broken, func(t *testing.T) {
			master, slave := syntheticPTY(t)
			state := ttyState(t, slave)
			state.Lflag &^= unix.ECHO | unix.ECHONL
			if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, &state); err != nil {
				t.Fatal("synthetic setup failed")
			}
			read, write, e := os.Pipe()
			if e != nil {
				t.Fatal("synthetic pipe unavailable")
			}
			defer read.Close()
			defer write.Close()
			in, out := slave, slave
			var check *os.File
			canary := []byte("synthetic-never-consume-canary\n")
			switch broken {
			case "stdin":
				in = read
				check = read
				write.Write(canary)
			case "stderr":
				out = write
				check = slave
				master.Write(canary)
			case "nil-stdin":
				in = nil
			case "nil-stderr":
				out = nil
				check = slave
				master.Write(canary)
			case "closed":
				in = read
				read.Close()
			}
			func() {
				defer func() {
					if recover() != nil {
						t.Error("invalid descriptor caused panic")
					}
				}()
				s, err := ReadSecret(context.Background(), in, out, Password)
				if !errors.Is(err, ErrTerminal) || s != nil {
					if s != nil {
						s.Clear()
					}
					t.Error("nonterminal input was accepted")
				}
			}()
			if check != nil {
				polls := []unix.PollFd{{Fd: int32(check.Fd()), Events: unix.POLLIN}}
				ready, pe := unix.Poll(polls, 50)
				if pe != nil || ready == 0 {
					t.Fatal("rejected terminal consumed queued synthetic input")
				}
				buf := make([]byte, len(canary))
				n, err := unix.Read(int(check.Fd()), buf)
				if err != nil || !bytes.Equal(buf[:n], canary) {
					t.Fatal("rejected terminal consumed queued synthetic input")
				}
			}
		})
	}
}

func TestCancelledReadRestoresTermiosAndDiscardsPartialInput(t *testing.T) {
	master, slave := syntheticPTY(t)
	original := ttyState(t, slave)
	original.Lflag |= unix.ECHONL
	if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, &original); err != nil {
		t.Fatal("synthetic setup failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type answer struct {
		s *Secret
		e error
	}
	done := make(chan answer, 1)
	go func() { s, e := ReadSecret(ctx, slave, slave, Password); done <- answer{s, e} }()
	deadline := time.Now().Add(2 * time.Second)
	for ttyState(t, slave).Lflag&unix.ECHO != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if ttyState(t, slave).Lflag&unix.ECHONL != 0 {
		t.Error("hidden terminal still echoes newline")
	}
	master.Write([]byte("synthetic-canceled-partial-input"))
	cancel()
	var got answer
	select {
	case got = <-done:
	case <-time.After(250 * time.Millisecond):
		t.Error("context cancellation did not stop input")
		master.Write([]byte{'\n'})
		select {
		case got = <-done:
		case <-time.After(time.Second):
			t.Fatal("failed to release synthetic reader")
		}
	}
	if !errors.Is(got.e, context.Canceled) || got.s != nil {
		if got.s != nil {
			got.s.Clear()
		}
		t.Error("canceled read returned a secret")
	}
	if !reflect.DeepEqual(original, ttyState(t, slave)) {
		t.Error("canceled input did not restore terminal")
	}
	if bytes.Contains(transcript(t, master), []byte("synthetic-canceled-partial-input")) {
		t.Error("partial synthetic input was echoed")
	}
	next := make(chan answer, 1)
	go func() { s, e := ReadSecret(context.Background(), slave, slave, Password); next <- answer{s, e} }()
	waitHidden(t, slave)
	master.Write([]byte{'\n'})
	select {
	case got = <-next:
	case <-time.After(time.Second):
		t.Fatal("retry reader blocked")
	}
	if got.e != nil || got.s == nil || len(got.s.Bytes()) != 0 {
		t.Error("canceled input remained queued for retry")
	}
	got.s.Clear()
}

func TestReadSecretRealPTYNoEchoRestoresAndClears(t *testing.T) {
	master, slave := syntheticPTY(t)
	before := ttyState(t, slave)
	type answer struct {
		secret *Secret
		err    error
	}
	result := make(chan answer, 1)
	go func() { s, e := ReadSecret(context.Background(), slave, slave, Password); result <- answer{s, e} }()
	waitHidden(t, slave)
	canary := []byte("synthetic-ownerinput-canary-only\n")
	if _, err := master.Write(canary); err != nil {
		t.Fatal("cannot write synthetic input")
	}
	var got answer
	select {
	case got = <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("hidden read blocked")
	}
	if got.err != nil || got.secret == nil {
		t.Fatal("hidden read failed")
	}
	if !bytes.Equal(got.secret.Bytes(), bytes.TrimSuffix(canary, []byte{'\n'})) {
		t.Fatal("synthetic secret mismatch")
	}
	if !reflect.DeepEqual(before, ttyState(t, slave)) {
		t.Fatal("terminal state not restored")
	}
	if bytes.Contains(transcript(t, master), bytes.TrimSuffix(canary, []byte{'\n'})) {
		t.Fatal("synthetic input was echoed")
	}
	alias := got.secret.Bytes()
	got.secret.Clear()
	if !bytes.Equal(alias, make([]byte, len(alias))) || len(got.secret.Bytes()) != 0 {
		t.Fatal("secret buffer not cleared")
	}
}
