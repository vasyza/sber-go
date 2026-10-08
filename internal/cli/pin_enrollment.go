package cli

import (
	"context"
	"errors"
	"io"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/enrollment"
)

func enrollOwnerPIN(ctx context.Context, a Authentication, auth PINEnrollmentAuthenticator, diagnostics io.Writer) (sber.SessionBundle, error) {
	for attempt := 0; ; attempt++ {
		pin, err := readNewPIN(ctx, a, auth, diagnostics)
		if err != nil {
			return sber.SessionBundle{}, err
		}
		bundle, err := auth.CreatePIN(ctx, pin)
		pin = ""
		if err == nil {
			return bundle, nil
		}
		if attempt >= 2 || !definitePINPolicyRejection(err) {
			return sber.SessionBundle{}, err
		}
		// A validated policy rejection did not enroll this PIN. The next
		// attempt requires a new owner prompt and confirmation, never replay.
		if diagnostics != nil {
			if _, err := io.WriteString(diagnostics, "The bank did not accept this new PIN.\nEnter a different new online banking PIN, or press Ctrl+C.\n"); err != nil {
				return sber.SessionBundle{}, enrollment.ErrPrepare
			}
		}
	}
}

func definitePINPolicyRejection(err error) bool {
	var pin *sber.PinAuthError
	return errors.As(err, &pin) && (pin.StatusCode == 400 || pin.StatusCode == 422) &&
		(pin.Code == "invalid_pin" || pin.Code == "invalid_pin_birthdate") && !pin.ResetCookies &&
		(pin.RemainingAttempts == nil || *pin.RemainingAttempts > 0)
}
