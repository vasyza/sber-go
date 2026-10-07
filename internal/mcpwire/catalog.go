package mcpwire

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"
)

type TextContent struct{ Text string }
type ToolResult struct {
	Content           []TextContent
	StructuredContent json.RawMessage
	IsError           bool
}
// Validator must reject invalid tool arguments before a handler can run.
type Validator func(context.Context, json.RawMessage) error
// Handler returns a strict, complete tool-result object; not a JSON-RPC envelope.
type Handler func(context.Context, json.RawMessage) (json.RawMessage, error)
// TypedHandler is the recovered text/structured convenience contract.
type TypedHandler func(context.Context, json.RawMessage) (ToolResult, error)
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
	Validate    Validator
	Handle      TypedHandler
	RawHandle   Handler
}
type Options struct {
	Info  Implementation
	Tools []Tool
	MaxInFlight int
}
type Server struct {
	info  Implementation
	tools []Tool
	maxInFlight int
}

func New(options Options) (*Server, error) {
	if !utf8.ValidString(options.Info.Name) || !utf8.ValidString(options.Info.Version) || len(options.Info.Name)>128 || len(options.Info.Version)>128 || len(options.Tools)>64 { return nil, ErrConfiguration }
	if options.Info.Name==""{options.Info.Name="mcpwire"}
	if options.Info.Version==""{options.Info.Version="native"}
	if options.MaxInFlight < 0 || options.MaxInFlight > 32 { return nil, ErrConfiguration }
	if options.MaxInFlight == 0 { options.MaxInFlight = 4 }
	server := &Server{info: options.Info, maxInFlight: options.MaxInFlight}
	seen := make(map[string]bool)
	for _, tool := range options.Tools {
		if !validName(tool.Name) || !utf8.ValidString(tool.Description) || len(tool.Description)>4096 || tool.Validate == nil || (tool.Handle == nil) == (tool.RawHandle == nil) || !validSchema(tool.InputSchema, 0) || !objectSchemaRoot(tool.InputSchema) || seen[tool.Name] {
			return nil, ErrConfiguration
		}
		seen[tool.Name] = true
		if !tool.ReadOnly {
			continue
		}
		tool.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
		server.tools = append(server.tools, tool)
	}
	sort.Slice(server.tools, func(i, j int) bool { return server.tools[i].Name < server.tools[j].Name })
	listing:=map[string]any{"jsonrpc":"2.0","result":server.modernResult(map[string]any{"tools":server.toolList()})}
	frame,err:=marshalResponse(listing,json.RawMessage(strings.Repeat("9",MaxIDBytes)))
	if err!=nil||len(frame)>MaxFrameBytes{return nil,ErrConfiguration}
	return server, nil
}

func (server *Server) toolList() []any {
	tools := make([]any, 0, len(server.tools))
	for _, tool := range server.tools {
		tools = append(tools, map[string]any{"name": tool.Name, "description": tool.Description, "inputSchema": tool.InputSchema,
			"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false}})
	}
	return tools
}
