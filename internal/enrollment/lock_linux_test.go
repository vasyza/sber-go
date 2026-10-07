//go:build linux

package enrollment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestConcurrentEnrollmentBusyBeforeCallback(t *testing.T) {
	profile := syntheticProfile(t)
	ready, release := make(chan struct{}), make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { close(ready); <-release; return syntheticWriter, nil })
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("first enrollment did not start")
	}
	var called atomic.Bool
	err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { called.Store(true); return syntheticWriter, nil })
	close(release)
	if !errors.Is(err, ErrBusy) || called.Load() {
		t.Error("second owner reached callback instead of nonblocking busy")
	}
	select {
	case err = <-result:
		if err != nil {
			t.Error("first enrollment failed")
		}
	case <-time.After(time.Second):
		t.Fatal("first owner blocked")
	}
}

func TestInterprocessEnrollmentLock(t *testing.T) {
	profile := syntheticProfile(t)
	err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestEnrollmentLockChild$")
		child.Env = []string{"SYNTHETIC_ENROLLMENT_CHILD_PROFILE=" + profile}
		if _, e := child.CombinedOutput(); e != nil {
			t.Error("second-process nonblocking lock test failed")
		}
		return syntheticWriter, nil
	})
	if err != nil {
		t.Fatal("parent enrollment failed")
	}
}
func TestEnrollmentLockChild(t *testing.T) {
	profile := os.Getenv("SYNTHETIC_ENROLLMENT_CHILD_PROFILE")
	if profile == "" {
		t.Skip("isolated child process only")
	}
	called := false
	err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
		called = true
		return nil, errors.New("synthetic-forbidden-callback")
	})
	if !errors.Is(err, ErrBusy) || called {
		t.Fatal("second process reached preparation")
	}
}

func TestUnsafeStateRejectedBeforeCallback(t *testing.T) {
	kinds := []string{"lock-mode", "lock-hardlink", "lock-symlink", "lock-dangling", "lock-fifo", "lock-directory", "parent-mode", "parent-symlink", "ancestor-symlink", "parent-owner", "lock-owner"}
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			state := filepath.Join(base, "private")
			os.Mkdir(state, 0700)
			profile := filepath.Join(state, "profile.json")
			lock := filepath.Join(state, ".profile.json.enrollment.lock")
			target := filepath.Join(base, "unrelated")
			canary := []byte("synthetic-unrelated-state-preserve")
			switch kind {
			case "lock-mode":
				os.WriteFile(lock, canary, 0644)
				os.Chmod(lock, 0644)
			case "lock-hardlink":
				os.WriteFile(target, canary, 0600)
				os.Link(target, lock)
			case "lock-symlink":
				os.WriteFile(target, canary, 0600)
				os.Symlink(target, lock)
			case "lock-dangling":
				os.Symlink(target, lock)
			case "lock-fifo":
				unix.Mkfifo(lock, 0600)
			case "lock-directory":
				os.Mkdir(lock, 0700)
			case "parent-mode":
				os.Chmod(state, 0755)
			case "parent-symlink":
				os.Remove(state)
				os.Mkdir(target, 0700)
				os.Symlink(target, state)
			case "ancestor-symlink":
				os.Symlink(base, filepath.Join(base, "alias"))
				profile = filepath.Join(base, "alias", "private", "profile.json")
			case "parent-owner":
				if os.Getuid() != 0 {
					t.Skip("foreign ownership setup needs root")
				}
				os.Chown(state, 60001, 60001)
			case "lock-owner":
				if os.Getuid() != 0 {
					t.Skip("foreign ownership setup needs root")
				}
				os.WriteFile(lock, canary, 0600)
				os.Chown(lock, 60001, 60001)
			}
			called := false
			err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { called = true; return syntheticWriter, nil })
			if !errors.Is(err, ErrUnsafe) || called {
				t.Error("unsafe state reached preparation")
			}
			if _, e := os.Lstat(profile); e == nil {
				t.Error("unsafe state published a profile")
			}
			if kind == "lock-mode" {
				data, e := os.ReadFile(lock)
				if e != nil || !bytes.Equal(data, canary) {
					t.Error("unsafe lock content changed")
				}
			}
			if kind == "lock-hardlink" || kind == "lock-symlink" {
				data, e := os.ReadFile(target)
				if e != nil || !bytes.Equal(data, canary) {
					t.Error("unrelated synthetic bytes changed")
				}
			}
		})
	}
}
