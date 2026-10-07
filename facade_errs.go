// Compatibility facade; implementation lives in internal/errs.
package sber

import (
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

type SberError = sdkErrs.SberError

var ErrClosed = sdkErrs.ErrClosed

type MissingSession = sdkErrs.MissingSession
type InsecureSessionFile = sdkErrs.InsecureSessionFile
type AuthenticationExpired = sdkErrs.AuthenticationExpired
type APIError = sdkErrs.APIError
type APIRejected = sdkErrs.APIRejected
type MutationUncertain = sdkErrs.MutationUncertain
type TransportError = sdkErrs.TransportError

// NewTransportError retains only cancellation and deadline identities. Raw
// transport errors may contain sensitive URLs and are never stored or unwrapped.
func NewTransportError(code string, cause error) *TransportError {
	return sdkErrs.NewTransportError(code, cause)
}

type PinAuthError = sdkErrs.PinAuthError
type PinCaptchaRequired = sdkErrs.PinCaptchaRequired
type PinOTPRequired = sdkErrs.PinOTPRequired
type ApiError = sdkErrs.ApiError
type ApiRejected = sdkErrs.ApiRejected
type PinOtpRequired = sdkErrs.PinOtpRequired
