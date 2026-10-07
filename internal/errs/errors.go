// Package sber implements the native SberBank Online SDK. Secret-bearing values
// expose only redacted formatting; persistence is an explicit private-file API.
package errs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// SberError is implemented by all SDK error types.
type SberError interface {
	error
	SDKError()
}

var ErrClosed = errors.New("sber: transport closed")

// Messages and remote metadata remain accessible to the explicit caller, but
// are NEVER included in Error, fmt output or ordinary JSON/log serialization.
type MissingSession struct{ Message string }
type InsecureSessionFile struct{ Message string }
type AuthenticationExpired struct{ Message string }
type APIError struct {
	Message    string
	StatusCode int
}
type APIRejected struct{ Message, Code, Title, Text, UUID, System string }
type MutationUncertain struct{ Message string }

// TransportError retains only a safe code and context error identity. The raw
// net/http error (which can contain credentials and URLs) is not unwrap-able.
type TransportError struct {
	Code  string
	cause error
}

// NewTransportError retains only cancellation and deadline identities. Raw
// transport errors may contain sensitive URLs and are never stored or unwrapped.
func NewTransportError(code string, cause error) *TransportError {
	var safe error
	if errors.Is(cause, context.Canceled) {
		safe = context.Canceled
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		safe = errors.Join(safe, context.DeadlineExceeded)
	}
	return &TransportError{Code: code, cause: safe}
}

// ContextCause returns only the safe context classifications, never a raw cause.
func (e TransportError) ContextCause() error { return e.cause }

func (e TransportError) Is(target error) bool {
	return (target == context.Canceled || target == context.DeadlineExceeded) && errors.Is(e.cause, target)
}

type PinAuthError struct {
	Message           string
	Code              string
	StatusCode        int
	RemainingAttempts *int
	ResetCookies      bool
}
type PinCaptchaRequired struct {
	PinAuthError
	ImageURL, AudioURL *string
}
type PinOTPRequired struct {
	PinAuthError
	Lifetime *int
}

// Aliases preserve the Python spelling without creating distinct Go contracts.
type ApiError = APIError
type ApiRejected = APIRejected
type PinOtpRequired = PinOTPRequired

func safeErrorJSON(kind string) ([]byte, error) {
	return json.Marshal(struct {
		Kind string `json:"error"`
	}{kind})
}
func FormatError(f fmt.State, text string) { _, _ = f.Write([]byte(text)) }

func (e MissingSession) Error() string                { return "sber: missing or invalid session" }
func (e MissingSession) String() string               { return e.Error() }
func (e MissingSession) GoString() string             { return e.Error() }
func (e MissingSession) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e MissingSession) MarshalJSON() ([]byte, error) { return safeErrorJSON("missing_session") }
func (e MissingSession) SDKError()                    {}

func (e InsecureSessionFile) Error() string              { return "sber: insecure session file" }
func (e InsecureSessionFile) String() string             { return e.Error() }
func (e InsecureSessionFile) GoString() string           { return e.Error() }
func (e InsecureSessionFile) Format(f fmt.State, v rune) { FormatError(f, e.Error()) }
func (e InsecureSessionFile) MarshalJSON() ([]byte, error) {
	return safeErrorJSON("insecure_session_file")
}
func (e InsecureSessionFile) SDKError() {}

func (e AuthenticationExpired) Error() string              { return "sber: authentication expired" }
func (e AuthenticationExpired) String() string             { return e.Error() }
func (e AuthenticationExpired) GoString() string           { return e.Error() }
func (e AuthenticationExpired) Format(f fmt.State, v rune) { FormatError(f, e.Error()) }
func (e AuthenticationExpired) MarshalJSON() ([]byte, error) {
	return safeErrorJSON("authentication_expired")
}
func (e AuthenticationExpired) SDKError() {}

func (e APIError) Error() string                { return "sber: unexpected API response" }
func (e APIError) String() string               { return e.Error() }
func (e APIError) GoString() string             { return e.Error() }
func (e APIError) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e APIError) MarshalJSON() ([]byte, error) { return safeErrorJSON("api_error") }
func (e APIError) SDKError()                    {}

func (e APIRejected) Error() string                { return "sber: API rejected request" }
func (e APIRejected) String() string               { return e.Error() }
func (e APIRejected) GoString() string             { return e.Error() }
func (e APIRejected) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e APIRejected) MarshalJSON() ([]byte, error) { return safeErrorJSON("api_rejected") }
func (e APIRejected) SDKError()                    {}
func (e APIRejected) Unwrap() error                { return &APIError{} }

func (e TransportError) Error() string                { return "sber: HTTP request failed" }
func (e TransportError) String() string               { return e.Error() }
func (e TransportError) GoString() string             { return e.Error() }
func (e TransportError) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e TransportError) MarshalJSON() ([]byte, error) { return safeErrorJSON("transport_error") }
func (e TransportError) SDKError()                    {}

func (e MutationUncertain) Error() string                { return "sber: mutation result uncertain; do not replay" }
func (e MutationUncertain) String() string               { return e.Error() }
func (e MutationUncertain) GoString() string             { return e.Error() }
func (e MutationUncertain) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e MutationUncertain) MarshalJSON() ([]byte, error) { return safeErrorJSON("mutation_uncertain") }
func (e MutationUncertain) SDKError()                    {}

func (e PinAuthError) Error() string {
	return "sber: authentication failed or requires owner interaction"
}
func (e PinAuthError) String() string               { return e.Error() }
func (e PinAuthError) GoString() string             { return e.Error() }
func (e PinAuthError) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e PinAuthError) MarshalJSON() ([]byte, error) { return safeErrorJSON("pin_auth_error") }
func (e PinAuthError) SDKError()                    {}

func (e PinCaptchaRequired) Error() string                { return "sber: CAPTCHA requires owner" }
func (e PinCaptchaRequired) String() string               { return e.Error() }
func (e PinCaptchaRequired) GoString() string             { return e.Error() }
func (e PinCaptchaRequired) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e PinCaptchaRequired) MarshalJSON() ([]byte, error) { return safeErrorJSON("captcha_required") }
func (e PinCaptchaRequired) Unwrap() error                { return &e.PinAuthError }

func (e PinOTPRequired) Error() string                { return "sber: one-time code requires owner" }
func (e PinOTPRequired) String() string               { return e.Error() }
func (e PinOTPRequired) GoString() string             { return e.Error() }
func (e PinOTPRequired) Format(f fmt.State, v rune)   { FormatError(f, e.Error()) }
func (e PinOTPRequired) MarshalJSON() ([]byte, error) { return safeErrorJSON("otp_required") }
func (e PinOTPRequired) Unwrap() error                { return &e.PinAuthError }
