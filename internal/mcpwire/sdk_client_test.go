package mcpwire

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vasyza/sber-go/internal/mcptools"
)

func connectSDKClient(t *testing.T, server *Server, version string) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	input, clientWriter := io.Pipe()
	clientReader, output := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, output) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-client", Version: "test"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}})
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: clientReader, Writer: clientWriter}, &mcp.ClientSessionOptions{ProtocolVersion: version})
	if err != nil {
		cancel()
		clientReader.Close()
		clientWriter.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		session.Close()
		finishServe(t, done)
		cancel()
	})
	return ctx, session
}

func TestOfficialSDKClientPreservesCatalogAndExactArguments(t *testing.T) {
	if !slices.Contains(mcp.SupportedProtocolVersions(), CurrentVersion) {
		t.Fatal("pinned SDK must support the requested protocol revision")
	}
	for _, version := range []string{CurrentVersion, LegacyVersion} {
		t.Run(version, func(t *testing.T) {
			var calls atomic.Int32
			var tools []Tool
			// Register synthetic handlers against the actual source catalog. No
			// bank clients, owner profiles, or credentials are used by this test.
			for _, definition := range mcptools.Catalog(false) {
				tools = append(tools, Tool{
					Name: definition.Name, Description: definition.Description,
					InputSchema: definition.InputSchema, ReadOnly: !definition.Mutation,
					Validate: func(_ context.Context, raw json.RawMessage) error {
						return mcptools.ValidateArguments(definition.Name, raw, false)
					},
					Handle: func(_ context.Context, raw json.RawMessage) (ToolResult, error) {
						calls.Add(1)
						// The SDK client decodes structured numbers into float64;
						// echo exact arguments as text in both result forms.
						structured, _ := json.Marshal(map[string]string{"originalArguments": string(raw)})
						return ToolResult{Content: []TextContent{{Text: string(raw)}}, StructuredContent: structured}, nil
					},
				})
			}
			server, err := New(Options{Info: Implementation{Name: "synthetic-sber", Version: "test"}, Tools: tools})
			if err != nil {
				t.Fatal(err)
			}
			ctx, session := connectSDKClient(t, server, version)
			if session.InitializeResult().ProtocolVersion != version {
				t.Fatal("client negotiated the wrong revision")
			}
			listed, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(listed.Tools) != 9 {
				t.Fatalf("want nine default tools; got %d", len(listed.Tools))
			}
			for _, tool := range listed.Tools {
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
					t.Fatal("read-only annotations were lost")
				}
			}
			if version == CurrentVersion && (listed.TTLMs != 0 || listed.CacheScope != "private") {
				t.Fatal("current client lost private cache policy")
			}
			const arguments = `{"limit":9007199254740993,"max_pages":3e1000}`
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "sber_operations", Arguments: json.RawMessage(arguments)})
			if err != nil || result.IsError || len(result.Content) != 1 {
				t.Fatalf("valid exact-number call failed: %v", err)
			}
			if text, ok := result.Content[0].(*mcp.TextContent); !ok || text.Text != arguments {
				t.Fatal("SDK path rewrote original exact-number arguments")
			}
			if structured, ok := result.StructuredContent.(map[string]any); !ok || structured["originalArguments"] != arguments {
				t.Fatal("SDK client lost exact structured text")
			}
			for _, params := range []*mcp.CallToolParams{
				{Name: "sber_transfer_start", Arguments: map[string]any{}},
				{Name: "sber_operations", Arguments: map[string]any{"LIMIT": 1}},
				{Name: "sber_operations", Arguments: json.RawMessage(`{"limit":1.01}`)},
				{Name: "sber_operations", Arguments: json.RawMessage(`null`)},
			} {
				_, err := session.CallTool(ctx, params)
				var failure *jsonrpc.Error
				if !errors.As(err, &failure) || failure.Code != jsonrpc.CodeInvalidParams || failure.Message != "Invalid params" {
					t.Fatalf("want static invalid-params error; got %v", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatal("invalid or default-disabled calls reached a handler")
			}
		})
	}
}
