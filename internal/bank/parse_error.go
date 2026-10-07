// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"encoding/json"
	"fmt"
)

// ParseError denotes an unusable response, never evidence of an empty history.
// Field() explicitly reads the schema location; no input values or error causes
// are retained. The private immutable string indirection also protects fmt paths
// that bypass formatting methods. Native zero and nil pointers read empty.
type ParseError struct{ field *string }

// NewParseError retains the supplied schema location for explicit Field reads.
// Its ordinary error text/JSON are static; it never wraps a private cause.
func NewParseError(field string) *ParseError {
	if field == "" {
		return &ParseError{}
	}
	return &ParseError{field: &field}
}

// Field is a deliberate raw schema-location read, not a logging representation.
func (e *ParseError) Field() string {
	if e == nil || e.field == nil {
		return ""
	}
	return *e.field
}

func (e *ParseError) Error() string { return "sber: invalid domain data" }

func (e *ParseError) SDKError() {}

func (e *ParseError) String() string { return e.Error() }

func (e *ParseError) GoString() string { return e.Error() }

func (e *ParseError) Format(f fmt.State, verb rune) { domainSafeFormat(f, e.Error()) }

func (e *ParseError) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Error string `json:"error"`
	}{"parse_error"})
}

func domainSafeFormat(f fmt.State, text string) { _, _ = f.Write([]byte(text)) }
