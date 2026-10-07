//go:build linux || darwin

package enrollment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	directory := testPrivateDir(t)
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
	directory := testPrivateDir(t)
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal("synthetic private directory unavailable")
	}
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

func syntheticProfile(t *testing.T) string {
	t.Helper()
	return filepath.Join(testPrivateDir(t), "private", "profile.json")
}

func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("cannot canonicalize synthetic directory")
	}
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("cannot make synthetic directory private")
	}
	return dir
}
func syntheticWriter(ctx context.Context, path string) error {
	return os.WriteFile(path, []byte("synthetic-enrollment-opaque-profile-only"), 0600)
}
func verifyNoCandidate(t *testing.T, profile string) {
	t.Helper()
	entries, e := os.ReadDir(filepath.Dir(profile))
	if e != nil {
		t.Fatal("synthetic state directory missing")
	}
	for _, entry := range entries {
		if entry.Name() != ".profile.json.enrollment.lock" && entry.Name() != "profile.json" {
			t.Fatal("enrollment left temporary state")
		}
	}
}

func TestNormalEnrollmentPrivateAtomicPublication(t *testing.T) {
	profile := syntheticProfile(t)
	called := false
	err := Enroll(context.Background(), profile, func(ctx context.Context) (CandidateWriter, error) {
		called = true
		lock := filepath.Join(filepath.Dir(profile), ".profile.json.enrollment.lock")
		if _, e := os.Stat(lock); e != nil {
			t.Error("callback started before private lock existed")
		}
		return syntheticWriter, nil
	})
	if err != nil || !called {
		t.Fatal("normal synthetic enrollment failed")
	}
	stat, e := os.Stat(profile)
	if e != nil || stat.Mode().Perm() != 0600 {
		t.Fatal("published profile is not private")
	}
	parent, e := os.Stat(filepath.Dir(profile))
	if e != nil || parent.Mode().Perm() != 0700 {
		t.Fatal("profile parent is not private")
	}
	data, e := os.ReadFile(profile)
	if e != nil || !bytes.Equal(data, []byte("synthetic-enrollment-opaque-profile-only")) {
		t.Fatal("opaque candidate bytes changed")
	}
	verifyNoCandidate(t, profile)
}

func TestExistingProfileRefusedBeforeCallback(t *testing.T) {
	for _, kind := range []string{"file", "symlink", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			profile := syntheticProfile(t)
			os.MkdirAll(filepath.Dir(profile), 0700)
			target := filepath.Join(testPrivateDir(t), "foreign")
			if kind != "dangling-symlink" {
				os.WriteFile(target, []byte("synthetic-established-profile-preserve"), 0600)
			}
			if kind == "file" {
				os.WriteFile(profile, []byte("synthetic-established-profile-preserve"), 0600)
			} else {
				os.Symlink(target, profile)
			}
			called := false
			err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { called = true; return syntheticWriter, nil })
			if !errors.Is(err, ErrExists) || called {
				t.Fatal("existing profile reached callback or was replaced")
			}
			if kind == "file" {
				data, e := os.ReadFile(profile)
				if e != nil || !bytes.Equal(data, []byte("synthetic-established-profile-preserve")) {
					t.Fatal("foreign bytes changed")
				}
			} else {
				link, e := os.Readlink(profile)
				if e != nil || link != target {
					t.Fatal("foreign symlink changed")
				}
			}
			verifyNoCandidate(t, profile)
		})
	}
}

func TestFailuresReleaseStableLockCleanAndRedact(t *testing.T) {
	for _, failure := range []string{"prepare-error", "prepare-panic", "prepare-cancel", "nil-writer", "write-error", "write-panic", "write-cancel"} {
		t.Run(failure, func(t *testing.T) {
			profile := syntheticProfile(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			canary := "synthetic-error-panic-never-report-canary"
			target := filepath.Join(testPrivateDir(t), "foreign")
			os.WriteFile(target, []byte("synthetic-foreign-cleanup-preserve"), 0600)
			var first unix.Stat_t
			var escaped bool
			var err error
			func() {
				defer func() {
					if recover() != nil {
						escaped = true
					}
				}()
				err = Enroll(ctx, profile, func(context.Context) (CandidateWriter, error) {
					unix.Stat(filepath.Join(filepath.Dir(profile), ".profile.json.enrollment.lock"), &first)
					switch failure {
					case "prepare-error":
						return nil, errors.New(canary)
					case "prepare-panic":
						panic(canary)
					case "prepare-cancel":
						cancel()
					case "nil-writer":
						return nil, nil
					}
					return func(ctx context.Context, path string) error {
						os.WriteFile(path, []byte("synthetic-partial-candidate"), 0600)
						os.Mkdir(filepath.Join(filepath.Dir(path), "nested"), 0700)
						os.WriteFile(filepath.Join(filepath.Dir(path), "nested", "partial"), []byte("synthetic-partial"), 0600)
						os.Symlink(target, filepath.Join(filepath.Dir(path), "foreign-link"))
						switch failure {
						case "write-error":
							return errors.New(canary)
						case "write-panic":
							panic(canary)
						case "write-cancel":
							cancel()
						}
						return nil
					}, nil
				})
			}()
			if escaped {
				t.Error("callback panic escaped enrollment boundary")
			}
			if err == nil || strings.Contains(fmt.Sprint(err), canary) {
				t.Error("failed enrollment succeeded or leaked callback data")
			}
			if strings.HasSuffix(failure, "cancel") && !errors.Is(err, context.Canceled) {
				t.Error("cancellation not propagated safely")
			}
			if _, e := os.Lstat(profile); e == nil {
				t.Error("failure published profile")
			}
			verifyNoCandidate(t, profile)
			data, e := os.ReadFile(target)
			if e != nil || string(data) != "synthetic-foreign-cleanup-preserve" {
				t.Error("cleanup followed foreign symlink")
			}
			var second unix.Stat_t
			lock := filepath.Join(filepath.Dir(profile), ".profile.json.enrollment.lock")
			if unix.Stat(lock, &second) != nil || !sameInode(first, second) || second.Mode&07777 != 0600 || second.Size != 0 {
				t.Error("stable private lock changed")
			}
			if e := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { return syntheticWriter, nil }); e != nil {
				t.Error("failure did not release process lock")
			}
		})
	}
}

func TestCancelledEnrollmentNeverCreatesState(t *testing.T) {
	profile := syntheticProfile(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := Enroll(ctx, profile, func(context.Context) (CandidateWriter, error) { called = true; return syntheticWriter, nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal("pre-canceled enrollment reached callback")
	}
	if _, e := os.Stat(filepath.Dir(profile)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("pre-canceled enrollment created state")
	}
}

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
			base := testPrivateDir(t)
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

func TestForeignProfileCreatedLatePreserved(t *testing.T) {
	for _, window := range []string{"prepare", "write"} {
		for _, kind := range []string{"file", "symlink", "dangling-symlink"} {
			t.Run(window+"-"+kind, func(t *testing.T) {
				profile := syntheticProfile(t)
				target := filepath.Join(testPrivateDir(t), "foreign")
				canary := []byte("synthetic-established-final-bytes")
				if kind != "dangling-symlink" {
					os.WriteFile(target, canary, 0600)
				}
				foreign := func() {
					if kind == "file" {
						os.WriteFile(profile, canary, 0600)
					} else {
						os.Symlink(target, profile)
					}
				}
				err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
					if window == "prepare" {
						foreign()
					}
					return func(ctx context.Context, path string) error {
						if window == "write" {
							foreign()
						}
						return syntheticWriter(ctx, path)
					}, nil
				})
				if !errors.Is(err, ErrExists) {
					t.Error("late foreign profile not rejected")
				}
				if kind == "file" {
					data, e := os.ReadFile(profile)
					if e != nil || !bytes.Equal(data, canary) {
						t.Error("foreign final bytes changed")
					}
				} else {
					link, e := os.Readlink(profile)
					if e != nil || link != target {
						t.Error("foreign symlink changed")
					}
				}
				if kind != "dangling-symlink" {
					data, e := os.ReadFile(target)
					if e != nil || !bytes.Equal(data, canary) {
						t.Error("foreign destination changed")
					}
				} else if _, e := os.Lstat(target); !errors.Is(e, os.ErrNotExist) {
					t.Error("dangling target created")
				}
				verifyNoCandidate(t, profile)
			})
		}
	}
}

func TestSwappedLockInodeRejected(t *testing.T) {
	for _, window := range []string{"prepare", "write"} {
		t.Run(window, func(t *testing.T) {
			profile := syntheticProfile(t)
			lock := filepath.Join(filepath.Dir(profile), ".profile.json.enrollment.lock")
			swap := func() {
				if e := os.Rename(lock, lock+".old"); e != nil {
					t.Fatal("synthetic lock rename failed")
				}
				os.WriteFile(lock, nil, 0600)
			}
			err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
				if window == "prepare" {
					swap()
				}
				return func(ctx context.Context, path string) error {
					if window == "write" {
						swap()
					}
					return syntheticWriter(ctx, path)
				}, nil
			})
			if !errors.Is(err, ErrUnsafe) {
				t.Error("swapped lock was accepted")
			}
			if _, e := os.Lstat(profile); e == nil {
				t.Error("swapped lock allowed publication")
			}
			os.Remove(lock + ".old")
			verifyNoCandidate(t, profile)
		})
	}
}

func TestParentDirectorySwapFailsClosed(t *testing.T) {
	for _, window := range []string{"prepare", "write"} {
		t.Run(window, func(t *testing.T) {
			profile := syntheticProfile(t)
			parent := filepath.Dir(profile)
			old := parent + "-moved"
			swap := func() {
				if e := os.Rename(parent, old); e != nil {
					t.Fatal("synthetic parent rename failed")
				}
				os.Mkdir(parent, 0700)
			}
			err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
				if window == "prepare" {
					swap()
				}
				return func(ctx context.Context, path string) error {
					if window == "write" {
						swap()
					}
					return syntheticWriter(ctx, path)
				}, nil
			})
			if !errors.Is(err, ErrUnsafe) {
				t.Error("parent swap not rejected")
			}
			if _, e := os.Lstat(profile); e == nil {
				t.Error("replacement parent received profile")
			}
			if _, e := os.Lstat(filepath.Join(old, "profile.json")); e == nil {
				t.Error("detached parent received profile")
			}
			verifyNoCandidate(t, filepath.Join(old, "profile.json"))
		})
	}
}

func TestUnsafeCandidateCannotPublish(t *testing.T) {
	for _, kind := range []string{"mode", "hardlink", "symlink", "dangling-symlink", "fifo", "directory", "missing", "temporary-mode"} {
		t.Run(kind, func(t *testing.T) {
			profile := syntheticProfile(t)
			target := filepath.Join(testPrivateDir(t), "foreign")
			canary := []byte("synthetic-candidate-foreign-preserve")
			os.WriteFile(target, canary, 0600)
			err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
				return func(ctx context.Context, path string) error {
					switch kind {
					case "mode":
						syntheticWriter(ctx, path)
						os.Chmod(path, 0644)
					case "hardlink":
						return os.Link(target, path)
					case "symlink":
						return os.Symlink(target, path)
					case "dangling-symlink":
						return os.Symlink(target+"-missing", path)
					case "fifo":
						return unix.Mkfifo(path, 0600)
					case "directory":
						return os.Mkdir(path, 0700)
					case "missing":
						return nil
					case "temporary-mode":
						syntheticWriter(ctx, path)
						return os.Chmod(filepath.Dir(path), 0755)
					}
					return nil
				}, nil
			})
			if !errors.Is(err, ErrUnsafe) {
				t.Error("unsafe candidate did not fail closed")
			}
			if _, e := os.Lstat(profile); e == nil {
				t.Error("unsafe candidate was published")
			}
			data, e := os.ReadFile(target)
			if e != nil || !bytes.Equal(data, canary) {
				t.Error("foreign candidate target changed")
			}
			verifyNoCandidate(t, profile)
		})
	}
}
