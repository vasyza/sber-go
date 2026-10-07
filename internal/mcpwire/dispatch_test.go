package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
)

func toolCall(id, name, arguments string, modern bool) string {
	params := `"name":` + strconvQuote(name) + `,"arguments":` + arguments
	if modern {
		params += "," + currentMeta
	}
	return `{"jsonrpc":"2.0","id":` + id + `,"method":"tools/call","params":{` + params + `}}` + "\n"
}
func strconvQuote(text string) string { raw, _ := json.Marshal(text); return string(raw) }

func TestDispatchExactArgumentsOnce(t *testing.T) {
	const arguments = `{"label":"synthetic","sequence":9007199254740993}`
	var validations, calls atomic.Int32
	tool := syntheticTool("local")
	tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"label":{"type":"string"},"sequence":{"type":"integer"}},"required":["label","sequence"],"additionalProperties":false}`)
	tool.Validate = func(ctx context.Context, raw json.RawMessage) error {
		validations.Add(1)
		if !bytes.Equal(raw, []byte(arguments)) {
			return errors.New("unsafe argument details")
		}
		return nil
	}
	tool.Handle = func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		if string(raw) != arguments {
			t.Errorf("want unchanged argument bytes; got %q", raw)
		}
		return ToolResult{Content: []TextContent{{Text: "synthetic local result"}}, StructuredContent: raw}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, toolCall(`"current"`, "local", arguments, true)+legacyInitialize+legacyInitialized+toolCall(`"legacy"`, "local", arguments, false))
	if len(responses) != 3 {
		t.Fatalf("want three responses; got %d", len(responses))
	}
	byID := make(map[string]map[string]json.RawMessage)
	for _, response := range responses {
		id := string(response["id"])
		if _, duplicate := byID[id]; duplicate { t.Fatal("duplicate response ID") }
		byID[id] = response
	}
	currentResponse, currentOK := byID[`"current"`]
	legacyResponse, legacyOK := byID[`"legacy"`]
	if !currentOK || !legacyOK { t.Fatal("missing correlated tool result") }
	current, legacy := resultFields(t, currentResponse), resultFields(t, legacyResponse)
	if string(current["resultType"]) != `"complete"` || string(current["structuredContent"]) != arguments {
		t.Fatalf("want exact current tool result; got %s", responses[0]["result"])
	}
	if _, ok := legacy["resultType"]; ok {
		t.Fatal("legacy resultType forbidden")
	}
	if string(legacy["structuredContent"]) != arguments {
		t.Fatalf("want exact legacy tool result; got %s", responses[2]["result"])
	}
	if calls.Load() != 2 || validations.Load() != 2 {
		t.Fatalf("want dispatch and validation once per request; got %d/%d", calls.Load(), validations.Load())
	}
}
