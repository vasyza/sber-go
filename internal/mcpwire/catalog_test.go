package mcpwire

import (
	"context"
	"encoding/json"
	"testing"
)

func syntheticTool(name string) Tool {
	return Tool{Name: name, Description: "Synthetic local handler only", ReadOnly: true,
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
		Validate:    func(context.Context, json.RawMessage) error { return nil },
		Handle: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: []TextContent{{Text: "synthetic"}}}, nil
		},
	}
}

func TestCatalogDefensiveReadOnlyList(t *testing.T) {
	tools := []Tool{syntheticTool("zeta"), syntheticTool("alpha"), syntheticTool("mutate")}
	tools[2].ReadOnly = false
	server, err := New(Options{Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	tools[0].Name = "changed"
	tools[1].InputSchema[2] = 'X'
	responses := serveText(t, server, `{"jsonrpc":"2.0","id":"list","method":"tools/list","params":{`+currentMeta+`}}`+"\n")
	fields := resultFields(t, responses[0])
	var listed []struct {
		Name        string
		InputSchema json.RawMessage
		Annotations struct{ ReadOnlyHint bool }
	}
	if err := json.Unmarshal(fields["tools"], &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 || listed[0].Name != "alpha" || listed[1].Name != "zeta" {
		t.Fatalf("want only immutable read-only handlers in sorted order; got %s", fields["tools"])
	}
	if !listed[0].Annotations.ReadOnlyHint || string(listed[0].InputSchema) != `{"type":"object","additionalProperties":false}` {
		t.Fatalf("want defensive schema copy and read-only hint; got %s", fields["tools"])
	}
}
