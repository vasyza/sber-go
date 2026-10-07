//go:build linux

package enrollment

import (
	"bytes"
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeProfileExistsMetadataOnly(t *testing.T) {
	for _, kind := range []string{"missing-parent", "missing", "regular", "mode", "symlink", "dangling-symlink", "hardlink", "fifo", "directory"} {
		t.Run(kind, func(t *testing.T) {
			profile := syntheticProfile(t)
			parent := filepath.Dir(profile)
			if kind != "missing-parent" {
				os.Mkdir(parent, 0700)
			}
			target := filepath.Join(t.TempDir(), "foreign")
			switch kind {
			case "regular":
				os.WriteFile(profile, []byte("synthetic-opaque-status-only"), 0600)
			case "mode":
				os.WriteFile(profile, nil, 0600)
				os.Chmod(profile, 0644)
			case "symlink":
				os.WriteFile(target, nil, 0600)
				os.Symlink(target, profile)
			case "dangling-symlink":
				os.Symlink(target, profile)
			case "hardlink":
				os.WriteFile(target, nil, 0600)
				os.Link(target, profile)
			case "fifo":
				unix.Mkfifo(profile, 0600)
			case "directory":
				os.Mkdir(profile, 0700)
			}
			exists, err := SafeProfileExists(profile)
			switch kind {
			case "regular":
				if !exists || err != nil {
					t.Error("safe regular profile missing")
				}
			case "missing", "missing-parent":
				if exists || err != nil {
					t.Error("absent state not reported absent")
				}
			default:
				if exists || !errors.Is(err, ErrUnsafe) {
					t.Error("unsafe profile accepted by metadata check")
				}
			}
			if kind == "missing-parent" {
				if _, e := os.Lstat(parent); !errors.Is(e, os.ErrNotExist) {
					t.Error("metadata query created owner state")
				}
			}
		})
	}
}

func TestLiteralUnsafePathsNeverReachCallback(t *testing.T) {
	base := t.TempDir()
	for _, profile := range []string{"", base + "/../profile.json", base + "//profile.json", base + "/./profile.json", base + "/", base + "/profile\x00.json"} {
		called := false
		err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) { called = true; return syntheticWriter, nil })
		if !errors.Is(err, ErrUnsafe) || called {
			t.Error("unsafe literal path normalized or accepted")
		}
	}
}

func TestPublicationPinsCandidateInode(t *testing.T) {
	directory := t.TempDir()
	parent, e := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal("synthetic parent open failed")
	}
	defer unix.Close(parent)
	candidate := filepath.Join(directory, "candidate.json")
	os.WriteFile(candidate, []byte("synthetic-pinned-original"), 0600)
	fd, e := unix.Open(candidate, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal("synthetic candidate open failed")
	}
	defer unix.Close(fd)
	os.Rename(candidate, candidate+".moved")
	os.WriteFile(candidate, []byte("synthetic-swapped-source"), 0600)
	if publishCandidate(parent, "profile.json", fd) != nil {
		t.Fatal("pinned publication failed")
	}
	data, e := os.ReadFile(filepath.Join(directory, "profile.json"))
	if e != nil || !bytes.Equal(data, []byte("synthetic-pinned-original")) {
		t.Fatal("publication followed swapped candidate name")
	}
}

func TestWriterAtomicRenameCompatibleWithPinnedPrivateDirectory(t *testing.T) {
	profile := syntheticProfile(t)
	err := Enroll(context.Background(), profile, func(context.Context) (CandidateWriter, error) {
		return func(ctx context.Context, path string) error {
			file, e := os.CreateTemp(filepath.Dir(path), ".synthetic-writer-*.tmp")
			if e != nil {
				return e
			}
			temp := file.Name()
			defer os.Remove(temp)
			if e = file.Chmod(0600); e != nil {
				file.Close()
				return e
			}
			if _, e = file.Write([]byte("synthetic-atomic-writer-opaque")); e != nil {
				file.Close()
				return e
			}
			if e = file.Sync(); e != nil {
				file.Close()
				return e
			}
			if e = file.Close(); e != nil {
				return e
			}
			return os.Rename(temp, path)
		}, nil
	})
	if err != nil {
		t.Fatal("atomic-renaming candidate writer incompatible")
	}
	data, e := os.ReadFile(profile)
	if e != nil || !bytes.Equal(data, []byte("synthetic-atomic-writer-opaque")) {
		t.Fatal("atomic candidate bytes changed")
	}
	verifyNoCandidate(t, profile)
}
