//go:build linux

package ownerinput

import (
	"strconv"

	"golang.org/x/sys/unix"
)

const readTermiosRequest = unix.TCGETS
const writeTermiosRequest = unix.TCSETS

func openOwnerTerminal(fd int) (int, error) {
	// Reopening a master creates a different terminal; owner input is a slave.
	if _, err := unix.IoctlGetInt(fd, unix.TIOCGPTN); err == nil {
		return -1, ErrTerminal
	}
	return unix.Open("/proc/self/fd/"+strconv.Itoa(fd), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
}

func flushOwnerInput(fd int) {
	_ = unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH) // Flush only queued input after a failed read.
}
