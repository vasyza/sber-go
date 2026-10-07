//go:build !linux

package session

// Non-Linux enrollment is unavailable. Ordinary private paths remain useful
// for synthetic application tests and use the existing safe writer.
func WriteEnrollmentCandidate(path string, bundle SessionBundle) error {
	return bundle.Save(path)
}
