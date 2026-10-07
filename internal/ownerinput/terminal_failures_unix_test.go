//go:build linux || darwin

package ownerinput

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

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
