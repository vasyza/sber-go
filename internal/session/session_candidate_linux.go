//go:build linux

package session

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

// WriteEnrollmentCandidate accepts the enrollment module's pinned directory
// capability, not a filesystem symlink. It duplicates and validates the open
// descriptor and creates only the fixed new candidate filename. Final atomic
// no-replace publication belongs to enrollment. Ordinary Save stays no-follow.
func WriteEnrollmentCandidate(path string, bundle SessionBundle) error {
	const prefix = "/proc/self/fd/"
	directory := filepath.Dir(path)
	if !strings.HasPrefix(directory, prefix) {
		return bundle.Save(path)
	}
	number, err := strconv.Atoi(strings.TrimPrefix(directory, prefix))
	if err != nil || number < 0 || directory != prefix+strconv.Itoa(number) || path != directory+"/profile.json" {
		return &sdkErrs.InsecureSessionFile{}
	}
	parent, err := unix.FcntlInt(uintptr(number), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	defer unix.Close(parent)
	var state unix.Stat_t
	uid := os.Getuid()
	if unix.Fstat(parent, &state) != nil || state.Mode&unix.S_IFMT != unix.S_IFDIR || state.Mode&07777 != 0700 || uid < 0 || uint64(state.Uid) != uint64(uid) {
		return &sdkErrs.InsecureSessionFile{}
	}
	raw, err := encodeSessionFile(bundle)
	if err != nil {
		return err
	}
	fd, err := unix.Openat(parent, "profile.json", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	file := os.NewFile(uintptr(fd), "private-enrollment-candidate")
	defer file.Close()
	if file.Chmod(0600) != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	if n, err := file.Write(raw); err != nil || n != len(raw) {
		return &sdkErrs.InsecureSessionFile{}
	}
	if file.Sync() != nil || file.Close() != nil || unix.Fsync(parent) != nil {
		return &sdkErrs.InsecureSessionFile{}
	}
	return nil
}
