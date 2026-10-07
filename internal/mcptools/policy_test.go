package mcptools

import (
	"testing"
)

func TestValidateArgumentsUnregisteredToolCannotBypassPolicy(t *testing.T) {
	for _, name := range []string{"sber_card_rename", "sber_transfer_start", "sber_transfer_prepare", "sber_transfer_confirm", "sber_transfer_resolve_uncertain", "sber_other", "SBER_products"} {
		if err := ValidateArguments(name, []byte(`{}`), false); err == nil || err.Error() != "MCP tool unavailable" {
			t.Fatalf("unregistered tool %s reachable or wrong static error", name)
		}
	}
	if err := ValidateArguments("sber_transfer_start", []byte(`{}`), true); err != nil {
		t.Fatal(err)
	}
}
