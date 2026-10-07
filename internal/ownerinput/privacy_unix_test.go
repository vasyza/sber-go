//go:build linux || darwin

package ownerinput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

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
