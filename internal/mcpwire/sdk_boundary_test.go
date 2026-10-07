package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
