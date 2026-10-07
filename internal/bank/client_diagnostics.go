package bank

import (
	"encoding/json"
	"fmt"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

// The terminal indirection also protects diagnostic verbs which fmt handles
// itself without consulting Formatter. Unwrap preserves deliberate errors.As
// access to response metadata; it is never followed by diagnostic reflection.
type clientDiagnosticError struct{ value **error }

func clientDiagnostic(err error) error {
	if err == nil {
		return nil
	}
	pointer := &err
	return clientDiagnosticError{value: &pointer}
}
func (e clientDiagnosticError) Unwrap() error                { return **e.value }
func (e clientDiagnosticError) Error() string                { return e.Unwrap().Error() }
func (e clientDiagnosticError) String() string               { return e.Error() }
func (e clientDiagnosticError) GoString() string             { return e.Error() }
func (e clientDiagnosticError) Format(f fmt.State, _ rune)   { sdkErrs.FormatError(f, e.Error()) }
func (e clientDiagnosticError) MarshalJSON() ([]byte, error) { return json.Marshal(e.Unwrap()) }
