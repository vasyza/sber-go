//go:build linux

package ownerinput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

// Deny actual TCGETS/TCSETS ioctls in a helper thread, not a fake terminal.
func denyEchoChanges(t *testing.T, request uint32) {
	t.Helper()
	runtime.LockOSThread()
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_IOCTL), Jf: 3},
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 24},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: request, Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EACCES)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	if unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != nil || unix.Prctl(unix.PR_SET_SECCOMP, unix.SECCOMP_MODE_FILTER, uintptr(unsafe.Pointer(&program)), 0, 0) != nil {
		t.Fatal("real seccomp echo-control test unavailable")
	}
	runtime.KeepAlive(filter)
}

func TestEchoAcquireIOCTLFailureBeforeCanary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	for _, request := range []string{"get", "set"} {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEchoFailureChild$")
		command.Env = []string{"SYNTHETIC_ECHO_FAILURE_CHILD=" + request}
		if _, err := command.CombinedOutput(); err != nil {
			t.Fatal("real ioctl rejection helper failed")
		}
	}
}
func TestEchoFailureChild(t *testing.T) {
	mode := os.Getenv("SYNTHETIC_ECHO_FAILURE_CHILD")
	if mode == "" {
		t.Skip("executed only in isolated helper process")
	}
	master, slave := syntheticPTY(t)
	settings := ttyState(t, slave)
	settings.Lflag &^= unix.ECHO | unix.ECHONL
	if unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, &settings) != nil {
		t.Fatal("synthetic setup failed")
	}
	canary := []byte("synthetic-ioctl-never-read-canary\n")
	if _, err := master.Write(canary); err != nil {
		t.Fatal("synthetic setup write failed")
	}
	request := uint32(unix.TCSETS)
	if mode == "get" {
		request = uint32(unix.TCGETS)
	}
	denyEchoChanges(t, request)
	s, err := ReadSecret(context.Background(), slave, slave, Password)
	if s != nil || !errors.Is(err, ErrTerminal) {
		if s != nil {
			s.Clear()
		}
		t.Fatal("echo acquisition failure did not fail closed")
	}
	polls := []unix.PollFd{{Fd: int32(slave.Fd()), Events: unix.POLLIN}}
	n, e := unix.Poll(polls, 100)
	if e != nil || n == 0 {
		t.Fatal("echo acquisition failure consumed synthetic canary")
	}
	buf := make([]byte, len(canary))
	n, e = unix.Read(int(slave.Fd()), buf)
	if e != nil || !bytes.Equal(buf[:n], canary) {
		t.Fatal("echo acquisition failure changed queued input")
	}
	if bytes.Contains(transcript(t, master), bytes.TrimSuffix(canary, []byte{'\n'})) {
		t.Fatal("ioctl rejection emitted synthetic canary")
	}
}

func TestRestoreFailureDiscardsSuccessfullyReadBuffer(t *testing.T) {
	master, slave := syntheticPTY(t)
	original := ttyState(t, slave)
	defer unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, &original)
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
