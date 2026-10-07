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

func TestDiscoveryAndListCacheContract(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"server/discover", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			responses := serveText(t, server, `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{`+currentMeta+`}}`+"\n")
			result := resultFields(t, responses[0])
			if string(result["ttlMs"]) != "0" || string(result["cacheScope"]) != `"private"` {
				t.Fatalf("missing conservative cache policy: %s", responses[0]["result"])
			}
		})
	}
}

func TestUnsupportedVersionIncludesNegotiationData(t *testing.T) {
	server, _ := New(Options{})
	meta := strings.Replace(currentMeta, CurrentVersion, "1900-01-01", 1)
	responses := serveText(t, server, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+meta+`}}`+"\n")
	var failure struct {
		Code int
		Data struct {
			Supported []string
			Requested string
		}
	}
	if err := json.Unmarshal(responses[0]["error"], &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Code != -32022 || failure.Data.Requested != "1900-01-01" || len(failure.Data.Supported) != 2 {
		t.Fatalf("missing version negotiation data: %s", responses[0]["error"])
	}
}

func TestLegacyNegotiatesSupportedCounteroffer(t *testing.T) {
	server, _ := New(Options{})
	responses := serveText(t, server, strings.Replace(legacyInitialize, LegacyVersion, "2024-11-05", 1)+legacyInitialized+`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`+"\n")
	if string(resultFields(t, responses[0])["protocolVersion"]) != `"2025-11-25"` {
		t.Fatal("unsupported client did not receive implemented legacy version")
	}
	if string(resultFields(t, responses[1])["tools"]) != "[]" {
		t.Fatal("negotiated legacy session unusable")
	}
}

func TestSchemaNullNeverCoercesToString(t *testing.T) {
	for _, schema := range []string{`{"type":"object","description":null}`, `{"type":"object","title":null}`, `{"type":"object","properties":{"":{"type":"string"}},"required":[null]}`, `{"type":"object","properties":{"x":{"type":["string",null]}}}`} {
		tool := syntheticTool("local")
		tool.InputSchema = json.RawMessage(schema)
		if _, err := New(Options{Tools: []Tool{tool}}); err == nil {
			t.Fatalf("null coerced in schema: %s", schema)
		}
	}
}

func toolCall(id, name, arguments string, modern bool) string {
	params := `"name":` + strconvQuote(name) + `,"arguments":` + arguments
	if modern {
		params += "," + currentMeta
	}
	return `{"jsonrpc":"2.0","id":` + id + `,"method":"tools/call","params":{` + params + `}}` + "\n"
}
func strconvQuote(text string) string { raw, _ := json.Marshal(text); return string(raw) }

func TestDispatchExactArgumentsOnce(t *testing.T) {
	const arguments = `{"label":"synthetic","sequence":9007199254740993}`
	var validations, calls atomic.Int32
	tool := syntheticTool("local")
	tool.InputSchema = json.RawMessage(`{"type":"object","properties":{"label":{"type":"string"},"sequence":{"type":"integer"}},"required":["label","sequence"],"additionalProperties":false}`)
	tool.Validate = func(ctx context.Context, raw json.RawMessage) error {
		validations.Add(1)
		if !bytes.Equal(raw, []byte(arguments)) {
			return errors.New("unsafe argument details")
		}
		return nil
	}
	tool.Handle = func(ctx context.Context, raw json.RawMessage) (ToolResult, error) {
		calls.Add(1)
		if string(raw) != arguments {
			t.Errorf("want unchanged argument bytes; got %q", raw)
		}
		return ToolResult{Content: []TextContent{{Text: "synthetic local result"}}, StructuredContent: raw}, nil
	}
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	responses := serveText(t, server, toolCall(`"current"`, "local", arguments, true))
	responses = append(responses, serveText(t, server, legacyInitialize+legacyInitialized+toolCall(`"legacy"`, "local", arguments, false))...)
	if len(responses) != 3 {
		t.Fatalf("want three responses; got %d", len(responses))
	}
	byID := make(map[string]map[string]json.RawMessage)
	for _, response := range responses {
		id := string(response["id"])
		if _, duplicate := byID[id]; duplicate {
			t.Fatal("duplicate response ID")
		}
		byID[id] = response
	}
	currentResponse, currentOK := byID[`"current"`]
	legacyResponse, legacyOK := byID[`"legacy"`]
	if !currentOK || !legacyOK {
		t.Fatal("missing correlated tool result")
	}
	current, legacy := resultFields(t, currentResponse), resultFields(t, legacyResponse)
	if string(current["resultType"]) != `"complete"` || string(current["structuredContent"]) != arguments {
		t.Fatalf("want exact current tool result; got %s", responses[0]["result"])
	}
	if _, ok := legacy["resultType"]; ok {
		t.Fatal("legacy resultType forbidden")
	}
	if string(legacy["structuredContent"]) != arguments {
		t.Fatalf("want exact legacy tool result; got %s", responses[2]["result"])
	}
	if calls.Load() != 2 || validations.Load() != 2 {
		t.Fatalf("want dispatch and validation once per request; got %d/%d", calls.Load(), validations.Load())
	}
}

const legacyInitialize = `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"synthetic-client","version":"test"}}}` + "\n"
const legacyInitialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"

func resultFields(t *testing.T, response map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response["result"], &fields); err != nil {
		t.Fatalf("want result; got %s", response["error"])
	}
	return fields
}

func TestLegacyHandshake(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	in := `{"jsonrpc":"2.0","id":"pre","method":"tools/list"}` + "\n" + legacyInitialize + `{"jsonrpc":"2.0","id":"notready","method":"tools/list"}` + "\n" + legacyInitialized + `{"jsonrpc":"2.0","id":"ready","method":"tools/list"}` + "\n"
	responses := serveText(t, server, in)
	if len(responses) != 4 {
		t.Fatalf("want four responses, notification silent; got %d", len(responses))
	}
	requireCode(t, responses[0], -32602)
	result := resultFields(t, responses[1])
	if string(result["protocolVersion"]) != `"2025-11-25"` || !object(result["serverInfo"]) {
		t.Fatalf("want legacy initialize result; got %s", responses[1]["result"])
	}
	if _, ok := result["resultType"]; ok {
		t.Fatal("legacy result must not contain current resultType")
	}
	// The SDK accepts calls after initialize, before the initialized notification.
	resultFields(t, responses[2])
	result = resultFields(t, responses[3])
	if string(result["tools"]) != `[]` {
		t.Fatalf("want empty legacy tools list; got %s", responses[3]["result"])
	}
}

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

// All protocol fixtures and handlers in this package are synthetic, not live.
func TestPerRequestMetadataNeverFallsBackToLegacyState(t *testing.T) {
	var calls int
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls++; return ToolResult{}, nil }
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, meta string
		code       int
	}{
		{"unsupported-date", `"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01","io.modelcontextprotocol/clientCapabilities":{}}`, -32022},
		{"private-version", `"_meta":{"io.modelcontextprotocol/protocolVersion":"SYNTHETIC-PRIVATE-MARKER","io.modelcontextprotocol/clientCapabilities":{}}`, -32022},
		{"missing-version", `"_meta":{"io.modelcontextprotocol/clientCapabilities":{}}`, -32602},
		{"missing-capabilities", `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}`, -32602},
		{"null-capabilities", `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":null}`, -32602},
		{"legacy-in-modern-meta", `"_meta":{"io.modelcontextprotocol/protocolVersion":"2025-11-25","io.modelcontextprotocol/clientCapabilities":{}}`, -32022},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := legacyInitialize + legacyInitialized + `{"jsonrpc":"2.0","id":"inline","method":"tools/call","params":{"name":"local","arguments":{},` + tc.meta + `}}` + "\n"
			responses := serveText(t, server, input)
			if len(responses) != 2 {
				t.Fatalf("want initialize and rejection; got %d", len(responses))
			}
			requireCode(t, responses[1], tc.code)
			var failure struct{ Message string }
			if json.Unmarshal(responses[1]["error"], &failure) != nil || bytes.Contains([]byte(failure.Message), []byte("SYNTHETIC-PRIVATE-MARKER")) {
				t.Fatal("version leaked into diagnostic message")
			}
			// MCP 2026-07-28 requires error.data.requested to echo the protocol
			// version. It is protocol negotiation data, never a credential field.
		})
	}
	if calls != 0 {
		t.Fatalf("want no dispatch; got %d", calls)
	}
	// Escaped spelling is decoded semantically, not compared as a raw literal.
	responses := serveText(t, server, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-2\u0038","io.modelcontextprotocol/clientCapabilities":{}}}}`+"\n")
	if string(resultFields(t, responses[0])["resultType"]) != `"complete"` {
		t.Fatal("want decoded metadata")
	}
}

func TestInvalidNotificationsNeverReplyOrInvokeHandlers(t *testing.T) {
	var calls int
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls++; return ToolResult{}, nil }
	server, err := New(Options{Tools: []Tool{tool}})
	if err != nil {
		t.Fatal(err)
	}
	notifications := []string{
		`{"jsonrpc":"1.0","method":"tools/call","params":{` + currentMeta + `,"name":"local"}}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":[]}`,
		`{"jsonrpc":"2.0","method":42}`,
		`{"jsonrpc":"2.0","method":"tools/call","method":"server/discover"}`,
		`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"local","arguments":{"x":1,"\u0078":2},` + currentMeta + `}}`,
		`{"jsonrpc":"2.0","METHOD":"tools/call","params":{` + currentMeta + `,"name":"local"}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":null}}`,
	}
	for _, frame := range notifications {
		var output bytes.Buffer
		_ = server.Serve(context.Background(), strings.NewReader(frame+"\n"), &output)
		if output.Len() != 0 {
			t.Fatal("invalid notification emitted output")
		}
	}
	if calls != 0 {
		t.Fatal("invalid notification invoked a handler")
	}
}

func TestMethodParameterCaseAndUnsupportedFieldsBlockDispatch(t *testing.T) {
	var calls int
	tool := syntheticTool("local")
	tool.Handle = func(context.Context, json.RawMessage) (ToolResult, error) { calls++; return ToolResult{}, nil }
	mutation := syntheticTool("mutate")
	mutation.ReadOnly = false
	mutation.Handle = tool.Handle
	server, err := New(Options{Tools: []Tool{tool, mutation}})
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []string{
		`"name":"local","arguments":{},"Name":"private",` + currentMeta,
		`"name":"local","arguments":{},"inputResponses":{},` + currentMeta,
		`"name":"mutate","arguments":{},` + currentMeta,
		`"name":"local","arguments":{},"_meta":{"io.modelcontextprotocol/ProtocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`,
	} {
		replies := serveText(t, server, legacyInitialize+legacyInitialized+`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{`+params+`}}`+"\n")
		requireCode(t, replies[1], -32602)
	}
	if calls != 0 {
		t.Fatal("forged or invalid call reached handler")
	}
}

func TestLegacyPingAndUnknownMethodsHaveExplicitRoutes(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"", legacyInitialize + legacyInitialized} {
		replies := serveText(t, server, prefix+`{"jsonrpc":"2.0","id":"unknown","method":"arbitrary"}`+"\n")
		code := -32601
		if prefix == "" {
			code = -32603
		} // SDK rejects calls before initialization.
		requireCode(t, replies[len(replies)-1], code)
	}
	replies := serveText(t, server, `{"jsonrpc":"2.0","id":"ping","method":"ping"}`+"\n")
	if string(replies[0]["result"]) != "{}" {
		t.Fatal("want legacy empty ping result")
	}
}

func TestDiscoveryListAndIdentityParametersAreValidated(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"private":true,` + currentMeta + `}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"cursor":"invalid-token",` + currentMeta + `}}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{}}}}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{}}}`,
	}
	for i, input := range cases {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			replies := serveText(t, server, input+"\n")
			requireCode(t, replies[0], -32602)
		})
	}
}

func TestCurrentUnknownMethodCannotBecomeDiscovery(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"arbitrary", "initialize", "resources/list", "tasks/get", "Tools/list"} {
		t.Run(method, func(t *testing.T) {
			responses := serveText(t, server, `{"jsonrpc":"2.0","id":"synthetic","method":`+strconvQuote(method)+`,"params":{`+currentMeta+`}}`+"\n")
			if len(responses) != 1 {
				t.Fatalf("want one error; got %d", len(responses))
			}
			requireCode(t, responses[0], -32601)
		})
	}
}

func TestModernDiscoveryAndListIncludeCachePolicy(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"server/discover", "tools/list"} {
		t.Run(method, func(t *testing.T) {
			replies := serveText(t, server, `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{`+currentMeta+`}}`+"\n")
			result := resultFields(t, replies[0])
			if string(result["ttlMs"]) != "0" || string(result["cacheScope"]) != `"private"` {
				t.Fatalf("missing noncacheable private result policy: %s", replies[0]["result"])
			}
		})
	}
}

func TestSDKUnsupportedVersionIncludesNegotiationData(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	meta := strings.Replace(currentMeta, CurrentVersion, "2099-01-01", 1)
	replies := serveText(t, server, `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+meta+`}}`+"\n")
	requireCode(t, replies[0], -32022)
	var failure struct {
		Data struct {
			Supported []string `json:"supported"`
			Requested string   `json:"requested"`
		} `json:"data"`
	}
	if err := json.Unmarshal(replies[0]["error"], &failure); err != nil {
		t.Fatal(err)
	}
	if len(failure.Data.Supported) == 0 || failure.Data.Supported[0] != CurrentVersion || failure.Data.Requested != "2099-01-01" {
		t.Fatal("unsupported-version response must carry the protocol negotiation data")
	}
}

func TestLegacyUnknownVersionReceivesCounteroffer(t *testing.T) {
	server, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	replies := serveText(t, server, strings.Replace(legacyInitialize, LegacyVersion, "1900-01-01", 1))
	if got := string(resultFields(t, replies[0])["protocolVersion"]); got != `"`+LegacyVersion+`"` {
		t.Fatalf("want legacy counteroffer; got %s", got)
	}
}

func TestSchemaNullStringsAreRejected(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","description":null}`,
		`{"type":"object","title":null}`,
		`{"type":"object","properties":{"":{"type":"string"}},"required":[null]}`,
	} {
		tool := syntheticTool("local")
		tool.InputSchema = json.RawMessage(schema)
		if _, err := New(Options{Tools: []Tool{tool}}); err != ErrConfiguration {
			t.Fatalf("want configuration rejection for %s", schema)
		}
	}
}

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
