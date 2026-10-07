//go:build linux

package session

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestEnrollmentCandidatePreservesExistingFileAndRejectsSymlinks(t *testing.T) {
	for _, kind := range []string{"regular", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			directory := testPrivateDir(t)
			parent, err := unix.Open(directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil {
				t.Fatal("synthetic directory unavailable")
			}
			defer unix.Close(parent)
			path := filepath.Join(directory, "profile.json")
			foreign := filepath.Join(testPrivateDir(t), "foreign")
			canary := []byte("synthetic-existing-exact-bytes")
			if err := os.WriteFile(foreign, canary, 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "regular" {
				if err := os.WriteFile(path, canary, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Symlink(foreign, path); err != nil {
					t.Fatal(err)
				}
			}
			bundle, _ := NewSessionBundle(SessionBundle{APIBase: AppOrigin, WebBase: AppOrigin})
			if err := WriteEnrollmentCandidate("/proc/self/fd/"+strconv.Itoa(parent)+"/profile.json", bundle); err == nil {
				t.Fatal("existing candidate accepted")
			}
			for _, target := range []string{path, foreign} {
				after, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(after, canary) {
					t.Fatal("existing bytes changed")
				}
			}
		})
	}
}
