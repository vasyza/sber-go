package mcpwire

import (
	"encoding/json"
	"testing"
)

const legacyInitialize = `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"synthetic-client","version":"test"}}}` + "\n"
const legacyInitialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"

func resultFields(t *testing.T, response map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response["result"], &fields); err != nil {
		t.Fatalf("want result; got %s", response["error"])
	}
	return fields
}

func TestLegacyHandshake(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	in := `{"jsonrpc":"2.0","id":"pre","method":"tools/list"}` + "\n" + legacyInitialize + `{"jsonrpc":"2.0","id":"notready","method":"tools/list"}` + "\n" + legacyInitialized + `{"jsonrpc":"2.0","id":"ready","method":"tools/list"}` + "\n"
	responses := serveText(t, server, in)
	if len(responses) != 4 {
		t.Fatalf("want four responses, notification silent; got %d", len(responses))
	}
	requireCode(t, responses[0], -32602)
	result := resultFields(t, responses[1])
	if string(result["protocolVersion"]) != `"2025-11-25"` || !object(result["serverInfo"]) {
		t.Fatalf("want legacy initialize result; got %s", responses[1]["result"])
	}
	if _, ok := result["resultType"]; ok {
		t.Fatal("legacy result must not contain current resultType")
	}
	// The SDK accepts calls after initialize, before the initialized notification.
	resultFields(t, responses[2])
	result = resultFields(t, responses[3])
	if string(result["tools"]) != `[]` {
		t.Fatalf("want empty legacy tools list; got %s", responses[3]["result"])
	}
}
