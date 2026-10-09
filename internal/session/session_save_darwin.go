//go:build darwin

package session

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

// A private staging directory pins the publication namespace on macOS, where
// procfs descriptor links are unavailable. Same-owner processes must respect
// this private namespace, just as they must respect the owner's process memory.
func saveSessionFile(path string, raw []byte) error {
	parent, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	defer unix.Close(parent)
	var directory unix.Stat_t
	uid := os.Getuid()
	if unix.Fstat(parent, &directory) != nil || directory.Mode&unix.S_IFMT != unix.S_IFDIR || directory.Mode&0777 != 0700 || uid < 0 || uint64(directory.Uid) != uint64(uid) {
		return &sdkErrs.InsecureSessionFile{}
	}
	name := filepath.Base(path)
	check := func() error {
		var target unix.Stat_t
		e := unix.Fstatat(parent, name, &target, unix.AT_SYMLINK_NOFOLLOW)
		if e == unix.ENOENT {
			return nil
		}
		if e != nil || target.Mode&unix.S_IFMT != unix.S_IFREG {
			return &sdkErrs.InsecureSessionFile{}
		}
		return nil
	}
	if err = check(); err != nil {
		return err
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	stageName := ".sber-publish-" + hex.EncodeToString(nonce[:])
	if unix.Mkdirat(parent, stageName, 0700) != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	defer func() { _ = unix.Unlinkat(parent, stageName, unix.AT_REMOVEDIR) }()
	stage, err := unix.Openat(parent, stageName, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	defer unix.Close(stage)
	defer func() { _ = unix.Unlinkat(stage, "payload", 0) }()
	fd, err := unix.Openat(stage, "payload", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	file := os.NewFile(uintptr(fd), "private-session")
	defer file.Close()
	if _, err = file.Write(raw); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = file.Sync(); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	var opened, current unix.Stat_t
	if unix.Fstat(fd, &opened) != nil || unix.Fstatat(stage, "payload", &current, unix.AT_SYMLINK_NOFOLLOW) != nil || opened.Dev != current.Dev || opened.Ino != current.Ino || current.Mode&unix.S_IFMT != unix.S_IFREG || current.Mode&0777 != 0600 {
		return &sdkErrs.InsecureSessionFile{}
	}
	if file.Close() != nil || unix.Fsync(stage) != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = check(); err != nil {
		return err
	}
	if unix.Renameat(stage, "payload", parent, name) != nil || unix.Fsync(parent) != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	return nil
}
