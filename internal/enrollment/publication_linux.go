//go:build linux

package enrollment

import (
	"crypto/rand"
	"encoding/hex"
	"golang.org/x/sys/unix"
	"os"
	"strconv"
)

func fdPath(fd int) string { return "/proc/self/fd/" + strconv.Itoa(fd) }

func checkBoundary(profile string, parent int, lockName string, lock int) error {
	var original, reopened unix.Stat_t
	if unix.Fstat(parent, &original) != nil || !privateDirectory(original) {
		return ErrUnsafe
	}
	current, _, e := privateParent(profile, false)
	if e != nil {
		return ErrUnsafe
	}
	defer unix.Close(current)
	if unix.Fstat(current, &reopened) != nil || !sameInode(original, reopened) {
		return ErrUnsafe
	}
	return checkLock(parent, lockName, lock)
}

type candidateDirectory struct {
	name string
	fd   int
	root *os.Root
}

func newCandidateDirectory(parent int, profileName string) (candidateDirectory, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var nonce [16]byte
		if _, e := rand.Read(nonce[:]); e != nil {
			return candidateDirectory{}, ErrWrite
		}
		name := "." + profileName + ".enroll-" + hex.EncodeToString(nonce[:])
		e := unix.Mkdirat(parent, name, 0700)
		if e == unix.EEXIST {
			continue
		}
		if e != nil {
			return candidateDirectory{}, ErrWrite
		}
		fd, e := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			unix.Unlinkat(parent, name, unix.AT_REMOVEDIR)
			return candidateDirectory{}, ErrUnsafe
		}
		root, e := os.OpenRoot(fdPath(fd))
		if e != nil {
			unix.Close(fd)
			unix.Unlinkat(parent, name, unix.AT_REMOVEDIR)
			return candidateDirectory{}, ErrUnsafe
		}
		return candidateDirectory{name, fd, root}, nil
	}
	return candidateDirectory{}, ErrWrite
}
func (c candidateDirectory) cleanup(parent int) error {
	defer unix.Close(c.fd)
	defer c.root.Close()
	dir, e := c.root.Open(".")
	if e != nil {
		return ErrCleanup
	}
	entries, e := dir.ReadDir(-1)
	dir.Close()
	if e != nil {
		return ErrCleanup
	}
	for _, entry := range entries {
		if c.root.RemoveAll(entry.Name()) != nil {
			return ErrCleanup
		}
	}
	var opened, current unix.Stat_t
	if unix.Fstat(c.fd, &opened) != nil || unix.Fstatat(parent, c.name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !sameInode(opened, current) {
		return ErrCleanup
	}
	if unix.Unlinkat(parent, c.name, unix.AT_REMOVEDIR) != nil {
		return ErrCleanup
	}
	return nil
}
func (c candidateDirectory) privateFile() (int, error) {
	var directory unix.Stat_t
	if unix.Fstat(c.fd, &directory) != nil || !privateDirectory(directory) {
		return -1, ErrUnsafe
	}
	fd, e := unix.Openat(c.fd, "profile.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return -1, ErrUnsafe
	}
	var state unix.Stat_t
	if unix.Fstat(fd, &state) != nil || !privateFile(state) {
		unix.Close(fd)
		return -1, ErrUnsafe
	}
	return fd, nil
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
