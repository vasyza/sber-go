// Package mcpwire adapts application-injected, read-only tools to the official
// MCP Go SDK. It contains no bank handlers, credentials, or session discovery.
package mcpwire

import (
	"encoding/json"
)

const CurrentVersion = "2026-07-28"
const LegacyVersion = "2025-11-25"

// Implementation is self-reported display information, never an identity proof.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func object(data json.RawMessage) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal(data, &value) == nil && value != nil
}

func stringValue(raw json.RawMessage) (string, bool) {
	var text string
	ok := len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &text) == nil
	return text, ok
}

func validImplementation(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	name, nameOK := stringValue(fields["name"])
	version, versionOK := stringValue(fields["version"])
	return nameOK && versionOK && name != "" && version != ""
}

func staticMessage(code int64) string {
	switch code {
	case -32700:
		return "Parse error"
	case -32600:
		return "Invalid request"
	case -32601:
		return "Method not found"
	case -32602:
		return "Invalid params"
	case -32022:
		return "Unsupported protocol version"
	default:
		return "Internal error"
	}
}
