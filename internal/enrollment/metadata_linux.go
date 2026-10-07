//go:build linux

package enrollment

import "golang.org/x/sys/unix"

// SafeProfileExists performs no bank request and never opens/reads a profile.
// It reports unsafe parents, symlinks, nonregular files, foreign ownership,
// non-0600 permissions and multiple links as ErrUnsafe rather than readiness.
// A missing parent/profile is false,nil and creates no owner state. This is for
// future local CLI status, not an MCP password/profile-metadata schema.
func SafeProfileExists(profile string) (bool, error) {
	parent, name, e := privateParent(profile, false)
	if e == unix.ENOENT {
		return false, nil
	}
	if e != nil {
		return false, ErrUnsafe
	}
	defer unix.Close(parent)
	var state unix.Stat_t
	if e := unix.Fstatat(parent, name, &state, unix.AT_SYMLINK_NOFOLLOW); e != nil {
		if e == unix.ENOENT {
			return false, nil
		}
		return false, ErrUnsafe
	}
	if !privateFile(state) {
		return false, ErrUnsafe
	}
	return true, nil
}
