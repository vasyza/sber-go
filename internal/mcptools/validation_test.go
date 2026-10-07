package mcptools

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateArgumentsOriginalIntegrity(t *testing.T) {
	invalid := [][]byte{
		[]byte(`{"password":"synthetic-marker"}`),
		[]byte(`{"Profile":"default"}`),
		[]byte(`{"profile":"a","pro\u0066ile":"b"}`),
		[]byte(`{"profile":"\ud800"}`),
		append(append([]byte(`{"profile":"`), 0xff), []byte(`"}`)...),
		[]byte(`{} {}`), []byte(`[]`), []byte(`null`),
	}
	for _, raw := range invalid {
		before := append([]byte(nil), raw...)
		if err := ValidateArguments("sber_auth_start", raw, false); err == nil || !strings.Contains(err.Error(), "invalid MCP arguments") {
			t.Fatal("malformed arguments were accepted or error was nonstatic")
		}
		if !bytes.Equal(raw, before) {
			t.Fatal("arguments mutated")
		}
	}
	if err := ValidateArguments("sber_auth_start", []byte(`{"profile":"default"}`), false); err != nil {
		t.Fatal(err)
	}
}
