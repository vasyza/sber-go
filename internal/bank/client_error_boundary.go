package bank

import (
	"context"
	"errors"
	"reflect"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

// Interface markers and even SDK concrete fields are not evidence of safe
// foreign diagnostics. Copy classifications; never retain an outer wrapper or
// its arbitrary text/cause. Do not consult foreign As/Is implementations.
// The node budget also bounds cycles and overly broad joined error graphs.
func clientCopyError(e error, remaining *int) error {
	if e == nil {
		return nil
	}
	if *remaining <= 0 {
		return &sdkErrs.TransportError{Code: "request_failed"}
	}
	*remaining--
	value := reflect.ValueOf(e)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return &sdkErrs.TransportError{Code: "request_failed"}
		}
	}
	switch e {
	case sdkErrs.ErrClosed:
		return sdkErrs.ErrClosed
	case context.Canceled:
		return sdkErrs.NewTransportError("canceled", context.Canceled)
	case context.DeadlineExceeded:
		return sdkErrs.NewTransportError("timeout", context.DeadlineExceeded)
	}
	// Normalize only exact known pointer types. No reflective foreign fields.
	switch x := e.(type) {
	case *sdkErrs.MissingSession:
		if x != nil {
			e = *x
		}
	case *sdkErrs.InsecureSessionFile:
		if x != nil {
			e = *x
		}
	case *sdkErrs.AuthenticationExpired:
		if x != nil {
			e = *x
		}
	case *sdkErrs.APIError:
		if x != nil {
			e = *x
		}
	case *sdkErrs.APIRejected:
		if x != nil {
			e = *x
		}
	case *sdkErrs.MutationUncertain:
		if x != nil {
			e = *x
		}
	case *sdkErrs.TransportError:
		if x != nil {
			e = *x
		}
	case *sdkErrs.PinAuthError:
		if x != nil {
			e = *x
		}
	case *sdkErrs.PinCaptchaRequired:
		if x != nil {
			e = *x
		}
	case *sdkErrs.PinOTPRequired:
		if x != nil {
			e = *x
		}
	}
	switch x := e.(type) {
	case sdkErrs.MissingSession:
		return &sdkErrs.MissingSession{}
	case sdkErrs.InsecureSessionFile:
		return &sdkErrs.InsecureSessionFile{}
	case sdkErrs.AuthenticationExpired:
		return &sdkErrs.AuthenticationExpired{}
	case sdkErrs.APIError:
		return &sdkErrs.APIError{StatusCode: clientSafeStatus(x.StatusCode)}
	case sdkErrs.APIRejected:
		return &sdkErrs.APIRejected{}
	case sdkErrs.MutationUncertain:
		return &sdkErrs.MutationUncertain{}
	case sdkErrs.TransportError:
		var cause error
		if x.ContextCause() != nil {
			safe := clientCopyError(x.ContextCause(), remaining)
			if errors.Is(safe, context.Canceled) {
				cause = context.Canceled
			}
			if errors.Is(safe, context.DeadlineExceeded) {
				cause = errors.Join(cause, context.DeadlineExceeded)
			}
		}
		return sdkErrs.NewTransportError(clientSafeTransportCode(x.Code), cause)
	case sdkErrs.PinAuthError:
		return clientCopyPINError(x)
	case sdkErrs.PinCaptchaRequired:
		return &sdkErrs.PinCaptchaRequired{PinAuthError: *clientCopyPINError(x.PinAuthError)}
	case sdkErrs.PinOTPRequired:
		out := &sdkErrs.PinOTPRequired{PinAuthError: *clientCopyPINError(x.PinAuthError)}
		if x.Lifetime != nil && *x.Lifetime >= 0 {
			v := *x.Lifetime
			out.Lifetime = &v
		}
		return out
	case *ParseError:
		return &ParseError{}
	case *PaginationLimitError:
		if x != nil && x.MaxPages >= 0 {
			return &PaginationLimitError{MaxPages: x.MaxPages}
		}
		return &PaginationLimitError{}
	case *ClientCleanupError:
		if x != nil && x.cleanup != nil {
			return &ClientCleanupError{cause: clientCopyError(x.cause, remaining), cleanup: x.cleanup}
		}
		return &sdkErrs.TransportError{Code: "request_failed"}
	}
	switch x := e.(type) {
	case interface{ Unwrap() []error }:
		children := x.Unwrap()
		copied := make([]error, 0)
		for _, child := range children {
			if *remaining <= 0 {
				copied = append(copied, &sdkErrs.TransportError{Code: "request_failed"})
				break
			}
			if child != nil {
				copied = append(copied, clientCopyError(child, remaining))
			} else {
				*remaining--
			}
		}
		if len(copied) > 0 {
			return errors.Join(copied...)
		}
	case interface{ Unwrap() error }:
		if child := x.Unwrap(); child != nil {
			return clientCopyError(child, remaining)
		}
	}
	return &sdkErrs.TransportError{Code: "request_failed"}
}
func clientSafeStatus(status int) int {
	if status >= 100 && status <= 599 {
		return status
	}
	return 0
}
func clientSafeTransportCode(code string) string {
	switch code {
	case "invalid_proxy", "proxy_authentication", "proxy_connect", "proxy_failed", "proxy_tls_closed":
		return code
	case "canceled", "close_failed", "endpoint_not_allowed", "invalid_ca_bundle", "invalid_client_options", "invalid_context", "invalid_encoding", "invalid_headers", "invalid_json_body", "invalid_mutation_sequence", "invalid_options", "mutation_disabled", "mutation_sequence_closed", "request_failed", "response_too_large", "retry_forbidden", "reused_transport", "timeout", "tls_expired", "tls_hostname", "tls_invalid", "tls_untrusted", "unsafe_page_id", "unsafe_request", "unsupported_cookie_metadata", "unsupported_encoding":
		return code
	}
	return "request_failed"
}
func clientCopyPINError(x sdkErrs.PinAuthError) *sdkErrs.PinAuthError {
	out := &sdkErrs.PinAuthError{StatusCode: clientSafeStatus(x.StatusCode), ResetCookies: x.ResetCookies}
	if x.RemainingAttempts != nil && *x.RemainingAttempts >= 0 {
		v := *x.RemainingAttempts
		out.RemainingAttempts = &v
	}
	switch x.Code {
	case "invalid_frontend_config", "login_page_rejected", "invalid_redirect", "unsafe_captcha_url", "unsafe_endpoint", "unsafe_redirect", "unsupported_browser_state":
		out.Code = x.Code
	}
	return out
}
