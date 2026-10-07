//go:build linux

package enrollment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestConcurrentForeignPublicationPreservesExactBytes(t *testing.T) {
	profile := syntheticProfile(t)
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
			return func(ctx context.Context, path string) error {
				if e := syntheticWriter(ctx, path); e != nil {
					return e
				}
				close(ready)
				<-release
				return nil
			}, nil
		})
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("synthetic writer did not start")
	}
	canary := []byte("synthetic-concurrent-foreign-exact-bytes")
	file, e := os.OpenFile(profile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		close(release)
		t.Fatal("synthetic foreign creator failed")
	}
	_, writeErr := file.Write(canary)
	syncErr := file.Sync()
	closeErr := file.Close()
	close(release)
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatal("synthetic foreign write failed")
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrExists) {
			t.Fatal("concurrent foreign profile was not protected")
		}
	case <-time.After(time.Second):
		t.Fatal("enrollment did not finish")
	}
	data, e := os.ReadFile(profile)
	if e != nil || !bytes.Equal(data, canary) {
		t.Fatal("concurrent established bytes changed")
	}
	verifyNoCandidate(t, profile)
}

func TestLockPathSwapAfterOpenBeforeFlockRejected(t *testing.T) {
	directory := t.TempDir()
	parent, e := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal("synthetic parent open failed")
	}
	defer unix.Close(parent)
	path := filepath.Join(directory, ".profile.json.enrollment.lock")
	lock, e := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC, 0600)
	if e != nil {
		t.Fatal("synthetic lock open failed")
	}
	defer unix.Close(lock)
	if checkLock(parent, filepath.Base(path), lock) != nil {
		t.Fatal("synthetic private lock rejected")
	}
	if os.Rename(path, path+".original") != nil {
		t.Fatal("synthetic lock swap failed")
	}
	os.WriteFile(path, nil, 0600)
	if unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB) != nil {
		t.Fatal("synthetic old lock acquisition failed")
	}
	if !errors.Is(checkLock(parent, filepath.Base(path), lock), ErrUnsafe) {
		t.Fatal("replaced lock inode not rejected after flock")
	}
}

func TestForeignUIDMetadataRejectedWithoutPrivilege(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "synthetic")
	os.WriteFile(path, nil, 0600)
	var file, dir unix.Stat_t
	if unix.Stat(path, &file) != nil || unix.Stat(directory, &dir) != nil {
		t.Fatal("synthetic metadata unavailable")
	}
	if !privateFile(file) || !privateDirectory(dir) {
		t.Fatal("owner synthetic metadata rejected")
	}
	file.Uid ^= 1
	dir.Uid ^= 1
	if privateFile(file) || privateDirectory(dir) {
		t.Fatal("foreign UID metadata accepted")
	}
}

func TestCooperativeLiveCancellationReleasesLock(t *testing.T) {
	profile := syntheticProfile(t)
	ready := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- Enroll(ctx, profile, func(ctx context.Context) (CandidateWriter, error) { close(ready); <-ctx.Done(); return nil, ctx.Err() })
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("first enrollment did not start")
	}
	called := false
	e := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { called = true; return syntheticWriter, nil })
	if called || !errors.Is(e, ErrBusy) {
		cancel()
		t.Fatal("live canceled owner lost exclusive lock early")
	}
	cancel()
	select {
	case e = <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal("live cancellation not propagated")
		}
	case <-time.After(time.Second):
		t.Fatal("live callback cancellation not released")
	}
	if e = Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { return syntheticWriter, nil }); e != nil {
		t.Fatal("cancellation retained enrollment lock")
	}
}
