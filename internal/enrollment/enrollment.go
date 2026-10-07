// Package enrollment provides the local owner-state publication boundary.
// It has no bank/authentication dependency and treats candidate bytes as opaque.
package enrollment

import (
	"context"
	"errors"
)

var (
	ErrExists      = errors.New("owner profile already exists")
	ErrBusy        = errors.New("another owner enrollment is active")
	ErrUnsafe      = errors.New("unsafe owner enrollment filesystem state")
	ErrPrepare     = errors.New("owner enrollment preparation failed")
	ErrWrite       = errors.New("owner enrollment candidate write failed")
	ErrPublish     = errors.New("owner enrollment publication failed")
	ErrCleanup     = errors.New("owner enrollment temporary cleanup failed")
	ErrUnsupported = errors.New("owner enrollment is supported only on Linux")
)

// CandidateWriter writes a private 0600 regular file to the supplied candidate
// path and returns only after all its writes finish. Use the existing SDK writer
// unchanged. Never log/inspect profile bytes or store raw password/PIN/OTP here.
type CandidateWriter func(context.Context, string) error

// Prepare performs future public bootstrap and owner interaction only after the
// private nonblocking enrollment lock and no-existing-profile checks succeed.
// Callbacks must cooperate with ctx and complete synchronously: an uncooperative
// callback retains the lock until it returns rather than allowing a live writer
// to overlap another enrollment. Panics in unrelated goroutines are not caught.
type Prepare func(context.Context) (CandidateWriter, error)
