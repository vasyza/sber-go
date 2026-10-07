package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const currentMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

func serveText(t *testing.T, server *Server, text string) []map[string]json.RawMessage {
	t.Helper()
	var out bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(text), &out); err != nil {
		t.Fatal(err)
	}
	var responses []map[string]json.RawMessage
	for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var response map[string]json.RawMessage
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("invalid output %q: %v", line, err)
		}
		responses = append(responses, response)
	}
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
	var out bytes.Buffer
	in := `{"jsonrpc":"2.0","id":"discover","method":"server/discover","params":{` + currentMeta + `}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var response map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(response["result"], &result); err != nil {
		t.Fatal(err)
	}
	if string(result["resultType"]) != `"complete"` {
		t.Fatalf("want complete result; got %s", out.String())
	}
	if string(result["supportedVersions"]) != `["2026-07-28","2025-11-25"]` {
		t.Fatalf("want explicit versions; got %s", out.String())
	}
	if string(result["capabilities"]) != `{"tools":{}}` {
		t.Fatalf("want only tools capability; got %s", out.String())
	}
	if _, present := result["serverInfo"]; present {
		t.Fatal("serverInfo must be in result._meta")
	}
	if !bytes.Contains(result["_meta"], []byte(`"synthetic"`)) {
		t.Fatalf("want server metadata; got %s", out.String())
	}
}
