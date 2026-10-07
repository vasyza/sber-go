//go:build linux || darwin

package enrollment

import (
	"strings"

	"golang.org/x/sys/unix"
)

// privateParent walks literal components without following symlinks, pins the
// final directory descriptor, and never fixes permissions of existing state.
func privateParent(profile string, create bool) (fd int, name string, err error) {
	if profile == "" || strings.IndexByte(profile, 0) >= 0 {
		return -1, "", ErrUnsafe
	}
	parts := strings.Split(profile, "/")
	anchor := "."
	if parts[0] == "" {
		anchor = "/"
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return -1, "", ErrUnsafe
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return -1, "", ErrUnsafe
		}
	}
	name = parts[len(parts)-1]
	fd, e := unix.Open(anchor, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return -1, "", ErrUnsafe
	}
	for _, part := range parts[:len(parts)-1] {
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e == unix.ENOENT && create {
			if e = unix.Mkdirat(fd, part, 0700); e == nil || e == unix.EEXIST {
				next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		unix.Close(fd)
		if e != nil {
			return -1, "", e
		}
		fd = next
	}
	var state unix.Stat_t
	if unix.Fstat(fd, &state) != nil || !privateDirectory(state) {
		unix.Close(fd)
		return -1, "", ErrUnsafe
	}
	return fd, name, nil
}
func privateDirectory(s unix.Stat_t) bool {
	return s.Mode&unix.S_IFMT == unix.S_IFDIR && s.Mode&07777 == 0700 && s.Uid == uint32(unix.Getuid())
}
func privateFile(s unix.Stat_t) bool {
	return s.Mode&unix.S_IFMT == unix.S_IFREG && s.Mode&07777 == 0600 && s.Uid == uint32(unix.Getuid()) && s.Nlink == 1
}
func sameInode(a, b unix.Stat_t) bool { return a.Dev == b.Dev && a.Ino == b.Ino }

func checkLock(parent int, name string, fd int) error {
	var opened, current unix.Stat_t
	if unix.Fstat(fd, &opened) != nil || !privateFile(opened) || unix.Fstatat(parent, name, &current, unix.AT_SYMLINK_NOFOLLOW) != nil || !privateFile(current) || !sameInode(opened, current) {
		return ErrUnsafe
	}
	return nil
}
func acquireLock(parent int, name string) (int, error) {
	fd, e := unix.Openat(parent, name, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return -1, ErrUnsafe
	}
	ok := false
	defer func() {
		if !ok {
			unix.Close(fd)
		}
	}()
	if e = checkLock(parent, name, fd); e != nil {
		return -1, e
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		if e == unix.EWOULDBLOCK || e == unix.EAGAIN {
			return -1, ErrBusy
		}
		return -1, ErrUnsafe
	}
	if e = checkLock(parent, name, fd); e != nil {
		return -1, e
	}
	ok = true
	return fd, nil
}
