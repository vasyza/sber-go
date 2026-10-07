// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// DecodeJSON preserves number tokens for exact money and rejects trailing JSON,
// malformed Unicode and decoded duplicate properties before Go can repair or
// overwrite them. Opaque identifiers must retain their original meaning.
// It does not perform transport, read HAR headers or load private profiles.
func DecodeJSON(r io.Reader) (map[string]any, error) {
	data, err := io.ReadAll(r)
	if err != nil || strictjson.Validate(data) != nil {
		return nil, NewParseError("JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, NewParseError("JSON")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, NewParseError("JSON")
	}
	return domainMapping(value, "JSON")
}

func domainMapping(value any, name string) (map[string]any, error) {
	m, ok := value.(map[string]any)
	if !ok || m == nil {
		return nil, NewParseError(name)
	}
	return m, nil
}

func domainOptionalMapping(value any) map[string]any { m, _ := value.(map[string]any); return m }

func domainID(value any) string {
	if value == nil {
		return ""
	}
	if b, ok := value.(bool); ok {
		if b {
			return "True"
		}
		return "False"
	}
	return fmt.Sprint(value)
}

func domainText(value any) string { return RedactPAN(domainID(value)) }

var panCandidate = regexp.MustCompile(`\p{Nd}(?:[ \x{00a0}-]?\p{Nd}){12,18}`)

// RedactPAN masks only Luhn-valid 13–19 digit candidates, including separated
// PANs; non-card long identifiers are not indiscriminately destroyed.
func RedactPAN(text string) string {
	var out strings.Builder
	previous, offset := 0, 0
	for offset < len(text) {
		span := panCandidate.FindStringIndex(text[offset:])
		if span == nil {
			break
		}
		a, b := offset+span[0], offset+span[1]
		first, size := utf8.DecodeRuneInString(text[a:])
		_ = first
		if a > 0 {
			prior, _ := utf8.DecodeLastRuneInString(text[:a])
			if unicode.IsDigit(prior) {
				offset = a + size
				continue
			}
		}
		// Simulate the source regex's negative-lookahead backtracking: a greedy
		// 19-digit prefix can cross into the next PAN and must fall back to the
		// longest complete candidate, not consume digits from that next PAN.
		end, count := 0, 0
		for i, r := range text[a:b] {
			if unicode.IsDigit(r) {
				count++
				pos := a + i + utf8.RuneLen(r)
				next, _ := utf8.DecodeRuneInString(text[pos:])
				if count >= 13 && !unicode.IsDigit(next) {
					end = pos
				}
			}
		}
		if end == 0 {
			offset = a + size
			continue
		}
		b = end
		offset = b
		digits := []rune(domainDigits(text[a:b]))
		if !domainLuhn(string(digits)) {
			continue
		}
		out.WriteString(text[previous:a])
		out.WriteString("•••• ")
		out.WriteString(string(digits[len(digits)-4:]))
		previous = b
	}
	out.WriteString(text[previous:])
	return out.String()
}

func domainDigits(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func domainLuhn(number string) bool {
	digits := []rune(number)
	if len(digits) == 0 {
		return false
	}
	total := 0
	for i, r := range digits {
		d, ok := domainDigitValue(r)
		if !ok {
			return false
		}
		if i%2 == len(digits)%2 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		total += d
	}
	return total%10 == 0
}

func domainLast4(value any) string {
	if !domainTruthy(value) {
		return ""
	}
	digits := []rune(domainDigits(domainID(value)))
	if len(digits) > 4 {
		return string(digits[len(digits)-4:])
	}
	return string(digits)
}

func domainDataList(section any, name string) ([]any, error) {
	v := domainOptionalMapping(section)["data"]
	if v == nil {
		return nil, nil
	}
	xs, ok := v.([]any)
	if !ok {
		return nil, NewParseError(name + ".data")
	}
	return xs, nil
}

func domainStringPointer(s string) *string { return &s }

func domainResource(value any) *Resource {
	raw := domainOptionalMapping(value)
	if !domainTruthy(raw["id"]) {
		return nil
	}
	id := domainID(raw["id"])
	kind, rest, found := strings.Cut(id, ":")
	if !found {
		return &Resource{Type: "unknown", ID: id}
	}
	return &Resource{Type: kind, ID: rest}
}

func domainSoftMoney(value any) *Money { m, _ := ParseMoney(value); return m }

// Absent/null money objects and empty objects retain the source's unknown
// amount semantics. Explicit nonobjects are malformed core data, not absence;
// optional monetary metadata uses domainSoftMoney instead.
func domainCoreMoney(value any, field string) (*Money, error) {
	if value == nil {
		return nil, nil
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, NewParseError(field)
	}
	return ParseMoney(value)
}

func domainEnvelope(payload map[string]any) error {
	if _, present := payload["sourceErrorResponse"]; present {
		return NewParseError("sourceErrorResponse")
	}
	if v, present := payload["success"]; present {
		if v != true && v != "true" {
			return NewParseError("success")
		}
	} else if _, present := payload["error"]; present {
		// Source error-only responses omit success. A missing product section
		// in such a response is unavailable, not a known empty portfolio.
		return NewParseError("error")
	}
	if _, present := domainOptionalMapping(payload["body"])["sourceErrorResponse"]; present {
		return NewParseError("body.sourceErrorResponse")
	}
	return nil
}
