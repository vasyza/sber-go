//go:build darwin

package enrollment

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

func candidateRootPath(profile string, _ int, name string) string {
	return filepath.Join(filepath.Dir(profile), name)
}

func (c candidateDirectory) path(profile string) string {
	return filepath.Join(candidateRootPath(profile, c.fd, c.name), "profile.json")
}

// macOS has no procfs inode links. Use its native no-replace rename inside the
// pinned private staging namespace, with the same owner-process trust boundary
// as private session saves. Never fall back to an overwriting rename.
func (c candidateDirectory) publish(parent int, name string, candidate int) error {
	var opened, current, directory unix.Stat_t
	if unix.Fstat(c.fd, &directory) != nil || !privateDirectory(directory) || unix.Fstat(candidate, &opened) != nil || !privateFile(opened) || unix.Fstatat(c.fd, "profile.json", &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateFile(current) || !sameInode(opened, current) {
		return ErrUnsafe
	}
	if unix.Fsync(candidate) != nil || unix.Fsync(c.fd) != nil {
		return ErrPublish
	}
	if e := unix.RenameatxNp(c.fd, "profile.json", parent, name, unix.RENAME_EXCL); e != nil {
		if e == unix.EEXIST {
			return ErrExists
		}
		return ErrPublish
	}
	var final unix.Stat_t
	if unix.Fstatat(parent, name, &final, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateFile(final) || !sameInode(opened, final) {
		return ErrPublish
	}
	if unix.Fsync(parent) != nil || unix.Fsync(c.fd) != nil {
		return ErrPublish
	}
	return nil
}
