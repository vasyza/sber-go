package mcpwire

import (
	"encoding/json"
	"strings"
	"testing"
)

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
