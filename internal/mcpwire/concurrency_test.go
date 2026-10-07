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

type frameWriter struct{ frames chan []byte }

func (w *frameWriter) Write(p []byte) (int, error) {
	b := append([]byte(nil), p...)
	w.frames <- b
	return len(p), nil
}

func waitSignal(t *testing.T, c <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(time.Second):
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
	case <-time.After(time.Second):
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
	case <-time.After(time.Second):
		t.Fatal("want prompt Serve exit")
	}
}

func TestActiveIDCollisionsRejectButCompletedIDsCanBeReused(t *testing.T) {
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
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
		return ToolResult{Content: []TextContent{{Text: "synthetic"}}}, nil
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
	io.WriteString(writer, toolCall(`"\u0061"`, "local", `{}`, true))
	waitSignal(t, entered, "handler entry")
	io.WriteString(writer, discoverFrame(`"a"`)+"\n")
	reply := nextFrame(t, out)
	requireCode(t, reply, -32600)
	close(release)
	reply = nextFrame(t, out)
	if string(reply["id"]) != `"\u0061"` {
		t.Fatal("want exact original ID bytes")
	}
	for i := 0; i < 8; i++ {
		io.WriteString(writer, toolCall(`"a"`, "local", `{}`, true))
		reply = nextFrame(t, out)
		resultFields(t, reply)
	}
	writer.Close()
	finishServe(t, done)
	finished = true
	if calls.Load() != 9 {
		t.Fatal("want no collision dispatch and no forever-seen ID set")
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
