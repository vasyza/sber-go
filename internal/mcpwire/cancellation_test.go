package mcpwire

import (
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
