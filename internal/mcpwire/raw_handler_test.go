package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
)

func TestInjectedRawHandlerUsesValidatedArgumentCopies(t *testing.T) {
	var validations, calls atomic.Int32
	const arguments = `{"synthetic":"native"}`
	tool := syntheticTool("raw")
	tool.Handle = nil
	tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"synthetic":{"type":"string"}},"required":["synthetic"],"additionalProperties":false}`)
	tool.Validate = func(ctx context.Context, raw json.RawMessage) error {
		validations.Add(1)
		if !bytes.Equal(raw, []byte(arguments)) {
			t.Error("validator did not get original bytes")
		}
		raw[2] = 'x' // This must not alter the bytes passed to the handler.
		return nil
	}
	tool.RawHandle = func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		calls.Add(1)
		if string(raw) != arguments {
			t.Error("handler did not get independent bytes")
		}
		return json.RawMessage(`{"content":[{"type":"text","text":"synthetic native result"}],"structuredContent":{"n":9007199254740993}}`), nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, toolCall(`"raw-call"`, "raw", arguments, true))
	if len(responses) != 1 {
		t.Fatalf("want one response; got %d", len(responses))
	}
	result := resultFields(t, responses[0])
	if string(result["resultType"]) != `"complete"` || string(result["structuredContent"]) != `{"n":9007199254740993}` {
		t.Fatal("want exact typed raw JSON result")
	}
	if validations.Load() != 1 || calls.Load() != 1 {
		t.Fatal("want exactly one validation and execution")
	}
}
