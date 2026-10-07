package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

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
