package cli

import (
	"os"
	"testing"
)

// Profile fixtures must be private independently of the developer's umask.
func testPrivateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}
