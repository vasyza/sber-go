//go:build linux || darwin

package ownerinput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
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
			// Darwin also reports FWASWRITTEN after the prompt is printed on
			// this terminal. Compare operating flags, not kernel write history.
			const operatingFlags = unix.O_ACCMODE | unix.O_NONBLOCK | unix.O_APPEND | unix.O_ASYNC | unix.O_SYNC | unix.O_DSYNC
			if e != nil || (after^flags)&operatingFlags != 0 {
				t.Fatalf("hidden input changed original stdin descriptor flags: before=%#x after=%#x", flags, after)
			}
			if bytes.Contains(transcript(t, master), []byte("synthetic-post-read-canary")) {
				t.Fatal("post-read failure echoed input")
			}
		})
	}
}

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
	state, err := unix.IoctlGetTermios(int(f.Fd()), readTermiosRequest)
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
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			t.Fatalf("synthetic transcript poll failed: %v", err)
		}
		if n == 0 || polls[0].Revents&unix.POLLIN == 0 {
			return output
		}
		buf := make([]byte, 512)
		n, err = unix.Read(int(f.Fd()), buf)
		if err == unix.EINTR {
			continue
		}
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
			if err := unix.IoctlSetTermios(int(slave.Fd()), writeTermiosRequest, &state); err != nil {
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
	if err := unix.IoctlSetTermios(int(slave.Fd()), writeTermiosRequest, &original); err != nil {
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

func TestSecretFormattingNeverDisclosesBuffer(t *testing.T) {
	s := &Secret{value: []byte("synthetic-format-canary-only")}
	defer s.Clear()
	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%p"} {
		if strings.Contains(fmt.Sprintf(format, s), "synthetic") || strings.Contains(fmt.Sprintf(format, *s), "synthetic") {
			t.Error("secret formatter leaked synthetic buffer")
		}
	}
}

func TestFixedPromptStagesRealPTY(t *testing.T) {
	for _, prompt := range []Prompt{Login, Password, OTP, NewPIN, ConfirmPIN, PIN, ConfirmAction} {
		t.Run(fmt.Sprint(int(prompt)), func(t *testing.T) {
			master, slave := syntheticPTY(t)
			done := make(chan bool, 1)
			go func() {
				s, e := ReadSecret(context.Background(), slave, slave, prompt)
				if s != nil {
					s.Clear()
				}
				done <- e == nil
			}()
			waitHidden(t, slave)
			master.Write([]byte("synthetic-stage-canary\n"))
			select {
			case ok := <-done:
				if !ok {
					t.Fatal("secret stage failed")
				}
			case <-time.After(time.Second):
				t.Fatal("stage reader blocked")
			}
			output := transcript(t, master)
			if bytes.Contains(output, []byte("synthetic-stage-canary")) || len(output) == 0 {
				t.Fatal("unsafe or missing fixed prompt")
			}
		})
	}
}

func TestConcurrentPromptRejectedWithoutReading(t *testing.T) {
	master, slave := syntheticPTY(t)
	ctx, cancel := context.WithCancel(context.Background())
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
	second := make(chan bool, 1)
	go func() {
		s, e := ReadSecret(context.Background(), slave, slave, OTP)
		if s != nil {
			s.Clear()
		}
		second <- s == nil && errors.Is(e, ErrBusy)
	}()
	select {
	case ok := <-second:
		if !ok {
			t.Error("concurrent prompt accepted")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("concurrent prompt blocked")
		master.Write([]byte{'\n'})
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first prompt not released")
	}
}

func TestPreCancelledContextNeverConsumes(t *testing.T) {
	master, slave := syntheticPTY(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, e := ReadSecret(ctx, slave, slave, Password)
	if s != nil || !errors.Is(e, context.Canceled) {
		t.Fatal("pre-cancelled prompt accepted")
	}
	if len(transcript(t, master)) != 0 {
		t.Fatal("pre-cancelled prompt wrote output")
	}
}

func TestInvalidPromptNeverConsumes(t *testing.T) {
	master, slave := syntheticPTY(t)
	s, e := ReadSecret(context.Background(), slave, slave, Prompt(255))
	if s != nil || !errors.Is(e, ErrPrompt) {
		t.Fatal("invalid prompt accepted")
	}
	if len(transcript(t, master)) != 0 {
		t.Fatal("invalid prompt wrote output")
	}
}

func TestReadOwnerSecretUsesActualStandardDescriptors(t *testing.T) {
	master, slave := syntheticPTY(t)
	oldIn, oldErr := os.Stdin, os.Stderr
	os.Stdin, os.Stderr = slave, slave
	defer func() { os.Stdin, os.Stderr = oldIn, oldErr }()
	done := make(chan bool, 1)
	go func() {
		s, e := ReadOwnerSecret(context.Background(), Login)
		if s != nil {
			s.Clear()
		}
		done <- e == nil
	}()
	waitHidden(t, slave)
	master.Write([]byte("synthetic-owner-wrapper-only\n"))
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("owner wrapper failed")
		}
	case <-time.After(time.Second):
		t.Fatal("owner wrapper blocked")
	}
	if bytes.Contains(transcript(t, master), []byte("synthetic-owner-wrapper-only")) {
		t.Fatal("owner wrapper echoed")
	}
}

func TestRestoreFailureDiscardsSuccessfullyReadBuffer(t *testing.T) {
	master, slave := syntheticPTY(t)
	original := ttyState(t, slave)
	defer unix.IoctlSetTermios(int(slave.Fd()), writeTermiosRequest, &original)
	var borrowed []byte
	calls := terminalCalls{
		readPassword: func(fd int) ([]byte, error) {
			value, err := term.ReadPassword(fd)
			borrowed = value
			unix.Close(fd) // Genuine EBADF during checked outer restoration.
			return value, err
		},
		restore: term.Restore,
	}
	done := make(chan bool, 1)
	go func() {
		s, err := readSecret(context.Background(), slave, slave, Password, calls)
		if s != nil {
			s.Clear()
		}
		done <- s == nil && errors.Is(err, ErrRestore)
	}()
	waitHidden(t, slave)
	master.Write([]byte("synthetic-restore-failure-canary\n"))
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("failed restoration returned successful secret")
		}
	case <-time.After(time.Second):
		t.Fatal("restore failure reader blocked")
	}
	if len(borrowed) == 0 || !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
		t.Fatal("restore failure did not clear read buffer")
	}
}

func TestPromptWriteFailureRestoresBeforeRead(t *testing.T) {
	master, slave := syntheticPTY(t)
	before := ttyState(t, slave)
	output, e := os.OpenFile(slave.Name(), os.O_RDONLY, 0)
	if e != nil {
		t.Fatal("synthetic terminal output open failed")
	}
	defer output.Close()
	s, err := ReadSecret(context.Background(), slave, output, Password)
	if s != nil || !errors.Is(err, ErrRead) {
		t.Fatal("unwritable terminal accepted")
	}
	if !reflect.DeepEqual(before, ttyState(t, slave)) {
		t.Fatal("write failure did not restore")
	}
	if len(transcript(t, master)) != 0 {
		t.Fatal("write failure emitted data")
	}
}
