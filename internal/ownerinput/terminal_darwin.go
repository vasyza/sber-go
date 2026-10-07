//go:build darwin

package ownerinput

import (
	"bytes"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

const readTermiosRequest = unix.TIOCGETA
const writeTermiosRequest = unix.TIOCSETA

func openOwnerTerminal(fd int) (int, error) {
	// /dev/fd duplicates the original open description on macOS. Ask the
	// kernel for this device's path and reopen it, leaving stdin flags alone.
	var path [unix.PathMax]byte
	var pinned runtime.Pinner
	pinned.Pin(&path[0])
	_, err := unix.FcntlInt(uintptr(fd), unix.F_GETPATH, int(uintptr(unsafe.Pointer(&path[0]))))
	pinned.Unpin()
	if err != nil {
		return -1, ErrTerminal
	}
	end := bytes.IndexByte(path[:], 0)
	if end <= 0 {
		return -1, ErrTerminal
	}
	name := string(path[:end])
	// Darwin PTY masters (/dev/ptmx and /dev/pty*) are never owner input.
	if !strings.HasPrefix(name, "/dev/tty") && name != "/dev/console" {
		return -1, ErrTerminal
	}
	return unix.Open(name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
}

func flushOwnerInput(fd int) {
	unix.IoctlSetPointerInt(fd, unix.TIOCFLUSH, 1) // FREAD: flush only queued input.
}
