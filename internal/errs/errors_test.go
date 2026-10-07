package errs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// All tokens below are synthetic, never bank responses or owner secrets.
func TestTypedErrorsRedactAllRepresentations(t *testing.T) {
	const secret = "synthetic-cookie-token-process-html"
	values := []error{
		&MissingSession{Message: secret}, &InsecureSessionFile{Message: secret},
		&AuthenticationExpired{Message: secret}, &APIError{Message: secret},
		&APIRejected{Message: secret, Code: secret, Title: secret, Text: secret, UUID: secret, System: secret},
		&TransportError{Code: "canceled", cause: context.Canceled}, &MutationUncertain{Message: secret},
		&PinAuthError{Message: secret, Code: secret},
		&PinCaptchaRequired{PinAuthError: PinAuthError{Message: secret, Code: secret}, ImageURL: &[]string{secret}[0]},
		&PinOTPRequired{PinAuthError: PinAuthError{Message: secret, Code: "otp_required"}},
	}
	for _, e := range values {
		t.Run(fmt.Sprintf("%T", e), func(t *testing.T) {
			for _, text := range []string{e.Error(), fmt.Sprint(e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e)} {
				if strings.Contains(text, secret) {
					t.Fatal("error leaked synthetic secret")
				}
			}
			b, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(b, []byte(secret)) {
				t.Fatal("JSON leaked secret")
			}
			var log bytes.Buffer
			slog.New(slog.NewJSONHandler(&log, nil)).Error("safe", "error", e)
			if strings.Contains(log.String(), secret) {
				t.Fatal("slog leaked secret")
			}
		})
	}
	var challenge *PinAuthError
	if !errors.As(values[8], &challenge) {
		t.Fatal("challenge must retain typed PIN error")
	}
	if !errors.Is(values[5], context.Canceled) {
		t.Fatal("context cancellation identity lost")
	}
}
