package mcpwire

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sort"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type TextContent struct{ Text string }
type ToolResult struct {
	Content           []TextContent
	StructuredContent json.RawMessage
	IsError           bool
}

// Validator must reject invalid tool arguments before a handler can run.
type Validator func(context.Context, json.RawMessage) error

// Handler returns a strict tool-result object, not a JSON-RPC envelope.
type Handler func(context.Context, json.RawMessage) (json.RawMessage, error)
type TypedHandler func(context.Context, json.RawMessage) (ToolResult, error)
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	ReadOnly    bool
	// LocalControl permits a local session-lifecycle operation. Only trusted
	// application registration sets this; tool arguments never enable writes.
	LocalControl bool
	Validate     Validator
	Handle       TypedHandler
	RawHandle    Handler
}
type Options struct {
	Info        Implementation
	Tools       []Tool
	MaxInFlight int
}
type Server struct {
	info        Implementation
	tools       []Tool
	maxInFlight int
}

func New(options Options) (*Server, error) {
	if !utf8.ValidString(options.Info.Name) || !utf8.ValidString(options.Info.Version) || len(options.Info.Name) > 128 || len(options.Info.Version) > 128 || len(options.Tools) > 64 {
		return nil, ErrConfiguration
	}
	if options.Info.Name == "" {
		options.Info.Name = "mcpwire"
	}
	if options.Info.Version == "" {
		options.Info.Version = "native"
	}
	if options.MaxInFlight < 0 || options.MaxInFlight > 32 {
		return nil, ErrConfiguration
	}
	if options.MaxInFlight == 0 {
		options.MaxInFlight = 4
	}
	server := &Server{info: options.Info, maxInFlight: options.MaxInFlight}
	seen := make(map[string]bool)
	for _, tool := range options.Tools {
		if !validName(tool.Name) || !utf8.ValidString(tool.Description) || len(tool.Description) > 4096 || tool.Validate == nil || (tool.Handle == nil) == (tool.RawHandle == nil) || !validSchema(tool.InputSchema, 0) || !objectSchemaRoot(tool.InputSchema) || seen[tool.Name] {
			return nil, ErrConfiguration
		}
		seen[tool.Name] = true
		if !tool.ReadOnly && !tool.LocalControl {
			continue
		}
		tool.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
		server.tools = append(server.tools, tool)
	}
	sort.Slice(server.tools, func(i, j int) bool { return server.tools[i].Name < server.tools[j].Name })
	// Reserve space for the SDK's result metadata and a bounded correlation ID.
	listing, err := json.Marshal(server.toolList())
	if err != nil || len(listing) > MaxFrameBytes-4096 {
		return nil, ErrConfiguration
	}
	// Preflight the actual SDK registration, which may reject schema metadata
	// (for example an overflowing numeric default) that is otherwise strict JSON.
	if _, err := server.sdkServer(); err != nil {
		return nil, ErrConfiguration
	}
	return server, nil
}

func (server *Server) toolList() []*mcp.Tool {
	tools := make([]*mcp.Tool, 0, len(server.tools))
	for _, tool := range server.tools {
		tools = append(tools, &mcp.Tool{
			Name: tool.Name, Description: tool.Description,
			InputSchema: append(json.RawMessage(nil), tool.InputSchema...),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tool.ReadOnly, DestructiveHint: boolPtr(false)},
		})
	}
	return tools
}

func boolPtr(value bool) *bool { return &value }

// Each Serve call owns a fresh SDK server and independent concurrency budget.
// The low-level AddTool API retains original argument bytes and exact numbers;
// the application's concrete validators remain responsible for semantics.
func (server *Server) sdkServer() (sdk *mcp.Server, err error) {
	defer func() {
		if recover() != nil {
			sdk, err = nil, ErrConfiguration
		}
	}()
	sdk = mcp.NewServer(&mcp.Implementation{Name: server.info.Name, Version: server.info.Version}, &mcp.ServerOptions{
		Logger:                    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Capabilities:              &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		SupportedProtocolVersions: []string{CurrentVersion, LegacyVersion},
		PageSize:                  64,
		SetCacheable: func(_ context.Context, _ mcp.Request, cache *mcp.Cacheable) {
			cache.TTLMs, cache.CacheScope = 0, "private"
		},
	})
	sdk.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "server/discover", "tools/list", "tools/call", "initialize", "ping", "notifications/initialized", "notifications/cancelled":
				return next(ctx, method, req)
			default:
				return nil, protocolError(-32601)
			}
		}
	})
	for i, descriptor := range server.toolList() {
		tool := server.tools[i]
		sdk.AddTool(descriptor, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return callTool(ctx, tool, req.Params.Arguments)
		})
	}
	return sdk, nil
}
