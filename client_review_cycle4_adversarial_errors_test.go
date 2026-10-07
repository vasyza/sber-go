package sber

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type client4ForgedClassification struct{ text string }

func (e *client4ForgedClassification) Error() string { return e.text }
func (*client4ForgedClassification) Is(error) bool   { return true }
func (e *client4ForgedClassification) As(target any) bool {
	if p, ok := target.(**ClientCleanupError); ok {
		*p = &ClientCleanupError{cause: e}
		return true
	}
	if p, ok := target.(**APIRejected); ok {
		*p = &APIRejected{Message: e.text}
		return true
	}
	return false
}

type client4ErrorCycle struct{}

func (*client4ErrorCycle) Error() string   { return "synthetic cycle" }
func (e *client4ErrorCycle) Unwrap() error { return e }

func TestClientCycle4TypedNilErrorsAreSanitized(t *testing.T) {
	inputs := []error{(*APIRejected)(nil), (*APIError)(nil), (*MissingSession)(nil), (*InsecureSessionFile)(nil), (*AuthenticationExpired)(nil), (*MutationUncertain)(nil), (*TransportError)(nil), (*PinAuthError)(nil), (*PinCaptchaRequired)(nil), (*PinOTPRequired)(nil), (*ClientCleanupError)(nil), (*PaginationLimitError)(nil)}
	for i, input := range inputs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Error("typed nil boundary panicked")
				}
			}()
			got := clientSafeError(input)
			if got == nil {
				t.Error("nonnil error swallowed")
			}
			var foreign *APIRejected
			if errors.As(got, &foreign) {
				t.Error("nil rejection fabricated definite outcome")
			}
		})
	}
}
func TestClientCycle4ForeignAsIsAndCyclesDoNotDeclassify(t *testing.T) {
	input := &client4ForgedClassification{text: "synthetic private"}
	got := clientSafeError(input)
	var pending *ClientCleanupError
	var rejected *APIRejected
	if errors.As(got, &pending) || errors.As(got, &rejected) || errors.Is(got, ErrClosed) || errors.Is(got, context.Canceled) || errors.Is(got, context.DeadlineExceeded) || errors.Is(got, input) {
		t.Error("foreign As/Is forged source classification")
	}
	if got := clientSafeError(&client4ErrorCycle{}); got == nil {
		t.Error("cyclic error swallowed")
	}
}
func TestClientCycle4OwnedErrorCopyDetachesMutableMetadata(t *testing.T) {
	remaining, lifetime := 3, 90
	original := &PinOTPRequired{PinAuthError: PinAuthError{StatusCode: 403, RemainingAttempts: &remaining, ResetCookies: true}, Lifetime: &lifetime}
	got := clientSafeError(original)
	remaining, lifetime = 0, 0
	var otp *PinOTPRequired
	if !errors.As(got, &otp) || otp == original || otp.RemainingAttempts == &remaining || otp.Lifetime == &lifetime || *otp.RemainingAttempts != 3 || *otp.Lifetime != 90 || !otp.ResetCookies {
		t.Error("owned metadata aliased input")
	}
	value := APIRejected{Message: "synthetic private"}
	var rejected *APIRejected
	if !errors.As(clientSafeError(value), &rejected) || rejected.Message != "" {
		t.Error("value classification not copied")
	}
}
