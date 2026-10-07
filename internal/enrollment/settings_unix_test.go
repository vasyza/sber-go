//go:build linux || darwin

package enrollment

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func settingsTestPath(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "settings", "config.json")
}

func TestPrivateSettingsAtomicReplacementAndCancellation(t *testing.T) {
	path := settingsTestPath(t)
	if raw, err := ReadPrivateFile(path, 4096); err != nil || raw != nil {
		t.Fatal("missing settings must be read without creation")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("read created settings state")
	}
	if err := ReplacePrivateFile(context.Background(), path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ReplacePrivateFile(ctx, path, []byte("new")); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled write accepted")
	}
	if raw, err := ReadPrivateFile(path, 4096); err != nil || string(raw) != "old" {
		t.Fatal("canceled write changed settings")
	}
	if err := ReplacePrivateFile(context.Background(), path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if raw, err := ReadPrivateFile(path, 4096); err != nil || string(raw) != "new" {
		t.Fatal("replacement failed")
	}
	for _, p := range []string{path, filepath.Dir(path)} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		want := os.FileMode(0600)
		if st.IsDir() {
			want = 0700
		}
		if st.Mode().Perm() != want {
			t.Fatal("settings permissions are not private")
		}
	}
}

func TestPrivateSettingsRejectsUnsafeFilesAndConcurrentWriter(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "permissions", "parent", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			path := settingsTestPath(t)
			if err := ReplacePrivateFile(context.Background(), path, []byte("old")); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				other := path + ".other"
				if err := os.Rename(path, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, path+".link"); err != nil {
					t.Fatal(err)
				}
			case "permissions":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "parent":
				if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := unix.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := ReadPrivateFile(path, 4096); !errors.Is(err, ErrUnsafe) {
				t.Fatal("unsafe settings read")
			}
			if err := ReplacePrivateFile(context.Background(), path, []byte("new")); !errors.Is(err, ErrUnsafe) {
				t.Fatal("unsafe settings replaced")
			}
		})
	}
	path := settingsTestPath(t)
	if err := ReplacePrivateFile(context.Background(), path, []byte("old")); err != nil {
		t.Fatal(err)
	}
	parent, name, err := privateParent(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(parent)
	lock, err := acquireLock(parent, "."+name+".settings.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(lock)
	if err := ReplacePrivateFile(context.Background(), path, []byte("new")); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent settings writer was not locked")
	}
}
