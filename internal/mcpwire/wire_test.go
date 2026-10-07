package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

const currentMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

func serveText(t *testing.T, server *Server, text string) []map[string]json.RawMessage {
	t.Helper()
	// A real MCP client keeps its input open until responses arrive. EOF is a
	// transport shutdown, and the official SDK cancels unfinished requests.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, writer := io.Pipe()
	defer writer.Close()
	defer input.Close()
	out := &frameWriter{frames: make(chan []byte, 64)}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, out) }()
	var responses []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if len(line) == 0 {
			continue
		}
		if _, err := io.WriteString(writer, line+"\n"); err != nil {
			t.Fatal(err)
		}
		var request map[string]json.RawMessage
		_ = json.Unmarshal([]byte(line), &request)
		if _, hasID := request["id"]; hasID {
			select {
			case raw := <-out.frames:
				var response map[string]json.RawMessage
				if err := json.Unmarshal(raw, &response); err != nil {
					t.Fatal(err)
				}
				responses = append(responses, response)
			case err := <-done:
				t.Fatalf("transport ended before reply: %v", err)
			case <-ctx.Done():
				t.Fatal("timed out waiting for reply")
			}
		}
	}
	// Barrier ensures preceding notifications are processed before closing.
	if _, err := io.WriteString(writer, discoverFrame(`"test-barrier"`)+"\n"); err != nil {
		t.Fatal(err)
	}
	barrier := nextFrame(t, out)
	if string(barrier["id"]) != `"test-barrier"` {
		t.Fatal("unexpected notification response")
	}
	writer.Close()
	finishServe(t, done)
	return responses
}

func requireCode(t *testing.T, response map[string]json.RawMessage, code int) {
	t.Helper()
	var failure struct {
		Code    int
		Message string
		Data    json.RawMessage
	}
	if err := json.Unmarshal(response["error"], &failure); err != nil {
		t.Fatalf("want error %d; got %s", code, response["result"])
	}
	if failure.Code != code {
		t.Fatalf("want code %d; got %d", code, failure.Code)
	}
}

func TestCurrentMetadataRequired(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`+"\n")
	if len(responses) != 1 {
		t.Fatalf("want one response; got %d", len(responses))
	}
	requireCode(t, responses[0], -32602)
}

func TestCurrentDiscover(t *testing.T) {
	server, err := New(Options{Info: Implementation{Name: "synthetic", Version: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	in := `{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{` + currentMeta + `}}` + "\n"
	response := serveText(t, server, in)[0]
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response["result"], &result); err != nil {
		t.Fatal(err)
	}
	if string(result["resultType"]) != `"complete"` {
		t.Fatalf("want complete result; got %s", response["result"])
	}
	if string(result["supportedVersions"]) != `["2026-07-28","2025-11-25"]` {
		t.Fatalf("want explicit versions; got %s", response["result"])
	}
	if string(result["capabilities"]) != `{"tools":{}}` {
		t.Fatalf("want only tools capability; got %s", response["result"])
	}
	if _, present := result["serverInfo"]; present {
		t.Fatal("serverInfo must be in result._meta")
	}
	if !bytes.Contains(result["_meta"], []byte(`"synthetic"`)) {
		t.Fatalf("want server metadata; got %s", response["result"])
	}
}
