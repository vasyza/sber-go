package mcptools

import (
	"reflect"
	"testing"
)

func TestCatalogReadOnlyRegistration(t *testing.T) {
	want := []string{"sber_setup_status", "sber_auth_start", "sber_auth_continue", "sber_auth_resend_otp", "sber_session_info", "sber_session_close", "sber_products", "sber_operations", "sber_operations_page"}
	definitions := Catalog(false)
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Mutation {
			t.Fatal("mutation registered by default")
		}
		names = append(names, definition.Name)
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("catalog %v, want %v", names, want)
	}
}
