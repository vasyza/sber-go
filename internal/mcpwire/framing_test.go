package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestProtocolIDsRejectInvalidAndInexactNumbers(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{`null`, `true`, `[]`, `{}`, `1.1`, `1e3`, `-0.0`, `9007199254740993`, strings.Repeat("9", 257), strconvQuote(strings.Repeat("x", 257))} {
		t.Run(id[:min(len(id), 20)], func(t *testing.T) {
			var output bytes.Buffer
			frame := `{"jsonrpc":"2.0","id":` + id + `,"method":"server/discover","params":{` + currentMeta + `}}` + "\n"
			if err := server.Serve(context.Background(), strings.NewReader(frame), &output); err != ErrInput || output.Len() != 0 {
				t.Fatal("invalid or inexact IDs must fail before SDK decoding and dispatch")
			}
		})
	}
}

func TestProtocolIDsPreserveSemanticCorrelation(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{`9007199254740991`, `-9007199254740991`, `-0`, `"\u0061"`, `"🚀"`, `""`, `"<>&"`, `"line\u2028separator"`} {
		replies := serveText(t, server, discoverFrame(id)+"\n")
		var want, got any
		decoder := json.NewDecoder(strings.NewReader(id))
		decoder.UseNumber()
		if err := decoder.Decode(&want); err != nil {
			t.Fatal(err)
		}
		decoder = json.NewDecoder(bytes.NewReader(replies[0]["id"]))
		decoder.UseNumber()
		if err := decoder.Decode(&got); err != nil {
			t.Fatal(err)
		}
		if id == "-0" {
			want = json.Number("0")
		}
		if want != got {
			t.Fatalf("want semantic ID %v, got %v", want, got)
		}
		resultFields(t, replies[0])
	}
}
