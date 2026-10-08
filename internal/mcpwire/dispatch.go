package mcpwire

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vasyza/sber-go/internal/strictjson"
)

func protocolError(code int64) error {
	return &jsonrpc.Error{Code: code, Message: staticMessage(code)}
}

func failedTool() *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Tool execution failed"}}, IsError: true}
}

func callTool(ctx context.Context, tool Tool, arguments json.RawMessage) (result *mcp.CallToolResult, err error) {
	defer func() {
		if recover() != nil {
			result, err = nil, protocolError(-32603)
		}
	}()
	if arguments == nil {
		arguments = json.RawMessage(`{}`)
	}
	if len(arguments) > MaxFrameBytes || strictjson.Validate(arguments) != nil || !object(arguments) {
		return nil, protocolError(-32602)
	}
	if tool.Validate(ctx, append(json.RawMessage(nil), arguments...)) != nil {
		return nil, protocolError(-32602)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if tool.RawHandle != nil {
		raw, err := tool.RawHandle(ctx, append(json.RawMessage(nil), arguments...))
		if err != nil {
			return failedTool(), nil
		}
		if len(raw) > MaxFrameBytes {
			return nil, protocolError(-32603)
		}
		if _, ok := toolResultFields(raw); !ok {
			return nil, protocolError(-32603)
		}
		var decoded struct {
			Content           []*mcp.TextContent `json:"content"`
			StructuredContent json.RawMessage    `json:"structuredContent"`
			IsError           bool               `json:"isError"`
		}
		if json.Unmarshal(raw, &decoded) != nil {
			return nil, protocolError(-32603)
		}
		result = &mcp.CallToolResult{Content: make([]mcp.Content, 0, len(decoded.Content)), IsError: decoded.IsError}
		for _, text := range decoded.Content {
			result.Content = append(result.Content, text)
		}
		if len(decoded.StructuredContent) != 0 {
			result.StructuredContent = append(json.RawMessage(nil), decoded.StructuredContent...)
		}
		return result, nil
	}
	typed, err := tool.Handle(ctx, append(json.RawMessage(nil), arguments...))
	if err != nil {
		return failedTool(), nil
	}
	result = &mcp.CallToolResult{Content: make([]mcp.Content, 0, len(typed.Content)), IsError: typed.IsError}
	for _, text := range typed.Content {
		if !utf8.ValidString(text.Text) {
			return nil, protocolError(-32603)
		}
		result.Content = append(result.Content, &mcp.TextContent{Text: text.Text})
	}
	if len(typed.StructuredContent) != 0 {
		if len(typed.StructuredContent) > MaxFrameBytes || strictjson.Validate(typed.StructuredContent) != nil || !object(typed.StructuredContent) {
			return nil, protocolError(-32603)
		}
		result.StructuredContent = append(json.RawMessage(nil), typed.StructuredContent...)
	}
	return result, nil
}
