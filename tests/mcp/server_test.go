package mcp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`

func roundTrip(t *testing.T, server *mcp.Server, method, params string) map[string]json.RawMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, writer := io.Pipe()
	output, receiver := io.Pipe()
	defer writer.Close()
	defer output.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, receiver) }()
	_, err := io.WriteString(writer, `{"jsonrpc":"2.0","id":1,"method":"`+method+`","params":{`+params+`}}`+"\n")
	if err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}
func toolResult(t *testing.T, envelope map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	if envelope["error"] != nil {
		t.Fatalf("unexpected protocol error: %s", envelope["error"])
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(envelope["result"], &result); err != nil {
		t.Fatal(err)
	}
	if string(result["isError"]) == "true" {
		t.Fatalf("unexpected tool error: %s", envelope["result"])
	}
	return result
}
func TestCatalogAdvertisesOnlyImplementedSessionTools(t *testing.T) {
	client := &testutil.Client{}
	server, err := mcp.New(mcp.Options{Client: client})
	if err != nil {
		t.Fatal(err)
	}
	envelope := roundTrip(t, server, "tools/list", meta)
	var result struct {
		Tools []struct{ Name string }
		TTL   int    `json:"ttlMs"`
		Scope string `json:"cacheScope"`
	}
	if json.Unmarshal(envelope["result"], &result) != nil || len(result.Tools) != 6 || result.Scope != "private" {
		t.Fatal("catalog not usable")
	}
	for _, tool := range result.Tools {
		if strings.Contains(tool.Name, "auth") || strings.Contains(tool.Name, "transfer") {
			t.Fatal("unimplemented or financial tool advertised")
		}
	}
	if len(client.Requests()) != 0 {
		t.Fatal("discovery contacted a bank")
	}
}
func TestProductsAndHistoryRunThroughTypedSDK(t *testing.T) {
	client := &testutil.Client{}
	server, _ := mcp.New(mcp.Options{Client: client})
	products := toolResult(t, roundTrip(t, server, "tools/call", meta+`,"name":"sber_products","arguments":{"force_update":true}`))
	if !strings.Contains(string(products["structuredContent"]), "9007199254740993.10") || strings.Contains(string(products["structuredContent"]), "4111111111111111") {
		t.Fatal("financial export changed precision or masking")
	}
	requests := client.Requests()
	if len(requests) != 1 || requests[0].Body["forceUpdate"] != true {
		t.Fatal("products handler not bound")
	}
	toolResult(t, roundTrip(t, server, "tools/call", meta+`,"name":"sber_products","arguments":{"session_id":null}`))
	if len(client.Requests()) != 2 {
		t.Fatal("nullable default scope did not use the selected client")
	}
	history := toolResult(t, roundTrip(t, server, "tools/call", meta+`,"name":"sber_operations","arguments":{"limit":3e1,"max_pages":1.0,"from_date":"2026-01-01"}`))
	if !strings.Contains(string(history["structuredContent"]), `"WindowCompleteness":"unknown"`) {
		t.Fatal("history incorrectly claims known coverage")
	}
}
func TestSessionMetadataAndArgumentsKeepCredentialBoundary(t *testing.T) {
	client := &testutil.Client{}
	server, _ := mcp.New(mcp.Options{Client: client})
	info := toolResult(t, roundTrip(t, server, "tools/call", meta+`,"name":"sber_session_info","arguments":{"check_live":true}`))
	if strings.Contains(string(info["structuredContent"]), "synthetic-private-cookie") || client.Warmups.Load() != 1 {
		t.Fatal("session metadata boundary failed")
	}
	for _, arguments := range []string{`{"session_id":"unselected"}`, `{"password":"synthetic-private"}`, `{"force_update":null}`} {
		envelope := roundTrip(t, server, "tools/call", meta+`,"name":"sber_products","arguments":`+arguments)
		if envelope["error"] == nil {
			t.Fatal("unsafe arguments reached handler")
		}
	}
	if len(client.Requests()) != 0 {
		t.Fatal("argument rejection performed a bank read")
	}
}
func TestSessionCloseInvalidatesFinancialReads(t *testing.T) {
	client := &testutil.Client{}
	server, _ := mcp.New(mcp.Options{Client: client})
	toolResult(t, roundTrip(t, server, "tools/call", meta+`,"name":"sber_session_close","arguments":{}`))
	envelope := roundTrip(t, server, "tools/call", meta+`,"name":"sber_products","arguments":{}`)
	if !strings.Contains(string(envelope["result"]), `"isError":true`) || len(client.Requests()) != 0 || client.Closes.Load() != 1 {
		t.Fatal("closed session resurrected")
	}
}
