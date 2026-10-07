package mcpwire

import (
	"context"
	"encoding/json"
	"strings"
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

func TestCatalogKeepsLocalControlDistinctFromFinancialMutations(t *testing.T) {
	closeTool, mutation := syntheticTool("session_close"), syntheticTool("financial_mutation")
	closeTool.ReadOnly, closeTool.LocalControl = false, true
	mutation.ReadOnly = false
	server, err := New(Options{Tools: []Tool{closeTool, mutation}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(server.toolList())
	if err != nil || strings.Contains(string(raw), "financial_mutation") || !strings.Contains(string(raw), `"readOnlyHint":false`) || !strings.Contains(string(raw), "session_close") {
		t.Fatal("lifecycle control enabled financial registration or misreported its side effect")
	}
}

func TestCatalogRejectsUnsafeMetadataNonObjectRootsAndUnboundedListing(t *testing.T) {
	for _, item := range []struct {
		name string
		info Implementation
	}{
		{"name-UTF8", Implementation{Name: string([]byte{0xff}), Version: "test"}},
		{"version-UTF8", Implementation{Name: "synthetic", Version: string([]byte{0xff})}},
		{"name-size", Implementation{Name: strings.Repeat("x", 129), Version: "test"}},
	} {
		t.Run(item.name, func(t *testing.T) {
			if _, err := New(Options{Info: item.info}); err != ErrConfiguration {
				t.Fatal("want static metadata rejection before encoding")
			}
		})
	}
	for i, schema := range []string{`{"type":"string"}`, `{}`, `{"type":["object","null"]}`} {
		t.Run("root-"+string(rune('a'+i)), func(t *testing.T) {
			tool := syntheticTool("local")
			tool.InputSchema = json.RawMessage(schema)
			if _, err := New(Options{Tools: []Tool{tool}}); err != ErrConfiguration {
				t.Fatal("MCP input schema must have object root")
			}
		})
	}
	t.Run("description-size", func(t *testing.T) {
		tool := syntheticTool("local")
		tool.Description = strings.Repeat("x", 4097)
		if _, err := New(Options{Tools: []Tool{tool}}); err != ErrConfiguration {
			t.Fatal("want bounded description")
		}
	})
	t.Run("catalog-count", func(t *testing.T) {
		tools := make([]Tool, 65)
		for i := range tools {
			tools[i] = syntheticTool("synthetic-" + strings.Repeat("x", i+1))
		}
		if _, err := New(Options{Tools: tools}); err != ErrConfiguration {
			t.Fatal("want bounded catalog registration")
		}
	})
	t.Run("catalog-encoded-size", func(t *testing.T) {
		tools := make([]Tool, 30)
		for i := range tools {
			tools[i] = syntheticTool("synthetic-" + strings.Repeat("x", i+1))
			tools[i].InputSchema = json.RawMessage(`{"type":"object","title":"` + strings.Repeat("x", 60000) + `"}`)
		}
		if _, err := New(Options{Tools: tools}); err != ErrConfiguration {
			t.Fatal("want catalog fitting the worst-ID output frame")
		}
	})
	t.Run("default-info", func(t *testing.T) {
		server, err := New(Options{})
		if err != nil {
			t.Fatal(err)
		}
		if server.info.Name == "" || server.info.Version == "" {
			t.Fatal("want valid default serverInfo")
		}
	})
}

func TestNewRejectsInvalidCatalog(t *testing.T) {
	cases := []struct {
		name      string
		change    func(*Tool)
		duplicate bool
	}{
		{"nil-validator", func(t *Tool) { t.Validate = nil }, false},
		{"nil-handler", func(t *Tool) { t.Handle = nil }, false},
		{"invalid-name", func(t *Tool) { t.Name = "not a name" }, false},
		{"invalid-native-utf8", func(t *Tool) { t.Description = string([]byte{0xff}) }, false},
		{"duplicate-name", func(t *Tool) {}, true},
		{"null-schema", func(t *Tool) { t.InputSchema = json.RawMessage(`null`) }, false},
		{"duplicate-escaped-schema-key", func(t *Tool) { t.InputSchema = json.RawMessage(`{"type":"object","\u0074ype":"string"}`) }, false},
		{"unsupported-ref", func(t *Tool) {
			t.InputSchema = json.RawMessage(`{"type":"object","$ref":"https://example.invalid/schema"}`)
		}, false},
		{"unsupported-dialect", func(t *Tool) {
			t.InputSchema = json.RawMessage(`{"type":"object","$schema":"http://json-schema.org/draft-07/schema#"}`)
		}, false},
		{"bad-required", func(t *Tool) { t.InputSchema = json.RawMessage(`{"type":"object","required":["absent"]}`) }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := syntheticTool("local")
			tc.change(&tool)
			tools := []Tool{tool}
			if tc.duplicate {
				tools = append(tools, tool)
			}
			if _, err := New(Options{Tools: tools}); err == nil {
				t.Fatal("want static configuration rejection")
			}
		})
	}
}
