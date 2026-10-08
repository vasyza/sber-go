package mcpwire

import (
	"bytes"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/vasyza/sber-go/internal/strictjson"
)

var ErrConfiguration = errors.New("invalid MCP configuration")

func validName(name string) bool {
	if len(name) == 0 || len(name) > 128 {
		return false
	}
	for _, c := range []byte(name) {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

// Only this documented structural subset is accepted; the mandatory concrete
// validator, not encoding/json or the descriptor, enforces tool arguments.
func validSchema(raw json.RawMessage, depth int) bool {
	if depth > 16 || len(raw) > 65536 || strictjson.Validate(raw) != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for key, value := range fields {
		switch key {
		case "$schema":
			if string(value) != `"https://json-schema.org/draft/2020-12/schema"` {
				return false
			}
		case "type":
			single, isString := stringValue(value)
			if isString {
				if !schemaType(single) {
					return false
				}
			} else {
				var types []json.RawMessage
				if json.Unmarshal(value, &types) != nil || len(types) == 0 {
					return false
				}
				seen := map[string]bool{}
				for _, token := range types {
					typ, ok := stringValue(token)
					if !ok || !schemaType(typ) || seen[typ] {
						return false
					}
					seen[typ] = true
				}
			}
		case "properties":
			var properties map[string]json.RawMessage
			if json.Unmarshal(value, &properties) != nil || properties == nil {
				return false
			}
			for _, schema := range properties {
				if !validSchema(schema, depth+1) {
					return false
				}
			}
		case "items":
			if !validSchema(value, depth+1) {
				return false
			}
		case "additionalProperties":
			if !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
				return false
			}
		case "required":
			var names []json.RawMessage
			if json.Unmarshal(value, &names) != nil || names == nil {
				return false
			}
			var properties map[string]json.RawMessage
			_ = json.Unmarshal(fields["properties"], &properties)
			seen := map[string]bool{}
			for _, token := range names {
				name, isString := stringValue(token)
				if _, ok := properties[name]; !isString || !ok || seen[name] {
					return false
				}
				seen[name] = true
			}
		case "enum":
			var choices []json.RawMessage
			if json.Unmarshal(value, &choices) != nil || len(choices) == 0 {
				return false
			}
		case "description", "title":
			text, ok := stringValue(value)
			if !ok || !utf8.ValidString(text) {
				return false
			}
		case "default": // Any strict JSON value is valid descriptor metadata.
		default:
			return false
		}
	}
	return true
}

func schemaType(name string) bool {
	switch name {
	case "object", "array", "string", "integer", "number", "boolean", "null":
		return true
	}
	return false
}

func objectSchemaRoot(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false
	}
	typ, ok := stringValue(fields["type"])
	return ok && typ == "object"
}
