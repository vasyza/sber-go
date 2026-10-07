package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancellationMatchesTypedIDContextAndSuppressesLateResponses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &frameWriter{frames: make(chan []byte, 16)}
	entered := make(chan context.Context, 2)
	stringCancelled, integerCancelled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"kind":{"type":"string"}},"required":["kind"],"additionalProperties":false}`)
	tool.Handle = func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		entered <- ctx
		<-ctx.Done()
		if string(raw) == `{"kind":"string"}` {
			close(stringCancelled)
		} else {
			close(integerCancelled)
		}
		return ToolResult{Content: []TextContent{{Text: "SYNTHETIC-LATE-RESULT-MUST-BE-SUPPRESSED"}}}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	finished := false
	defer func() {
		cancel()
		input.Close()
		writer.Close()
		if !finished {
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}()
	io.WriteString(writer, toolCall(`"1"`, "local", `{"kind":"string"}`, true))
	var first, second context.Context
	select {
	case first = <-entered:
	case <-time.After(time.Second):
		t.Fatal("want first context")
	}
	io.WriteString(writer, toolCall(`1`, "local", `{"kind":"integer"}`, true))
	select {
	case second = <-entered:
	case <-time.After(time.Second):
		t.Fatal("want second context")
	}
	for _, params := range []string{`"requestId":1.0`, `"RequestId":"1"`, `"requestId":"unknown"`, `"requestId":"1","reason":42`} {
		io.WriteString(writer, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{`+params+`}}`+"\n")
	}
	io.WriteString(writer, discoverFrame(`"barrier"`)+"\n")
	reply := nextFrame(t, out)
	if string(reply["id"]) != `"barrier"` {
		t.Fatal("want ignored invalid notifications")
	}
	select {
	case <-stringCancelled:
		t.Fatal("invalid cancellation affected string request")
	case <-integerCancelled:
		t.Fatal("invalid cancellation affected integer request")
	default:
	}
	io.WriteString(writer, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"\u0031","reason":"SYNTHETIC-PRIVATE-REASON"}}`+"\n")
	waitSignal(t, stringCancelled, "matching request cancellation")
	if first == second || first == ctx || second == ctx {
		t.Fatal("want distinct request contexts")
	}
	select {
	case <-integerCancelled:
		t.Fatal("string ID cancellation matched integer ID")
	default:
	}
	io.WriteString(writer, discoverFrame(`"after"`)+"\n")
	reply = nextFrame(t, out)
	if string(reply["id"]) != `"after"` {
		t.Fatal("cancelled request wrote a late response")
	}
	io.WriteString(writer, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`+"\n")
	waitSignal(t, integerCancelled, "integer request cancellation")
	writer.Close()
	finishServe(t, done)
	finished = true
	if len(out.frames) != 0 {
		t.Fatal("late responses remain after Serve shutdown")
	}
	if calls.Load() != 2 {
		t.Fatal("want one execution per non-colliding request")
	}
}

type frameWriter struct{ frames chan []byte }

func (w *frameWriter) Write(p []byte) (int, error) {
	b := append([]byte(nil), p...)
	w.frames <- b
	return len(p), nil
}

// A peer may see every response byte before the underlying Write returns.
type heldFrameWriter struct {
	frameWriter
	ctx     context.Context
	release chan struct{}
	first   atomic.Bool
}

func (w *heldFrameWriter) Write(p []byte) (int, error) {
	n, err := w.frameWriter.Write(p)
	if w.first.CompareAndSwap(false, true) {
		select {
		case <-w.release:
		case <-w.ctx.Done():
		}
	}
	return n, err
}

func waitSignal(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatal("want prompt " + what)
	}
}
func nextFrame(t *testing.T, w *frameWriter) map[string]json.RawMessage {
	t.Helper()
	select {
	case raw := <-w.frames:
		var fields map[string]json.RawMessage
		if json.Unmarshal(bytes.TrimSpace(raw), &fields) != nil {
			t.Fatal("want single JSON-RPC frame")
		}
		return fields
	case <-time.After(5 * time.Second):
		t.Fatal("want prompt response while another request is blocked")
		return nil
	}
}
func finishServe(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("want prompt Serve exit")
	}
}

func TestDuplicateActiveIDsTerminateWithoutSecondDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &frameWriter{frames: make(chan []byte, 8)}
	entered, exited := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		close(entered)
		<-ctx.Done()
		close(exited)
		return ToolResult{}, ctx.Err()
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	io.WriteString(writer, toolCall(`"\u0061"`, "local", `{}`, true))
	waitSignal(t, entered, "handler entry")
	io.WriteString(writer, discoverFrame(`"a"`)+"\n")
	select {
	case err := <-done:
		if err != ErrInput {
			t.Fatalf("want static duplicate-ID transport failure; got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("duplicate ID did not terminate")
	}
	waitSignal(t, exited, "cancelled handler cleanup")
	if calls.Load() != 1 {
		t.Fatal("duplicate ID dispatched a second handler")
	}
}

func TestCompletedIDsCanBeReused(t *testing.T) {
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls.Add(1); return ToolResult{}, nil }
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	replies := serveText(t, server, toolCall(`"a"`, "local", `{}`, true)+toolCall(`"a"`, "local", `{}`, true))
	if len(replies) != 2 || calls.Load() != 2 {
		t.Fatal("completed ID could not be reused")
	}
	for _, reply := range replies {
		resultFields(t, reply)
	}
}

func TestVisibleResponseAllowsImmediateIDReuseAtCapacity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &heldFrameWriter{frameWriter: frameWriter{frames: make(chan []byte, 8)}, ctx: ctx, release: make(chan struct{})}
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		return ToolResult{}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}, MaxInFlight: 1})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	frame := toolCall(`"repeat"`, "local", `{}`, true)
	if _, err := io.WriteString(writer, frame); err != nil {
		t.Fatal(err)
	}
	resultFields(t, nextFrame(t, &out.frameWriter))
	if _, err := io.WriteString(writer, frame); err != nil {
		t.Fatal(err)
	}
	select {
	case <-out.frames:
		t.Fatal("second response bypassed the blocked first write")
	case <-time.After(100 * time.Millisecond):
	}
	close(out.release)
	resultFields(t, nextFrame(t, &out.frameWriter))
	writer.Close()
	finishServe(t, done)
	if calls.Load() != 2 {
		t.Fatal("visible response prevented legitimate completed-ID reuse")
	}
}

func TestFiniteInFlightRejectsExcessWithoutBlockingControl(t *testing.T) {
	for _, limit := range []int{-1, 33} {
		if _, err := New(Options{MaxInFlight: limit}); err != ErrConfiguration {
			t.Fatal("want static invalid limit error")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &frameWriter{frames: make(chan []byte, 8)}
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
		if calls.Add(1) == 1 {
			close(entered)
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ToolResult{Content: []TextContent{{Text: "synthetic"}}}, nil
	}
	server, err := New(Options{MaxInFlight: 1, Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	finished := false
	defer func() {
		cancel()
		input.Close()
		writer.Close()
		if !finished {
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}()
	io.WriteString(writer, toolCall(`"first"`, "local", `{}`, true))
	waitSignal(t, entered, "handler entry")
	io.WriteString(writer, toolCall(`"excess"`, "local", `{}`, true))
	reply := nextFrame(t, out)
	if string(reply["id"]) != `"excess"` {
		t.Fatal("want bounded capacity rejection")
	}
	requireCode(t, reply, -32603)
	io.WriteString(writer, discoverFrame(`"control"`)+"\n")
	reply = nextFrame(t, out)
	if string(reply["id"]) != `"control"` {
		t.Fatal("controls must remain usable at capacity")
	}
	close(release)
	nextFrame(t, out)
	writer.Close()
	finishServe(t, done)
	finished = true
	if calls.Load() != 1 {
		t.Fatal("capacity rejection invoked excess handler")
	}
}

func TestBlockedToolDoesNotBlockDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &frameWriter{frames: make(chan []byte, 8)}
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ToolResult{Content: []TextContent{{Text: "synthetic completion"}}}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	finished := false
	defer func() {
		cancel()
		input.Close()
		writer.Close()
		if !finished {
			select {
			case <-done:
			case <-time.After(time.Second):
			}
		}
	}()
	if _, err := io.WriteString(writer, toolCall(`"slow"`, "local", `{}`, true)); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, entered, "handler entry")
	wrote := make(chan struct{})
	go func() { io.WriteString(writer, discoverFrame(`"quick"`)+"\n"); close(wrote) }()
	reply := nextFrame(t, out)
	if string(reply["id"]) != `"quick"` {
		t.Fatal("want discovery response before blocked tool")
	}
	if string(resultFields(t, reply)["resultType"]) != `"complete"` {
		t.Fatal("want complete discovery")
	}
	waitSignal(t, wrote, "discovery read")
	close(release)
	reply = nextFrame(t, out)
	if string(reply["id"]) != `"slow"` {
		t.Fatal("want correlated tool reply")
	}
	writer.Close()
	finishServe(t, done)
	finished = true
	if calls.Load() != 1 {
		t.Fatal("want exactly one execution")
	}
}

func TestInjectedRawHandlerUsesValidatedArgumentCopies(t *testing.T) {
	var validations, calls atomic.Int32
	const arguments = `{"synthetic":"native"}`
	tool := syntheticTool("raw")
	tool.Handle = nil
	tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"synthetic":{"type":"string"}},"required":["synthetic"],"additionalProperties":false}`)
	tool.Validate = func(ctx context.Context, raw json.RawMessage) error {
		validations.Add(1)
		if !bytes.Equal(raw, []byte(arguments)) {
			t.Error("validator did not get original bytes")
		}
		raw[2] = 'x' // This must not alter the bytes passed to the handler.
		return nil
	}
	tool.RawHandle = func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		calls.Add(1)
		if string(raw) != arguments {
			t.Error("handler did not get independent bytes")
		}
		return json.RawMessage(`{"content":[{"type":"text","text":"synthetic native result"}],"structuredContent":{"n":9007199254740993}}`), nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, toolCall(`"raw-call"`, "raw", arguments, true))
	if len(responses) != 1 {
		t.Fatalf("want one response; got %d", len(responses))
	}
	result := resultFields(t, responses[0])
	if string(result["resultType"]) != `"complete"` || string(result["structuredContent"]) != `{"n":9007199254740993}` {
		t.Fatal("want exact typed raw JSON result")
	}
	if validations.Load() != 1 || calls.Load() != 1 {
		t.Fatal("want exactly one validation and execution")
	}
}
