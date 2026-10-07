//go:build linux

package enrollment

import (
	"context"
	"errors"

	"golang.org/x/sys/unix"
)

// Enroll acquires a stable private nonblocking process lock before Prepare.
// The lock inode is intentionally never removed, on success or failure. A
// publication error can occur after link/fsync; callers must inspect metadata
// rather than assume that any error means the destination is absent.
func Enroll(ctx context.Context, profile string, prepare Prepare) (err error) {
	if ctx == nil || prepare == nil {
		return ErrPrepare
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	parent, name, e := privateParent(profile, true)
	if e != nil {
		return ErrUnsafe
	}
	defer unix.Close(parent)
	lockName := "." + name + ".enrollment.lock"
	lock, e := acquireLock(parent, lockName)
	if e != nil {
		return e
	}
	defer unix.Close(lock)
	if checkBoundary(profile, parent, lockName, lock) != nil {
		return ErrUnsafe
	}
	var state unix.Stat_t
	if e := unix.Fstatat(parent, name, &state, unix.AT_SYMLINK_NOFOLLOW); e == nil {
		return ErrExists
	} else if e != unix.ENOENT {
		return ErrUnsafe
	}
	writer, e := prepareCandidate(ctx, prepare)
	if e != nil {
		return e
	}
	if checkBoundary(profile, parent, lockName, lock) != nil {
		return ErrUnsafe
	}
	temporary, e := newCandidateDirectory(parent, name)
	if e != nil {
		return e
	}
	defer func() {
		if e := temporary.cleanup(parent); e != nil {
			err = errors.Join(err, e)
		}
	}()
	if e := writeCandidate(ctx, fdPath(temporary.fd)+"/profile.json", writer); e != nil {
		return e
	}
	candidate, e := temporary.privateFile()
	if e != nil {
		return e
	}
	defer unix.Close(candidate)
	if checkBoundary(profile, parent, lockName, lock) != nil {
		return ErrUnsafe
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	return publishCandidate(parent, name, candidate)
}
