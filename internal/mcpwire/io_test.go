package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type panicWriter struct{}

func (panicWriter) Write([]byte) (int, error) { panic("SYNTHETIC-PRIVATE-WRITER-PANIC") }

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("SYNTHETIC-PRIVATE-READER-PANIC") }

// Keep input open until the failed output terminates the SDK session.
func serveUntilWriteFailure(t *testing.T, server *Server, frame string, output io.Writer) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, output) }()
	if _, err := io.WriteString(writer, frame); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatal("output failure did not terminate")
		return nil
	}
}

func TestWriterPanicIsStaticTerminalAndNeverRetriesHandler(t *testing.T) {
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls.Add(1); return ToolResult{}, nil }
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		err = serveUntilWriteFailure(t, server, toolCall(`1`, "local", `{}`, true), panicWriter{})
	}()
	if panicked || err != ErrOutput || calls.Load() != 1 {
		t.Fatal("want static writer failure and one handler attempt")
	}
}
func TestReaderPanicIsStaticTerminal(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = server.Serve(context.Background(), panicReader{}, &out); err != ErrInput || out.Len() != 0 {
		t.Fatal("want static reader failure without output")
	}
}

type failingWriter struct{ calls atomic.Int32 }

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls.Add(1)
	return len(p), errors.New("SYNTHETIC-PRIVATE-WRITER-ERROR")
}
func TestFullWriteWithErrorStillFailsWithoutRetry(t *testing.T) {
	var calls atomic.Int32
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls.Add(1); return ToolResult{}, nil }
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	out := &failingWriter{}
	err = serveUntilWriteFailure(t, server, toolCall(`1`, "local", `{}`, true), out)
	if err != ErrOutput || calls.Load() != 1 || out.calls.Load() != 1 {
		t.Fatal("want full+error terminal, no handler/write retry")
	}
}

type closeFaultOutput struct {
	bytes.Buffer
	closed atomic.Int32
}

func (w *closeFaultOutput) Close() error {
	w.closed.Add(1)
	return errors.New("SYNTHETIC-PRIVATE-CLOSE-ERROR")
}
func TestOutputCloseErrorIsNotFalseSuccess(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	out := &closeFaultOutput{}
	err = server.Serve(context.Background(), strings.NewReader(discoverFrame(`1`)+"\n"), out)
	if err != ErrOutput || out.closed.Load() != 1 {
		t.Fatal("want exact once close with static output failure")
	}
}

type ownedReader struct {
	*io.PipeReader
	reading  chan struct{}
	readOnce atomic.Bool
	closed   atomic.Int32
}

func (r *ownedReader) Read(p []byte) (int, error) {
	if r.readOnce.CompareAndSwap(false, true) {
		close(r.reading)
	}
	return r.PipeReader.Read(p)
}
func (r *ownedReader) Close() error { r.closed.Add(1); return r.PipeReader.Close() }
func TestContextCancellationClosesOwnedBlockedReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pipe, writer := io.Pipe()
	defer writer.Close()
	input := &ownedReader{PipeReader: pipe, reading: make(chan struct{})}
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { var out bytes.Buffer; done <- server.Serve(ctx, input, &out) }()
	waitSignal(t, input.reading, "blocking read")
	cancel()
	select {
	case err := <-done:
		if err == nil || strings.Contains(err.Error(), "SYNTHETIC-PRIVATE") {
			t.Fatal("want static cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not unblock owned reader")
	}
	if input.closed.Load() != 1 {
		t.Fatalf("want exact once close; got %d", input.closed.Load())
	}
}

func TestEOFPromptlyCancelsAndJoinsActiveHandler(t *testing.T) {
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
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
	out := &frameWriter{frames: make(chan []byte, 8)}
	go func() { done <- server.Serve(context.Background(), input, out) }()
	io.WriteString(writer, toolCall(`1`, "local", `{}`, true))
	waitSignal(t, entered, "handler entry")
	writer.Close()
	finishServe(t, done)
	waitSignal(t, exited, "handler cleanup")
	if calls.Load() != 1 {
		t.Fatal("must not replay handler on EOF")
	}
	if len(out.frames) != 0 {
		t.Fatal("EOF must not emit a successful unfinished tool result")
	}
}

type ownedOutput struct {
	*io.PipeWriter
	writing   chan struct{}
	writeOnce atomic.Bool
	closed    atomic.Int32
}

func (w *ownedOutput) Write(p []byte) (int, error) {
	if w.writeOnce.CompareAndSwap(false, true) {
		close(w.writing)
	}
	return w.PipeWriter.Write(p)
}
func (w *ownedOutput) Close() error { w.closed.Add(1); return w.PipeWriter.Close() }
func TestContextCancellationUnblocksOwnedWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	out := &ownedOutput{PipeWriter: writer, writing: make(chan struct{})}
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	input, inputWriter := io.Pipe()
	defer input.Close()
	defer inputWriter.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	if _, err := io.WriteString(inputWriter, discoverFrame(`1`)+"\n"); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, out.writing, "blocking write")
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want terminal writer cancellation error")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not close writer")
	}
	if out.closed.Load() != 1 {
		t.Fatal("want exact once output close")
	}
}

func TestRawOutputRejectsOriginalUnicodeDuplicatesAndResultShape(t *testing.T) {
	outputs := []string{
		`{"content":[],"structuredContent":{"x":1,"\u0078":2}}`,
		`{"content":[{"type":"text","text":"\ud800"}]}`,
		"{\"content\":[{\"type\":\"text\",\"text\":\"" + string([]byte{0xff}) + "\"}]}",
		`{"content":[]} {}`, `null`, `[]`, `{}`, `{"Content":[]}`, `{"content":null}`,
		`{"content":[{"type":"text","Text":"private"}]}`,
		`{"content":[],"structuredContent":[]}`,
		`{"content":[],"isError":"false"}`,
		`{"content":[],"resultType":"input_required"}`,
	}
	for i, raw := range outputs {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			calls := 0
			tool := syntheticTool("raw")
			tool.Handle = nil
			tool.RawHandle = func(context.Context, json.RawMessage) (json.RawMessage, error) {
				calls++
				return json.RawMessage(raw), nil
			}
			server, err := New(Options{Tools: []Tool{tool}})
			if err != nil {
				t.Fatal(err)
			}
			reply := serveText(t, server, toolCall(`1`, "raw", `{}`, true))[0]
			requireCode(t, reply, -32603)
			if calls != 1 {
				t.Fatalf("want exactly one call; got %d", calls)
			}
		})
	}
}

func TestNativeTypedOutputCannotReplaceInvalidUTF8(t *testing.T) {
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
		return ToolResult{Content: []TextContent{{Text: string([]byte{0xff})}}}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, toolCall(`1`, "local", `{}`, true))
	requireCode(t, responses[0], -32603)
}

func TestCallbackErrorsAndPanicsStayStaticWithoutRetry(t *testing.T) {
	for _, stage := range []string{"validation-error", "validation-panic", "handler-error", "handler-panic"} {
		t.Run(stage, func(t *testing.T) {
			validations, calls := 0, 0
			tool := syntheticTool("local")
			tool.Validate = func(context.Context, json.RawMessage) error {
				validations++
				if stage == "validation-panic" {
					panic("SYNTHETIC-PRIVATE-PANIC")
				}
				if stage == "validation-error" {
					return errors.New("SYNTHETIC-PRIVATE-ERROR")
				}
				return nil
			}
			tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
				calls++
				if stage == "handler-panic" {
					panic("SYNTHETIC-PRIVATE-PANIC")
				}
				return ToolResult{Content: []TextContent{{Text: "SYNTHETIC-PRIVATE-RESULT"}}}, errors.New("SYNTHETIC-PRIVATE-ERROR")
			}
			server, err := New(Options{Tools: []Tool{tool}})
			if err != nil {
				t.Fatal(err)
			}
			reply := serveText(t, server, toolCall(`1`, "local", `{}`, true))[0]
			encoded, _ := json.Marshal(reply)
			if bytes.Contains(encoded, []byte("SYNTHETIC-PRIVATE")) {
				t.Fatal("private callback value leaked")
			}
			if validations != 1 || (strings.HasPrefix(stage, "validation") && calls != 0) || (!strings.HasPrefix(stage, "validation") && calls != 1) {
				t.Fatal("want no retry/no dispatch after failed validation")
			}
			if stage == "validation-error" {
				requireCode(t, reply, -32602)
			} else if strings.HasSuffix(stage, "panic") {
				requireCode(t, reply, -32603)
			} else {
				result := resultFields(t, reply)
				if string(result["isError"]) != "true" || !bytes.Contains(result["content"], []byte("Tool execution failed")) {
					t.Fatal("want sanitized tool execution failure")
				}
			}
		})
	}
}

func TestEncodedOutputExactOneMiBBoundary(t *testing.T) {
	info := Implementation{Name: "synthetic", Version: "test"}
	empty := map[string]any{"jsonrpc": "2.0", "id": 1, "result": map[string]any{"content": []any{map[string]any{"type": "text", "text": ""}}, "resultType": "complete", "_meta": map[string]any{"io.modelcontextprotocol/serverInfo": info}}}
	encoded, err := json.Marshal(empty)
	if err != nil {
		t.Fatal(err)
	}
	for _, delta := range []int{0, 1} {
		t.Run(string(rune('a'+delta)), func(t *testing.T) {
			text := strings.Repeat("x", frameLimitTest-len(encoded)+delta)
			calls := 0
			tool := syntheticTool("local")
			tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) {
				calls++
				return ToolResult{Content: []TextContent{{Text: text}}}, nil
			}
			server, err := New(Options{Info: info, Tools: []Tool{tool}})
			if err != nil {
				t.Fatal(err)
			}
			reply := serveText(t, server, toolCall(`1`, "local", `{}`, true))[0]
			if delta == 0 {
				result := resultFields(t, reply)
				var content []struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(result["content"], &content) != nil || len(content) != 1 || len(content[0].Text) != len(text) {
					t.Fatal("want large output preserved")
				}
			} else {
				requireCode(t, reply, -32603)
			}
			if calls != 1 {
				t.Fatal("must not retry on output rejection")
			}
		})
	}
}
