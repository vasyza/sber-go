//go:build linux

package ownerinput

import (
	"context"
	"os"
	"strconv"
	"sync"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var promptMutex sync.Mutex

type terminalCalls struct {
	readPassword func(int) ([]byte, error)
	restore      func(int, *term.State) error
}

// ReadSecret requires both descriptors to be terminals. In production pass the
// actual owner-operated stdin/stderr, never an agent's pipe or MCP input. Bind
// ctx to interrupt/termination signals at the future CLI boundary. Concurrent
// prompts fail with ErrBusy. Neither the terminal driver nor Go memory is a
// protection boundary against other processes running as this owner.
func ReadSecret(ctx context.Context, input, output *os.File, prompt Prompt) (*Secret, error) {
	return readSecret(ctx, input, output, prompt, terminalCalls{term.ReadPassword, term.Restore})
}

func readSecret(ctx context.Context, input, output *os.File, prompt Prompt, calls terminalCalls) (secret *Secret, err error) {
	if ctx == nil || input == nil || output == nil || !term.IsTerminal(int(input.Fd())) || !term.IsTerminal(int(output.Fd())) {
		return nil, ErrTerminal
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	// Reopen this exact terminal with an independent nonblocking description;
	// dup would share O_NONBLOCK with the owner's stdin. No input is read here.
	label, ok := promptLabel(prompt)
	if !ok {
		return nil, ErrPrompt
	}
	if !promptMutex.TryLock() {
		return nil, ErrBusy
	}
	defer promptMutex.Unlock()
	originalFD := int(input.Fd())
	// Reopening a PTY master opens a different terminal. An owner's stdin is
	// a slave/console; reject master endpoints before any echo changes/read.
	if _, e := unix.IoctlGetInt(originalFD, unix.TIOCGPTN); e == nil {
		return nil, ErrTerminal
	}
	fd, e := unix.Open("/proc/self/fd/"+strconv.Itoa(originalFD), unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrTerminal
	}
	defer unix.Close(fd)
	var original, current unix.Stat_t
	if unix.Fstat(originalFD, &original) != nil || unix.Fstat(fd, &current) != nil || original.Dev != current.Dev || original.Ino != current.Ino || original.Rdev != current.Rdev {
		return nil, ErrTerminal
	}
	state, e := term.GetState(fd)
	if e != nil {
		return nil, ErrTerminal
	}
	acquired := false
	defer func() {
		if acquired && err != nil {
			unix.IoctlSetInt(fd, unix.TCFLSH, unix.TCIFLUSH)
		}
		if acquired && calls.restore(fd, state) != nil {
			secret.Clear()
			secret = nil
			err = ErrRestore
		}
	}()
	// Only the cancellation wait needs this canonical no-echo guard. The line
	// reader remains x/term.ReadPassword; there is no fallback or bespoke getpass.
	settings, e := unix.IoctlGetTermios(fd, unix.TCGETS)
	if e != nil {
		return nil, ErrTerminal
	}
	settings.Lflag &^= unix.ECHO | unix.ECHONL
	settings.Lflag |= unix.ICANON | unix.ISIG
	settings.Iflag |= unix.ICRNL
	if unix.IoctlSetTermios(fd, unix.TCSETS, settings) != nil {
		return nil, ErrTerminal
	}
	acquired = true
	if _, e := output.WriteString(label); e != nil {
		return nil, ErrRead
	}
	for {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		polls := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, e := unix.Poll(polls, 20)
		if e == unix.EINTR {
			continue
		}
		if e != nil {
			return nil, ErrRead
		}
		if n == 0 {
			continue
		}
		if polls[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return nil, ErrRead
		}
		if polls[0].Revents&unix.POLLIN == 0 {
			continue
		}
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		value, e := calls.readPassword(fd)
		if e != nil {
			clear(value[:cap(value)])
			return nil, ErrRead
		}
		if e := ctx.Err(); e != nil {
			clear(value[:cap(value)])
			return nil, e
		}
		return &Secret{value: value}, nil
	}
}
