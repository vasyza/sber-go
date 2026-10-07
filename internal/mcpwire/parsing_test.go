package mcpwire

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictRequestParsing(t *testing.T) {
	cases := []struct {
		name, frame string
		code        int
	}{
		{"escaped-duplicate", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"local","arguments":{"x":1,"\u0078":2},` + currentMeta + `}}`, -32700},
		{"invalid-native-utf8", "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"" + string([]byte{0xff}) + "\"}", -32700},
		{"unpaired-surrogate", `{"jsonrpc":"2.0","id":1,"method":"\ud800"}`, -32700},
		{"trailing-document", `{"jsonrpc":"2.0","id":1,"method":"server/discover"} {}`, -32700},
		{"array-not-batch", `[{"jsonrpc":"2.0","id":1,"method":"server/discover"}]`, -32600},
		{"wrong-jsonrpc", `{"jsonrpc":"1.0","id":1,"method":"server/discover"}`, -32600},
		{"case-alias-envelope", `{"jsonrpc":"2.0","id":1,"METHOD":"server/discover"}`, -32600},
		{"missing-method", `{"jsonrpc":"2.0","id":1}`, -32600},
		{"params-array", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":[]}`, -32602},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, err := New(Options{Tools: []Tool{syntheticTool("local")}})
			if err != nil {
				t.Fatal(err)
			}
			valid := `{"jsonrpc":"2.0","id":"after","method":"server/discover","params":{` + currentMeta + `}}` + "\n"
			if tc.name == "params-array" {
				responses := serveText(t, server, tc.frame+"\n"+valid)
				requireCode(t, responses[0], tc.code)
				resultFields(t, responses[1])
			} else {
				var output bytes.Buffer
				if err := server.Serve(context.Background(), strings.NewReader(tc.frame+"\n"+valid), &output); err == nil || output.Len() != 0 {
					t.Fatal("malformed framing or envelope must terminate without dispatch or reflection")
				}
			}
		})
	}
}

func TestMalformedClientResponsesStillTerminateWithoutReply(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, frame := range []string{
		`{"jsonrpc":"2.0","id":1,"result":{"x":1,"\u0078":2}}`,
		`{"jsonrpc":"2.0","id":1,"error":{"message":"\ud800"}}`,
	} {
		var out bytes.Buffer
		if err := server.Serve(context.Background(), strings.NewReader(frame+"\n"), &out); err == nil || out.Len() != 0 {
			t.Fatal("want rejected client response, not reply")
		}
	}
}

func TestClientResponseTerminatesWithoutReply(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = server.Serve(context.Background(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{}}`+"\n"), &out)
	if err == nil || out.Len() != 0 {
		t.Fatalf("want client response rejected without reply; got %v %q", err, out.String())
	}
}

func TestNotificationsCannotInvokeToolsOrInitialize(t *testing.T) {
	calls := 0
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls++; return ToolResult{}, nil }
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	in := `{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"local","version":"test"}}}` + "\n" + legacyInitialized + `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"local","arguments":{},` + currentMeta + `}}` + "\n" + `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	responses := serveText(t, server, in)
	if len(responses) != 1 || calls != 0 {
		t.Fatalf("want silent non-dispatching notifications; got %d/%d", len(responses), calls)
	}
	requireCode(t, responses[0], -32602)
}
