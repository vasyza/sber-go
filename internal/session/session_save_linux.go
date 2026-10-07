//go:build linux

package session

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	"golang.org/x/sys/unix"
)

func saveSessionFile(path string, raw []byte) error {
	dfd, err := unix.Open(filepath.Dir(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	parent := os.NewFile(uintptr(dfd), "private-session-directory")
	defer parent.Close()
	dirInfo, err := parent.Stat()
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode().Perm()&0077 != 0 {
		return &sdkErrs.InsecureSessionFile{}
	}
	owner, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		return &sdkErrs.InsecureSessionFile{}
	}
	name := filepath.Base(path)
	checkTarget := func() error {
		var target unix.Stat_t
		err := unix.Fstatat(dfd, name, &target, unix.AT_SYMLINK_NOFOLLOW)
		if err == unix.ENOENT {
			return nil
		}
		if err != nil || target.Mode&unix.S_IFMT != unix.S_IFREG {
			return &sdkErrs.InsecureSessionFile{}
		}
		return nil
	}
	if err := checkTarget(); err != nil {
		return err
	}
	anchored := "/proc/self/fd/" + strconv.Itoa(dfd)
	f, err := os.CreateTemp(anchored, ".sber-session-*.tmp")
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	tmp := filepath.Base(f.Name())
	defer unix.Unlinkat(dfd, tmp, 0)
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if _, err = f.Write(raw); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = f.Sync(); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	info, err := f.Stat()
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = validateSessionStat(info, true); err != nil {
		return err
	}
	validated, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return &sdkErrs.InsecureSessionFile{}
	}
	// Reject an observed source exchange. This check is NOT the publication
	// guarantee: the subsequent link uses the open descriptor, never this name.
	var source unix.Stat_t
	if err = unix.Fstatat(dfd, tmp, &source, unix.AT_SYMLINK_NOFOLLOW); err != nil || source.Mode&unix.S_IFMT != unix.S_IFREG || source.Dev != validated.Dev || source.Ino != validated.Ino {
		return &sdkErrs.InsecureSessionFile{}
	}
	stagePath, err := os.MkdirTemp(anchored, ".sber-publish-")
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	stageName := filepath.Base(stagePath)
	defer unix.Unlinkat(dfd, stageName, unix.AT_REMOVEDIR)
	stageFD, err := unix.Openat(dfd, stageName, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	stage := os.NewFile(uintptr(stageFD), "private-session-publication")
	defer stage.Close()
	defer unix.Unlinkat(stageFD, "payload", 0)
	stageInfo, err := stage.Stat()
	if err != nil || !stageInfo.IsDir() || stageInfo.Mode().Perm()&0077 != 0 {
		return &sdkErrs.InsecureSessionFile{}
	}
	stageOwner, ok := stageInfo.Sys().(*syscall.Stat_t)
	if !ok || stageOwner.Uid != uint32(os.Getuid()) {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = unix.Linkat(unix.AT_FDCWD, "/proc/self/fd/"+strconv.Itoa(int(f.Fd())), stageFD, "payload", unix.AT_SYMLINK_FOLLOW); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	var linked unix.Stat_t
	if err = unix.Fstatat(stageFD, "payload", &linked, unix.AT_SYMLINK_NOFOLLOW); err != nil || linked.Mode&unix.S_IFMT != unix.S_IFREG || linked.Dev != validated.Dev || linked.Ino != validated.Ino {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = unix.Unlinkat(dfd, tmp, 0); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = f.Close(); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = stage.Sync(); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = checkTarget(); err != nil {
		return err
	}
	if err = unix.Renameat(stageFD, "payload", dfd, name); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = unix.Unlinkat(dfd, stageName, unix.AT_REMOVEDIR); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if err = parent.Sync(); err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	return nil
}
