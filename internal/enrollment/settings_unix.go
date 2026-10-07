//go:build linux || darwin

package enrollment

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// ReadPrivateFile reads a bounded snapshot from a private, single-link regular
// file. Missing state returns nil without creating directories or locks.
func ReadPrivateFile(path string, limit int64) ([]byte, error) {
	if limit < 1 || limit > 1024*1024 {
		return nil, ErrUnsafe
	}
	parent, name, err := privateParent(path, false)
	if err == unix.ENOENT {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnsafe
	}
	defer unix.Close(parent)
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err == unix.ENOENT {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUnsafe
	}
	file := os.NewFile(uintptr(fd), "private settings")
	defer file.Close()
	var state unix.Stat_t
	if unix.Fstat(fd, &state) != nil || !privateFile(state) || state.Size > limit {
		return nil, ErrUnsafe
	}
	raw, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(raw)) > limit || checkSettingsParent(path, parent) != nil {
		return nil, ErrUnsafe
	}
	return raw, nil
}

// ReplacePrivateFile reuses enrollment's descriptor-pinned private directory
// boundary, with a separate stable lock and atomic replacement for settings.
// The caller supplies the complete record; no existing credentials are merged.
func ReplacePrivateFile(ctx context.Context, path string, raw []byte) (err error) {
	if ctx == nil || len(raw) > 1024*1024 {
		return ErrPrepare
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, name, err := privateParent(path, true)
	if err != nil {
		return ErrUnsafe
	}
	defer unix.Close(parent)
	lockName := "." + name + ".settings.lock"
	lock, err := acquireLock(parent, lockName)
	if err != nil {
		return err
	}
	defer unix.Close(lock)
	if checkBoundary(path, parent, lockName, lock) != nil || checkSettingsDestination(parent, name) != nil {
		return ErrUnsafe
	}
	temporary, err := newCandidateDirectory(path, parent, name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, temporary.cleanup(parent)) }()
	file, err := temporary.root.OpenFile("profile.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrWrite
	}
	n, writeErr := file.Write(raw)
	if writeErr == nil && n == len(raw) {
		writeErr = file.Sync()
	} else {
		writeErr = ErrWrite
	}
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return ErrWrite
	}
	candidate, err := temporary.privateFile()
	if err != nil {
		return err
	}
	defer unix.Close(candidate)
	var opened, current unix.Stat_t
	if unix.Fstat(candidate, &opened) != nil || unix.Fstatat(temporary.fd, "profile.json", &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateFile(current) || !sameInode(opened, current) || checkBoundary(path, parent, lockName, lock) != nil || checkSettingsDestination(parent, name) != nil {
		return ErrUnsafe
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if unix.Renameat(temporary.fd, "profile.json", parent, name) != nil {
		return ErrPublish
	}
	var final unix.Stat_t
	if unix.Fstatat(parent, name, &final, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateFile(final) || !sameInode(opened, final) || unix.Fsync(parent) != nil || unix.Fsync(temporary.fd) != nil {
		return ErrPublish
	}
	return nil
}

func checkSettingsDestination(parent int, name string) error {
	var state unix.Stat_t
	err := unix.Fstatat(parent, name, &state, unix.AT_SYMLINK_NOFOLLOW)
	if err == unix.ENOENT || err == nil && privateFile(state) {
		return nil
	}
	return ErrUnsafe
}

func checkSettingsParent(path string, parent int) error {
	current, _, err := privateParent(path, false)
	if err != nil {
		return ErrUnsafe
	}
	defer unix.Close(current)
	var opened, reopened unix.Stat_t
	if unix.Fstat(parent, &opened) != nil || !privateDirectory(opened) || unix.Fstat(current, &reopened) != nil || !sameInode(opened, reopened) {
		return ErrUnsafe
	}
	return nil
}
