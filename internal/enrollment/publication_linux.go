//go:build linux

package enrollment

import (
	"strconv"

	"golang.org/x/sys/unix"
)

func fdPath(fd int) string { return "/proc/self/fd/" + strconv.Itoa(fd) }

func candidateRootPath(_ string, fd int, _ string) string { return fdPath(fd) }

func (c candidateDirectory) path(_ string) string { return fdPath(c.fd) + "/profile.json" }

func (c candidateDirectory) publish(parent int, name string, candidate int) error {
	return publishCandidate(parent, name, candidate)
}

// publishCandidate links the pinned source inode, not its changeable filename.
// Following this internally generated procfs descriptor is deliberate; neither
// untrusted filesystem symlinks nor the destination are followed or replaced.
func publishCandidate(parent int, name string, candidate int) error {
	var state unix.Stat_t
	if unix.Fstat(candidate, &state) != nil || !privateFile(state) {
		return ErrUnsafe
	}
	if unix.Fsync(candidate) != nil {
		return ErrPublish
	}
	if e := unix.Linkat(unix.AT_FDCWD, fdPath(candidate), parent, name, unix.AT_SYMLINK_FOLLOW); e != nil {
		if e == unix.EEXIST {
			return ErrExists
		}
		return ErrPublish
	}
	var final unix.Stat_t
	if unix.Fstatat(parent, name, &final, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameInode(state, final) || final.Mode&07777 != 0600 || final.Uid != state.Uid {
		return ErrPublish
	}
	if unix.Fsync(parent) != nil {
		return ErrPublish
	}
	return nil
}
