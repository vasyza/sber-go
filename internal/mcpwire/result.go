package mcpwire

import (
	"bytes"
	"encoding/json"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// Raw results are deliberately limited to text content and structured objects.
// Validation sees original bytes before encoding/json can erase bad Unicode.
func toolResultFields(raw json.RawMessage) (map[string]any, bool) {
	if strictjson.Validate(raw) != nil || !object(raw) {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, false
	}
	for key, value := range fields {
		switch key {
		case "content":
			var items []json.RawMessage
			if json.Unmarshal(value, &items) != nil || items == nil {
				return nil, false
			}
			for _, item := range items {
				var text map[string]json.RawMessage
				if json.Unmarshal(item, &text) != nil || len(text) != 2 {
					return nil, false
				}
				typ, ok := stringValue(text["type"])
				if !ok || typ != "text" {
					return nil, false
				}
				if _, ok := stringValue(text["text"]); !ok {
					return nil, false
				}
			}
		case "structuredContent":
			if !object(value) {
				return nil, false
			}
		case "isError":
			if !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	if _, exists := fields["content"]; !exists {
		return nil, false
	}
	result := make(map[string]any, len(fields))
	for k, v := range fields {
		result[k] = append(json.RawMessage(nil), v...)
	}
	return result, true
}
