package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRawOutputRejectsOriginalUnicodeDuplicatesAndResultShape(t *testing.T) {
	outputs := []string{
		`{"content":[],"structuredContent":{"x":1,"\u0078":2}}`,
		`{"content":[{"type":"text","text":"\ud800"}]}`,
		"{\"content\":[{\"type\":\"text\",\"text\":\"" + string([]byte{0xff}) + "\"}]}",
		`{"content":[]} {}`, `null`, `[]`, `{}`, `{"Content":[]}`, `{"content":null}`,
		`{"content":[{"type":"text","Text":"private"}]}`,
		`{"content":[],"structuredContent":[]}`,
		`{"content":[],"isError":"false"}`,
		`{"content":[],"resultType":"input_required"}`,
	}
	for i, raw := range outputs {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			calls := 0
			tool := syntheticTool("raw")
			tool.Handle = nil
			tool.RawHandle = func(context.Context, json.RawMessage) (json.RawMessage, error) {
				calls++
				return json.RawMessage(raw), nil
			}
			server, err := New(Options{Tools: []Tool{tool}})
			if err != nil {
				t.Fatal(err)
			}
			reply := serveText(t, server, toolCall(`1`, "raw", `{}`, true))[0]
			requireCode(t, reply, -32603)
			if calls != 1 {
				t.Fatalf("want exactly one call; got %d", calls)
			}
		})
	}
}

func TestNativeTypedOutputCannotReplaceInvalidUTF8(t *testing.T) {
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{Content: []TextContent{{Text: string([]byte{0xff})}}}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, toolCall(`1`, "local", `{}`, true))
	requireCode(t, responses[0], -32603)
}

func TestCallbackErrorsAndPanicsStayStaticWithoutRetry(t *testing.T) {
	for _, stage := range []string{"validation-error", "validation-panic", "handler-error", "handler-panic"} {
		t.Run(stage, func(t *testing.T) {
			validations, calls := 0, 0
			tool := syntheticTool("local")
			tool.Validate = func(context.Context, json.RawMessage) error {
				validations++
				if stage == "validation-panic" {
					panic("SYNTHETIC-PRIVATE-PANIC")
				}
				if stage == "validation-error" {
					return errors.New("SYNTHETIC-PRIVATE-ERROR")
				}
				return nil
			}
			tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
				calls++
				if stage == "handler-panic" {
					panic("SYNTHETIC-PRIVATE-PANIC")
				}
				return ToolResult{Content: []TextContent{{Text: "SYNTHETIC-PRIVATE-RESULT"}}}, errors.New("SYNTHETIC-PRIVATE-ERROR")
			}
			server, err := New(Options{Tools: []Tool{tool}})
			if err != nil {
				t.Fatal(err)
			}
			reply := serveText(t, server, toolCall(`1`, "local", `{}`, true))[0]
			encoded, _ := json.Marshal(reply)
			if bytes.Contains(encoded, []byte("SYNTHETIC-PRIVATE")) {
				t.Fatal("private callback value leaked")
			}
			if validations != 1 || (strings.HasPrefix(stage, "validation") && calls != 0) || (!strings.HasPrefix(stage, "validation") && calls != 1) {
				t.Fatal("want no retry/no dispatch after failed validation")
			}
			if stage == "validation-error" {
				requireCode(t, reply, -32602)
			} else if strings.HasSuffix(stage, "panic") {
				requireCode(t, reply, -32603)
			} else {
				result := resultFields(t, reply)
				if string(result["isError"]) != "true" || !bytes.Contains(result["content"], []byte("Tool execution failed")) {
					t.Fatal("want sanitized tool execution failure")
				}
			}
		})
	}
}

func TestEncodedOutputExactOneMiBBoundary(t *testing.T) {
	info := Implementation{Name: "synthetic", Version: "test"}
	empty := map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": ""}}, "resultType": "complete", "_meta": map[string]any{"io.modelcontextprotocol/serverInfo": info}}}
	encoded, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	for _, delta := range []int{0, 1} {
		t.Run(string(rune('a'+delta)), func(t *testing.T) {
			text := strings.Repeat("x", frameLimitTest-len(encoded)+delta)
			calls := 0
			tool := syntheticTool("local")
			tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
				calls++
				return ToolResult{Content: []TextContent{{Text: text}}}, nil
			}
			server, err := New(Options{Info: info, Tools: []Tool{tool}})
			if err != nil {
				t.Fatal(err)
			}
			reply := serveText(t, server, toolCall(`1`, "local", `{}`, true))[0]
			if delta == 0 {
				result := resultFields(t, reply)
				var content []struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(result["content"], &content) != nil || len(content) != 1 || len(content[0].Text) != len(text) {
					t.Fatal("want large output preserved")
				}
			} else {
				requireCode(t, reply, -32603)
			}
			if calls != 1 {
				t.Fatal("must not retry on output rejection")
			}
		})
	}
}
