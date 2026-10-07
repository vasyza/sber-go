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
		err = server.Serve(context.Background(), strings.NewReader(toolCall(`1`, "local", `{}`, true)), panicWriter{})
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
	err = server.Serve(context.Background(), strings.NewReader(toolCall(`1`, "local", `{}`, true)), out)
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
	reply := nextFrame(t, out)
	if string(resultFields(t, reply)["isError"]) != "true" {
		t.Fatal("context error must not become successful zero result")
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
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, strings.NewReader(discoverFrame(`1`)+"\n"), out) }()
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
