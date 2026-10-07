package mcptools

import (
	"encoding/json"
	"errors"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// MaximumArgumentBytes bounds validation of a complete original document.
const MaximumArgumentBytes = 1 << 20

// ErrInvalidArguments is static: no input key, value, secret or decoder cause
// enters MCP diagnostics.
var ErrInvalidArguments = errors.New("invalid MCP arguments")

// ErrToolUnavailable hides unregistered and default-disabled financial tools.
var ErrToolUnavailable = errors.New("MCP tool unavailable")

// ValidateArguments checks the complete original document before any permissive
// decoder can replace Unicode or collapse escaped duplicate argument names.
// It performs no I/O and does not confer bank or transaction authorization.
func ValidateArguments(name string, raw []byte, allowWrites bool) error {
	var selected *contract
	for _, c := range sourceContracts() {
		if c.name == name {
			copy := c
			selected = &copy
			break
		}
	}
	if selected == nil || (selected.mutation && !allowWrites) {
		return ErrToolUnavailable
	}
	if len(raw) > MaximumArgumentBytes {
		return ErrInvalidArguments
	}
	if strictjson.Validate(raw) != nil {
		return ErrInvalidArguments
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return ErrInvalidArguments
	}
	for key := range values {
		known := false
		for _, a := range selected.arguments {
			if key == a.name {
				known = true
				break
			}
		}
		if !known {
			return ErrInvalidArguments
		}
	}
	for _, a := range selected.arguments {
		value, present := values[a.name]
		if !present {
			if !a.optional {
				return ErrInvalidArguments
			}
			continue
		}
		s := string(value)
		switch a.kind {
		case "nullable_string":
			if s == "null" {
				continue
			}
			fallthrough
		case "string":
			var decoded string
			if len(s) == 0 || s[0] != '"' || json.Unmarshal(value, &decoded) != nil {
				return ErrInvalidArguments
			}
		case "boolean":
			if s != "true" && s != "false" {
				return ErrInvalidArguments
			}
		case "integer":
			if len(s) == 0 || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) {
				return ErrInvalidArguments
			}
			if !integralNumber(s) {
				return ErrInvalidArguments
			}
		}
	}
	return nil
}
