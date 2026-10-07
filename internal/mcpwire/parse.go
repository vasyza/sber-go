package mcpwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/vasyza/sber-go/internal/strictjson"
)

var ErrClientResponse = errors.New("client responses are not permitted on stdio")

type request struct {
	id           json.RawMessage
	method       string
	params       map[string]json.RawMessage
	meta         map[string]json.RawMessage
	modern       bool
	notification bool
}

func stringValue(raw json.RawMessage) (string, bool) {
	var text string
	returnText := len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &text) == nil
	return text, returnText
}

func parseRequest(frame []byte) (request, int, error) {
	var req request
	if strictjson.Validate(frame) != nil {
		// Rejected bytes can only be classified for silence, never dispatched or
		// echoed. No permissive decode is an acceptance oracle.
		var invalid map[string]json.RawMessage
		if json.Unmarshal(frame, &invalid) == nil {
            if _,exists:=invalid["result"];exists{return req,0,ErrClientResponse}
            if _,exists:=invalid["error"];exists{return req,0,ErrClientResponse}
            req.notification = notificationShape(invalid)
        }
		return req, -32700, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(frame, &fields) != nil || fields == nil {
		return req, -32600, nil
	}
	if _, response := fields["result"]; response {
		return req, 0, ErrClientResponse
	}
	if _, response := fields["error"]; response {
		return req, 0, ErrClientResponse
	}
	req.notification = notificationShape(fields)
	if raw, hasID := fields["id"]; hasID {
		if !validID(raw) {
			return req, -32600, nil
		}
		req.id = append(json.RawMessage(nil), raw...)
	}
	method, methodOK := stringValue(fields["method"])
	req.method = method

	version, versionOK := stringValue(fields["jsonrpc"])
	if !versionOK || version != "2.0" || !methodOK || method == "" {
		return req, -32600, nil
	}
	for key := range fields {
		switch key {
		case "jsonrpc", "id", "method", "params":
		default:
			return req, -32600, nil
		}
	}
	req.params = map[string]json.RawMessage{}
	if raw, exists := fields["params"]; exists {
		if !object(raw) {
			return req, -32602, nil
		}
		_ = json.Unmarshal(raw, &req.params)
	}
	if raw, exists := req.params["_meta"]; exists {
		if !object(raw) {
			return req, -32602, nil
		}
		_ = json.Unmarshal(raw, &req.meta)
	}
	_, inlineVersion := req.meta["io.modelcontextprotocol/protocolVersion"]
	_, inlineCapabilities := req.meta["io.modelcontextprotocol/clientCapabilities"]
	req.modern = method == "server/discover" || inlineVersion || inlineCapabilities
	for key := range req.meta {
		if strings.EqualFold(key, "io.modelcontextprotocol/protocolVersion") || strings.EqualFold(key, "io.modelcontextprotocol/clientCapabilities") { req.modern = true }
	}
	return req, 0, nil
}

func staticMessage(code int) string {
	switch code {
	case -32700:
		return "Parse error"
	case -32600:
		return "Invalid request"
	case -32601:
		return "Method not found"
	case -32602:
		return "Invalid params"
	case -32603:
		return "Internal error"
	case -32022:
		return "Unsupported protocol version"
	}
	return "Internal error"
}

func trimmed(raw json.RawMessage) json.RawMessage { return bytes.TrimSpace(raw) }

func notificationShape(fields map[string]json.RawMessage) bool {
	if fields == nil { return false }
	if _, hasID := fields["id"]; hasID { return false }
	for key := range fields { if strings.EqualFold(key, "method") { return true } }
	return false
}

func allowedFields(fields map[string]json.RawMessage, allowed ...string) bool {
	for key := range fields {
		found := false
		for _, name := range allowed { if key == name { found = true; break } }
		if !found { return false }
	}
	return true
}

func validCancellation(req request) bool {
	if !allowedFields(req.params, "_meta", "requestId", "reason") || !validID(req.params["requestId"]) { return false }
	if reason, exists := req.params["reason"]; exists {
		if _, ok := stringValue(reason); !ok { return false }
	}
	return true
}

func validImplementation(raw json.RawMessage) bool {
	if !object(raw){return false}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw,&fields)!=nil{return false}
	name,nameOK:=stringValue(fields["name"])
	version,versionOK:=stringValue(fields["version"])
	return nameOK&&versionOK&&name!=""&&version!=""
}
