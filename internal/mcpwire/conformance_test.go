package mcpwire

import (
	"encoding/json"
	"strings"
	"testing"
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
