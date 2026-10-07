package sber

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestFoundationSessionPublicationSourceExchange(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")
	prior := []byte("previous private profile")
	if err := os.WriteFile(target, prior, 0600); err != nil {
		t.Fatal(err)
	}
	outsider := filepath.Join(t.TempDir(), "unrelated.json")
	if err := os.WriteFile(outsider, []byte("unrelated sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	b := syntheticBundle()
	b.Cookies = nil
	for i := 0; i < 200; i++ {
		b.Cookies = append(b.Cookies, CookieRecord{Name: fmt.Sprintf("audit%d", i), Value: strings.Repeat("s", 4000), Domain: "online.sberbank.ru", Path: "/", Secure: true})
	}
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	if _, err := unix.InotifyAddWatch(fd, dir, unix.IN_CREATE); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- b.Save(target) }()
	deadline := time.Now().Add(5 * time.Second)
	exchanged := false
	for time.Now().Before(deadline) && !exchanged {
		ready := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if _, err := unix.Poll(ready, 10); err != nil && err != unix.EINTR {
			t.Fatal(err)
		}
		buf := make([]byte, 4096)
		_, _ = unix.Read(fd, buf)
		names, err := filepath.Glob(filepath.Join(dir, ".sber-session-*.tmp"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			info, err := os.Lstat(name)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}
			if err := os.Rename(name, name+".displaced"); err != nil {
				continue
			}
			if err := os.Symlink(outsider, name); err != nil {
				t.Fatal(err)
			}
			exchanged = true
			break
		}
	}
	var saveErr error
	select {
	case saveErr = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Save did not complete")
	}
	if !exchanged {
		t.Fatal("source exchange fixture missed open temporary inode")
	}
	var insecure *InsecureSessionFile
	if !errors.As(saveErr, &insecure) {
		t.Errorf("source exchange returned %v instead of rejecting replaced pathname", saveErr)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("Save published substituted symlink/wrong inode")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != string(prior) {
		t.Error("failed source exchange changed existing profile")
	}
	data, err = os.ReadFile(outsider)
	if err != nil || string(data) != "unrelated sentinel" {
		t.Error("Save touched unrelated symlink target")
	}
}

func TestFoundationSessionTrustedParentAndRenewal(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")
	b := syntheticBundle()
	if err := b.Save(target); err != nil {
		t.Fatal(err)
	}
	oldInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	b.Cookies[0].Value = "renewed synthetic value"
	if err := b.Save(target); err != nil {
		t.Fatalf("normal overwrite/renewal unsupported: %v", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(info, oldInfo) || info.Mode().Perm() != 0600 {
		t.Error("renewal not atomic/private")
	}
	renewed, err := LoadSessionBundle(target)
	if err != nil || renewed.Cookies[0].Value != b.Cookies[0].Value {
		t.Error("renewal payload did not roundtrip")
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var insecure *InsecureSessionFile
	if err := b.Save(target); !errors.As(err, &insecure) {
		t.Error("untrusted/public parent accepted")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "parent-link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(filepath.Join(link, "state.json")); !errors.As(err, &insecure) {
		t.Error("symlink parent accepted as trusted boundary")
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0].Name() != "state.json" {
		t.Error("normal Save left publication staging artifacts")
	}
}
