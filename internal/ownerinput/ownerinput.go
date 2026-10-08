// Package ownerinput implements fail-closed local terminal input. It never
// obtains credentials from arguments, environment variables, or a network.
package ownerinput

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
)

var (
	ErrTerminal    = errors.New("owner input requires a safe terminal")
	ErrRead        = errors.New("owner terminal input failed")
	ErrRestore     = errors.New("owner terminal state could not be restored")
	ErrBusy        = errors.New("another owner terminal prompt is active")
	ErrPrompt      = errors.New("invalid owner terminal prompt")
	ErrUnsupported = errors.New("owner terminal input requires Linux or macOS")
)

type Prompt uint8

const (
	Login Prompt = iota + 1
	Password
	OTP
	NewPIN
	ConfirmPIN
	PIN
	ConfirmAction
	Phone
	CardNumber
)

func promptLabel(prompt Prompt) (string, bool) {
	switch prompt {
	case Login:
		return "Login (hidden): ", true
	case Password:
		return "Password (hidden): ", true
	case OTP:
		return "One-time code (hidden): ", true
	case NewPIN:
		return "New online-banking PIN (not card PIN, hidden): ", true
	case ConfirmPIN:
		return "Confirm new PIN (hidden): ", true
	case PIN:
		return "Online-banking PIN (not card PIN, hidden): ", true
	case ConfirmAction:
		return "Type CONFIRM to send this operation (hidden): ", true
	case Phone:
		return "Bank phone number (hidden): ", true
	case CardNumber:
		return "Card number (not card PIN or CVV, hidden): ", true
	}
	return "", false
}

// ReadOwnerSecret is the production entry point: no alternate input channels.
func ReadOwnerSecret(ctx context.Context, prompt Prompt) (*Secret, error) {
	return ReadSecret(ctx, os.Stdin, os.Stderr, prompt)
}

// Secret owns a mutable buffer. Bytes borrows it; callers must not retain or
// concurrently use that view after Clear. Clearing is best effort: Go, the
// terminal driver, and downstream conversions may retain other memory copies.
type Secret struct{ value []byte }

// Format intentionally redacts every formatting verb, including value copies.
func (s Secret) Format(state fmt.State, verb rune) { _, _ = state.Write([]byte("[REDACTED]")) }
func (s Secret) String() string                    { return "[REDACTED]" }
func (s Secret) GoString() string                  { return "[REDACTED]" }

func (s *Secret) Bytes() []byte {
	if s == nil {
		return nil
	}
	return s.value
}
func (s *Secret) Clear() {
	if s == nil {
		return
	}
	clear(s.value[:cap(s.value)])
	runtime.KeepAlive(s.value)
	s.value = nil
}
