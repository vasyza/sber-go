//go:build linux || darwin

package enrollment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

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
