package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// Profile fixtures must be private independently of the developer's umask.
func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// macOS /var is a symlink. Enrollment intentionally requires literal,
	// symlink-free paths; canonicalize only these generated test fixtures.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
