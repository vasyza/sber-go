//go:build !linux

package session

// macOS enrollment supplies a fresh private staging directory. The ordinary
// safe writer fills it; enrollment owns final atomic no-replace publication.
func WriteEnrollmentCandidate(path string, bundle SessionBundle) error {
	return bundle.Save(path)
}
