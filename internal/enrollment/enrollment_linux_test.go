//go:build linux

package enrollment

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func syntheticProfile(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "private", "profile.json")
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
			target := filepath.Join(t.TempDir(), "foreign")
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
