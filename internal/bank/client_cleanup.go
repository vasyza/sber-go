package bank

import (
	"encoding/json"
	"fmt"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

// ClientCleanupError keeps retryable ownership when a constructor cannot finish
// and closing its discarded transport also fails. The client is never usable.
// Inspect the original safe failure with errors.Is/As, then retry Close on this
// error (not construction/login/business requests). Normal diagnostics redact it.
type ClientCleanupError struct {
	cause   error
	cleanup func() error
}

func (*ClientCleanupError) Error() string {
	return "sber: client construction failed; transport cleanup pending"
}
func (e *ClientCleanupError) String() string             { return e.Error() }
func (e *ClientCleanupError) GoString() string           { return e.Error() }
func (e *ClientCleanupError) Format(f fmt.State, v rune) { sdkErrs.FormatError(f, e.Error()) }
func (*ClientCleanupError) MarshalJSON() ([]byte, error) {
	return json.Marshal("client_cleanup_pending")
}
func (*ClientCleanupError) SDKError()       {}
func (e *ClientCleanupError) Unwrap() error { return e.cause }
func (e *ClientCleanupError) Close() error  { return e.cleanup() }
func clientDiscardOwnedConstruction(owned *clientOwnedTransport, primary error) error {
	if owned == nil {
		return primary
	}
	if e := owned.close(); e != nil {
		return &ClientCleanupError{cause: primary, cleanup: owned.close}
	}
	return primary
}
