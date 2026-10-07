package sber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

type client4PrivateCause struct{ text string }

func (e *client4PrivateCause) Error() string { return e.text }

type client4WrappedError struct {
	inner error
	text  string
}

func (e *client4WrappedError) Error() string { return e.text }
func (e *client4WrappedError) Unwrap() error { return e.inner }

func client4AssertPrivateErrorAbsent(t *testing.T, got error, marker string, raw error) {
	t.Helper()
	if got == nil {
		t.Fatal("failure lost")
	}
	var foreign *client4PrivateCause
	var wrapper *client4WrappedError
	if errors.Is(got, raw) || errors.As(got, &foreign) || errors.As(got, &wrapper) {
		t.Error("foreign error object retained")
	}
	badVerb := "%w"
	texts := []string{got.Error(), fmt.Errorf("%w", got).Error(), fmt.Sprintf(badVerb, got), fmt.Sprintf("%#v", got)}
	b, err := json.Marshal(got)
	if err == nil {
		texts = append(texts, string(b))
	}
	for _, text := range texts {
		if strings.Contains(text, marker) {
			t.Error("private error text escaped copied boundary")
		}
	}
}

func TestClientCycle4ConcreteErrorsAreOwnedSafeCopies(t *testing.T) {
	marker := strings.Join([]string{"cycle4", "synthetic", "private", "error"}, "-")
	rawCause := &client4PrivateCause{marker}
	cases := []struct {
		name           string
		input          error
		classification func(error) bool
	}{
		{"missing", &MissingSession{Message: marker}, func(e error) bool { var x *MissingSession; return errors.As(e, &x) && x.Message == "" }},
		{"insecure", &InsecureSessionFile{Message: marker}, func(e error) bool { var x *InsecureSessionFile; return errors.As(e, &x) && x.Message == "" }},
		{"expired", &AuthenticationExpired{Message: marker}, func(e error) bool { var x *AuthenticationExpired; return errors.As(e, &x) && x.Message == "" }},
		{"api", &APIError{Message: marker, StatusCode: 502}, func(e error) bool { var x *APIError; return errors.As(e, &x) && x.Message == "" && x.StatusCode == 502 }},
		{"rejected", &APIRejected{Message: marker, Code: marker, Text: marker, Title: marker, UUID: marker, System: marker}, func(e error) bool { var x *APIRejected; return errors.As(e, &x) && *x == (APIRejected{}) }},
		{"uncertain", &MutationUncertain{Message: marker}, func(e error) bool { var x *MutationUncertain; return errors.As(e, &x) && x.Message == "" }},
		{"transport", &TransportError{Code: marker, cause: rawCause}, func(e error) bool {
			var x *TransportError
			return errors.As(e, &x) && x.Code == "request_failed" && x.cause == nil
		}},
		{"pin", &PinAuthError{Message: marker, Code: marker, StatusCode: 403, ResetCookies: true}, func(e error) bool {
			var x *PinAuthError
			return errors.As(e, &x) && x.Message == "" && x.Code == "" && x.StatusCode == 403 && x.ResetCookies
		}},
		{"captcha", &PinCaptchaRequired{PinAuthError: PinAuthError{Message: marker, Code: marker}, ImageURL: &marker, AudioURL: &marker}, func(e error) bool {
			var x *PinCaptchaRequired
			return errors.As(e, &x) && x.Message == "" && x.Code == "" && x.ImageURL == nil && x.AudioURL == nil
		}},
		{"otp", &PinOTPRequired{PinAuthError: PinAuthError{Message: marker, Code: marker}}, func(e error) bool { var x *PinOTPRequired; return errors.As(e, &x) && x.Message == "" && x.Code == "" }},
		{"parse", NewParseError(marker), func(e error) bool { var x *ParseError; return errors.As(e, &x) && x.Field() == "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := &client4WrappedError{inner: tc.input, text: marker}
			got := clientSafeError(input)
			client4AssertPrivateErrorAbsent(t, got, marker, input)
			if got == tc.input || !tc.classification(got) {
				t.Error("recognized classification was not copied into safe owned fields")
			}
			tc.input = nil
			client4AssertPrivateErrorAbsent(t, clientSafeError(got), marker, input)
		})
	}
}
func TestClientCycle4SafeCopyPreservesCompositeContextAndCleanup(t *testing.T) {
	marker := strings.Join([]string{"cycle4", "synthetic", "private", "cause"}, "-")
	foreign := &client4PrivateCause{marker}
	calls := 0
	cleanup := &ClientCleanupError{cause: errors.Join(&MissingSession{Message: marker}, context.Canceled, foreign), cleanup: func() error { calls++; return nil }}
	input := &client4WrappedError{inner: errors.Join(cleanup, &MutationUncertain{Message: marker}, &TransportError{Code: "deadline_exceeded", cause: fmt.Errorf("%s: %w", marker, context.DeadlineExceeded)}, foreign), text: marker}
	got := clientSafeError(input)
	client4AssertPrivateErrorAbsent(t, got, marker, input)
	var pending *ClientCleanupError
	var uncertain *MutationUncertain
	var missing *MissingSession
	if !errors.As(got, &pending) || !errors.As(got, &uncertain) || !errors.As(got, &missing) || !errors.Is(got, context.Canceled) || !errors.Is(got, context.DeadlineExceeded) {
		t.Fatal("composite source classifications lost")
	}
	if pending == cleanup || pending.Close() != nil || calls != 1 {
		t.Fatal("cleanup ownership not carried into owned copy")
	}
	if missing.Message != "" || uncertain.Message != "" {
		t.Error("copied messages retain foreign text")
	}
}
