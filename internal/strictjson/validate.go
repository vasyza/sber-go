// Package strictjson validates untrusted JSON without rewriting identifiers.
// It performs no I/O, authentication, logging or JavaScript evaluation.
package strictjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// ErrInvalidJSON is deliberately static; source tokens never enter diagnostics.
var ErrInvalidJSON = errors.New("invalid JSON document")

// Validate requires a complete JSON document with valid Unicode text.
func Validate(data []byte) error {
	if !utf8.Valid(data) || !json.Valid(data) || !validStringEscapes(data) {
		return ErrInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if !uniqueValue(decoder) {
		return ErrInvalidJSON
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrInvalidJSON
	}
	return nil
}

// Compare decoded property names, including escaped spellings. Seen-key sets
// are per object: separate objects may legitimately share property names.
func uniqueValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, composite := token.(json.Delim)
	if !composite {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := token.(string)
			if !ok {
				return false
			}
			if _, duplicate := seen[key]; duplicate {
				return false
			}
			seen[key] = struct{}{}
			if !uniqueValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !uniqueValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

// encoding/json accepts unpaired surrogate escapes by replacing them. Validate
// their spelling before any decode so opaque identifiers cannot silently merge.
func validStringEscapes(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		first, ok := hexCodeUnit(data[i+1 : i+5])
		if !ok {
			return false
		}
		if first >= 0xd800 && first <= 0xdbff {
			if i+10 >= len(data) || data[i+5] != '\\' || data[i+6] != 'u' {
				return false
			}
			second, ok := hexCodeUnit(data[i+7 : i+11])
			if !ok || second < 0xdc00 || second > 0xdfff {
				return false
			}
			i += 10
		} else {
			if first >= 0xdc00 && first <= 0xdfff {
				return false
			}
			i += 4
		}
	}
	return !inString
}

func hexCodeUnit(data []byte) (uint16, bool) {
	if len(data) != 4 {
		return 0, false
	}
	var value uint16
	for _, c := range data {
		var digit uint16
		switch {
		case c >= '0' && c <= '9':
			digit = uint16(c - '0')
		case c >= 'a' && c <= 'f':
			digit = uint16(c-'a') + 10
		case c >= 'A' && c <= 'F':
			digit = uint16(c-'A') + 10
		default:
			return 0, false
		}
		value = value*16 + digit
	}
	return value, true
}
