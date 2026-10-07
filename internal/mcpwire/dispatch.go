package mcpwire

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/vasyza/sber-go/internal/strictjson"
)

func (server *Server) call(ctx context.Context, params map[string]json.RawMessage) (fields map[string]any, code int) {
	defer func() {
		if recover() != nil {
			fields, code = nil, -32603
		}
	}()
	if !allowedFields(params, "_meta", "name", "arguments") {
		return nil, -32602
	}
	var name string
	if json.Unmarshal(params["name"], &name) != nil {
		return nil, -32602
	}
	arguments := params["arguments"]
	if arguments == nil {
		arguments = json.RawMessage(`{}`)
	}
	if !object(arguments) {
		return nil, -32602
	}
	for _, tool := range server.tools {
		if tool.Name != name {
			continue
		}
		if tool.Validate(ctx, append(json.RawMessage(nil), arguments...)) != nil {
			return nil, -32602
		}
		if tool.RawHandle != nil {
			raw, err := tool.RawHandle(ctx, append(json.RawMessage(nil), arguments...))
			if err != nil {
				return map[string]any{"content": []any{map[string]any{"type": "text", "text": "Tool execution failed"}}, "isError": true}, 0
			}
			fields, ok := toolResultFields(raw)
			if !ok {
				return nil, -32603
			}
			return fields, 0
		}
		result, err := tool.Handle(ctx, append(json.RawMessage(nil), arguments...))
		if err != nil {
			result = ToolResult{Content: []TextContent{{Text: "Tool execution failed"}}, IsError: true}
		}
		content := make([]any, 0, len(result.Content))
		for _, item := range result.Content {
			if !utf8.ValidString(item.Text) {
				return nil, -32603
			}
			content = append(content, map[string]any{"type": "text", "text": item.Text})
		}
		fields := map[string]any{"content": content, "isError": result.IsError}
		if len(result.StructuredContent) != 0 {
			if strictjson.Validate(result.StructuredContent) != nil || !object(result.StructuredContent) {
				return nil, -32603
			}
			fields["structuredContent"] = result.StructuredContent
		}
		return fields, 0
	}
	return nil, -32602
}
