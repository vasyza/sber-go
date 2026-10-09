package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/vasyza/sber-sdk/internal/mcptools"
)

func TestMixedRequestResponseEnvelopesNeverDispatch(t *testing.T) {
	for _, extra := range []string{`"result":{}`, `"error":{"code":-32603,"message":"synthetic"}`} {
		var calls atomic.Int32
		tool := syntheticTool("local")
		tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
			calls.Add(1)
			return ToolResult{}, nil
		}
		server, err := New(Options{Tools: []Tool{tool}})
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		frame := strings.TrimSuffix(toolCall(`1`, "local", `{}`, true), "}\n") + "," + extra + "}\n"
		if err := server.Serve(context.Background(), strings.NewReader(frame), &output); err == nil || calls.Load() != 0 || output.Len() != 0 {
			t.Fatal("mixed request/response must terminate before dispatch")
		}
	}
}

func TestModernDiscoveryCannotAuthorizeCallsWithoutMetadata(t *testing.T) {
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		return ToolResult{}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	replies := serveText(t, server, discoverFrame(`1`)+"\n"+toolCall(`2`, "local", `{}`, false))
	requireCode(t, replies[1], -32602)
	if calls.Load() != 0 {
		t.Fatal("request without metadata inherited modern discovery state")
	}
}

func TestLegacyInitializeRequiresStringVersion(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"", `"protocolVersion":null,`, `"protocolVersion":42,`} {
		frame := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{` + version + `"capabilities":{},"clientInfo":{"name":"synthetic","version":"test"}}}` + "\n"
		replies := serveText(t, server, frame)
		requireCode(t, replies[0], -32602)
	}
}

func TestFailedInitializeCannotAuthorizeLegacyCalls(t *testing.T) {
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		return ToolResult{}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	replies := serveText(t, server, discoverFrame(`1`)+"\n"+legacyInitialize+toolCall(`2`, "local", `{}`, false))
	requireCode(t, replies[1], -32603)
	requireCode(t, replies[2], -32602)
	if calls.Load() != 0 {
		t.Fatal("failed initialization opened the legacy gate")
	}
}

func TestSDKIncompatibleSchemaFailsAtConstruction(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","default":3e1000}`,
		`{"type":"object","enum":[3e1000]}`,
	} {
		tool := syntheticTool("local")
		tool.InputSchema = json.RawMessage(schema)
		if _, err := New(Options{Tools: []Tool{tool}}); err != ErrConfiguration {
			t.Fatal("SDK-incompatible schema must fail statically before Serve can panic")
		}
	}
}

func TestBlockedOutputRetainsToolAdmissionBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, inputWriter := io.Pipe()
	defer input.Close()
	defer inputWriter.Close()
	outputReader, outputWriter := io.Pipe()
	defer outputReader.Close()
	output := &ownedOutput{PipeWriter: outputWriter, writing: make(chan struct{})}
	var calls atomic.Int32
	oversubscribed := make(chan struct{})
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		if calls.Add(1) == 2 {
			close(oversubscribed)
		}
		return ToolResult{}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, output) }()
	if _, err := io.WriteString(inputWriter, toolCall(`1`, "local", `{}`, true)); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, output.writing, "first blocked result")
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		for i := 2; i < 34; i++ {
			id, _ := json.Marshal(i)
			if _, err := io.WriteString(inputWriter, toolCall(string(id), "local", `{}`, true)); err != nil {
				return
			}
		}
	}()
	select {
	case <-oversubscribed:
		cancel()
		<-done
		t.Fatal("completed callbacks released admission before their blocked results")
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != ErrCancelled {
			t.Fatalf("want cancellation; got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked output did not shut down")
	}
	waitSignal(t, senderDone, "backpressured input cleanup")
	if calls.Load() != 1 {
		t.Fatal("blocked output exceeded the tool budget")
	}
}

func TestBlockedOutputRetainsRequestID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, inputWriter := io.Pipe()
	defer input.Close()
	defer inputWriter.Close()
	outputReader, outputWriter := io.Pipe()
	defer outputReader.Close()
	output := &ownedOutput{PipeWriter: outputWriter, writing: make(chan struct{})}
	var calls atomic.Int32
	duplicate := make(chan struct{})
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		if calls.Add(1) == 2 {
			close(duplicate)
		}
		return ToolResult{}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}, MaxInFlight: 4})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, output) }()
	if _, err := io.WriteString(inputWriter, toolCall(`1`, "local", `{}`, true)); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, output.writing, "first blocked result")
	if _, err := io.WriteString(inputWriter, toolCall(`1`, "local", `{}`, true)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-duplicate:
		cancel()
		<-done
		t.Fatal("request ID was reused before its response was emitted")
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if err != ErrCancelled {
			t.Fatalf("want cancellation; got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked duplicate did not shut down")
	}
	if calls.Load() != 1 {
		t.Fatal("blocked duplicate request dispatched twice")
	}
}

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
