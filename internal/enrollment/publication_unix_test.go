//go:build linux || darwin

package enrollment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

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
