package mcptools

import (
	"strings"
	"testing"
)

func TestValidateArgumentsSizeBound(t *testing.T) {
	const maximum = 1 << 20
	prefix, suffix := `{"profile":"`, `"}`
	atLimit := []byte(prefix + strings.Repeat("x", maximum-len(prefix)-len(suffix)) + suffix)
	if len(atLimit) != maximum {
		t.Fatal("invalid exact-boundary fixture")
	}
	if err := ValidateArguments("sber_auth_start", atLimit, false); err != nil {
		t.Fatal("exact-boundary schema-valid arguments rejected")
	}
	tooLarge := []byte(prefix + strings.Repeat("x", maximum-len(prefix)-len(suffix)+1) + suffix)
	if err := ValidateArguments("sber_auth_start", tooLarge, false); err != ErrInvalidArguments {
		t.Fatal("oversized argument document accepted or error was nonstatic")
	}
}
