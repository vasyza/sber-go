//go:build darwin

package enrollment

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// SafeProfileExists checks only private metadata and never reads profile data.
func SafeProfileExists(profile string) (bool, error) {
	if profile == "" || strings.IndexByte(profile, 0) >= 0 || filepath.Clean(profile) != profile || filepath.Base(profile) == "." {
		return false, ErrUnsafe
	}
	fd, e := unix.Open(filepath.Dir(profile), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if e == unix.ENOENT {
		return false, nil
	}
	if e != nil {
		return false, ErrUnsafe
	}
	defer unix.Close(fd)
	var parent, file unix.Stat_t
	if unix.Fstat(fd, &parent) != nil || parent.Mode&07777 != 0700 || parent.Uid != uint32(os.Getuid()) {
		return false, ErrUnsafe
	}
	e = unix.Fstatat(fd, filepath.Base(profile), &file, unix.AT_SYMLINK_NOFOLLOW)
	if e == unix.ENOENT {
		return false, nil
	}
	if e != nil || file.Mode&unix.S_IFMT != unix.S_IFREG || file.Mode&07777 != 0600 || file.Uid != uint32(os.Getuid()) || file.Nlink != 1 {
		return false, ErrUnsafe
	}
	return true, nil
}
