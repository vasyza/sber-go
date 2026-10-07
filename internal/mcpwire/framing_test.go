package mcpwire

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

const frameLimitTest = 1 << 20

func discoverFrame(id string) string {
	return `{"jsonrpc":"2.0","id":` + id + `,"method":"server/discover","params":{` + currentMeta + `}}`
}

type fragmentReader struct{ source io.Reader }

func (r fragmentReader) Read(p []byte) (int, error) { return r.source.Read(p[:min(len(p), 7)]) }

func TestFramingAcceptsOneMiBAndFragmentedReads(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	frame := discoverFrame(`"boundary"`)
	frame += strings.Repeat(" ", frameLimitTest-len(frame))
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	out := &frameWriter{frames: make(chan []byte, 8)}
	done := make(chan error, 1)
	go func() { done <- server.Serve(context.Background(), fragmentReader{input}, out) }()
	if _, err := io.WriteString(writer, frame+"\n"); err != nil {
		t.Fatal(err)
	}
	resultFields(t, nextFrame(t, out))
	if _, err := io.WriteString(writer, discoverFrame(`"after"`)+"\n"); err != nil {
		t.Fatal(err)
	}
	resultFields(t, nextFrame(t, out))
	writer.Close()
	finishServe(t, done)
}

func TestFramingRejectsUnterminatedAndOversizeInput(t *testing.T) {
	for _, input := range []string{discoverFrame(`1`), strings.Repeat("x", frameLimitTest+1) + "\n", strings.Repeat("x", frameLimitTest+1)} {
		server, err := New(Options{})
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := server.Serve(context.Background(), strings.NewReader(input), &out); err == nil {
			t.Fatal("want static framing failure")
		}
		if out.Len() != 0 {
			t.Fatal("must not dispatch partial/oversize frame")
		}
	}
}

type partialWriter struct{ calls int }

func (w *partialWriter) Write(p []byte) (int, error) { w.calls++; return len(p) - 1, nil }
func TestWriterShortWriteIsTerminal(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	w := &partialWriter{}
	if err := serveUntilWriteFailure(t, server, discoverFrame(`1`)+"\n", w); err == nil {
		t.Fatal("want short write error, not false success")
	}
	if w.calls != 1 {
		t.Fatalf("want one write attempt; got %d", w.calls)
	}
}

type faultReader struct {
	data []byte
	err  error
}

func (r *faultReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}
func TestInputFaultDoesNotBecomeEOFSuccess(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = server.Serve(context.Background(), &faultReader{data: []byte(discoverFrame(`1`)), err: errors.New("SYNTHETIC-PRIVATE-IO")}, &out)
	if err == nil || strings.Contains(err.Error(), "SYNTHETIC-PRIVATE-IO") || out.Len() != 0 {
		t.Fatal("want private I/O error without partial dispatch")
	}
}
