package session

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	"github.com/vasyza/sber-go/internal/strictjson"
)

const MaxFrontendHTMLCharacters = 4 * 1024 * 1024

// FrontendConfig is an immutable primary/PIN runtime snapshot. Raw fields are
// available only through explicit accessors. Ordinary fmt/log/JSON output hides
// them, including fmt fallbacks that bypass formatting methods.
// ParsePINConfig/ParsePrimaryConfig validate runtime input; NewFrontendConfig is
// deliberate raw construction and never validates, normalizes or invents SRP.
// Copies share immutable identity. Compare accessors for content equality.
// The zero value reads empty/zero/false and remains comparable to FrontendConfig{}.
// See docs/SDK.md for the constructor and accessor contracts.
type FrontendConfig struct{ data **frontendConfigData }

type frontendConfigData struct {
	baseURL, processID               string
	pinLength                        int
	nHex, gHex                       string
	seamlessWeb, redirectPost        bool
	cardKeyID, cardKeyValue, qrScope string
	qrSize                           int
}

// NewFrontendConfig retains every supplied value verbatim, including partial
// synthetic configs. It performs no parsing, validation, network access or SRP
// defaulting. All-empty arguments return the native zero value.
func NewFrontendConfig(baseURL, processID string, pinLength int, nHex, gHex string, seamlessWeb, redirectPost bool) FrontendConfig {
	if baseURL == "" && processID == "" && pinLength == 0 && nHex == "" && gHex == "" && !seamlessWeb && !redirectPost {
		return FrontendConfig{}
	}
	data := &frontendConfigData{baseURL: baseURL, processID: processID, pinLength: pinLength, nHex: nHex, gHex: gHex, seamlessWeb: seamlessWeb, redirectPost: redirectPost}
	// The terminal pointee must itself be a pointer: with an ordinary *record,
	// fmt's nested badVerb can reset depth to zero and dereference the record.
	// Neither level escapes through an accessor or a mutable raw-fields DTO.
	return FrontendConfig{data: &data}
}

// BaseURL is an explicit raw read for constructing legitimate wire endpoints.
func (c FrontendConfig) BaseURL() string {
	if c.data == nil {
		return ""
	}
	return (*c.data).baseURL
}

// ProcessID is an explicit raw read for legitimate process headers.
func (c FrontendConfig) ProcessID() string {
	if c.data == nil {
		return ""
	}
	return (*c.data).processID
}

// PINLength returns the supplied length; the zero value has no invented default.
func (c FrontendConfig) PINLength() int {
	if c.data == nil {
		return 0
	}
	return (*c.data).pinLength
}

// NHex is an explicit raw read of the supplied SRP modulus.
func (c FrontendConfig) NHex() string {
	if c.data == nil {
		return ""
	}
	return (*c.data).nHex
}

// GHex is an explicit raw read of the supplied SRP generator.
func (c FrontendConfig) GHex() string {
	if c.data == nil {
		return ""
	}
	return (*c.data).gHex
}

// SeamlessWeb returns the supplied seamless-navigation flag.
func (c FrontendConfig) SeamlessWeb() bool {
	if c.data == nil {
		return false
	}
	return (*c.data).seamlessWeb
}

// RedirectPost returns the supplied redirect-method flag.
func (c FrontendConfig) RedirectPost() bool {
	if c.data == nil {
		return false
	}
	return (*c.data).redirectPost
}

// CardEncryptionKey returns the public RSA key and its bank key identifier.
// The key is used only to encrypt a card number for an authentication request.
func (c FrontendConfig) CardEncryptionKey() (id, key string) {
	if c.data != nil {
		return (*c.data).cardKeyID, (*c.data).cardKeyValue
	}
	return "", ""
}
func (c FrontendConfig) QRScope() string {
	if c.data == nil {
		return ""
	}
	return (*c.data).qrScope
}
func (c FrontendConfig) QRSize() int {
	if c.data == nil {
		return 0
	}
	return (*c.data).qrSize
}

func (c FrontendConfig) String() string               { return "FrontendConfig(<redacted>)" }
func (c FrontendConfig) GoString() string             { return c.String() }
func (c FrontendConfig) Format(f fmt.State, v rune)   { sdkErrs.FormatError(f, c.String()) }
func (c FrontendConfig) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }
func invalidFrontendConfig() error {
	return &sdkErrs.PinAuthError{Code: "invalid_frontend_config", Message: "invalid frontend config"}
}

// ParsePINConfig accepts only the remembered-browser PIN mode, with PIN enabled
// and already enrolled. Every security-relevant field must be a literal.
func ParsePINConfig(html string) (FrontendConfig, error) {
	invalid := func() (FrontendConfig, error) { return FrontendConfig{}, invalidFrontendConfig() }
	props, ok := frontendProperties(html)
	if !ok {
		return invalid()
	}
	pin, ok := objectProperties(props["pinConfig"])
	if !ok {
		return invalid()
	}
	mode, ok := literalString(props, "authTypeByCookie")
	if !ok || mode != "pin" {
		return invalid()
	}
	enabled, ok := literalBool(pin, "enabled")
	if !ok || !enabled {
		return invalid()
	}
	enrolled, ok := literalBool(pin, "hasPin")
	if !ok || !enrolled {
		return invalid()
	}
	length, ok := literalInt(pin, "length")
	if !ok || length < 4 || length > 12 {
		return invalid()
	}
	n, nOK := literalString(pin, "srp_N")
	g, gOK := literalString(pin, "srp_g")
	if !nOK || !gOK || !safeFrontendGroup(n, g) {
		return invalid()
	}
	return assembleFrontendConfig(props, length, n, g)
}

// ParsePrimaryConfig accepts only cold-browser primary SRP and new PIN
// enrollment. It never falls back to PIN parameters or synthesizes a group.
func ParsePrimaryConfig(html string) (FrontendConfig, error) {
	invalid := func() (FrontendConfig, error) { return FrontendConfig{}, invalidFrontendConfig() }
	props, ok := frontendProperties(html)
	if !ok {
		return invalid()
	}
	mode, ok := literalString(props, "authTypeByCookie")
	if !ok || mode != "start" {
		return invalid()
	}
	srp, ok := objectProperties(props["srpConfig"])
	if !ok {
		return invalid()
	}
	enabled, ok := literalBool(srp, "enabled")
	if !ok || !enabled {
		return invalid()
	}
	pin, ok := objectProperties(props["pinConfig"])
	if !ok {
		return invalid()
	}
	enabled, ok = literalBool(pin, "enabled")
	if !ok || !enabled {
		return invalid()
	}
	hasPIN, ok := literalBool(pin, "hasPin")
	if !ok || hasPIN {
		return invalid()
	}
	validation, ok := objectProperties(props["validation"])
	if !ok {
		return invalid()
	}
	pinValidation, ok := objectProperties(validation["pin"])
	if !ok {
		return invalid()
	}
	length, ok := literalInt(pinValidation, "length")
	if !ok || length < 4 || length > 12 {
		return invalid()
	}
	n, nOK := literalString(srp, "N")
	g, gOK := literalString(srp, "g")
	if !nOK || !gOK || !safeFrontendGroup(n, g) {
		return invalid()
	}
	return assembleFrontendConfig(props, length, n, g)
}

func assembleFrontendConfig(props map[string]string, length int, n, g string) (FrontendConfig, error) {
	invalid := func() (FrontendConfig, error) { return FrontendConfig{}, invalidFrontendConfig() }
	base, ok := literalString(props, "baseApiUrl")
	if !ok {
		return invalid()
	}
	base, ok = normalizeAuthBase(base)
	if !ok {
		return invalid()
	}
	process, ok := literalString(props, "processId")
	if !ok || process == "" || utf8.RuneCountInString(process) > 512 || HasControls(process) {
		return invalid()
	}
	seamless, ok := optionalLiteralBool(props, "isSeamlessWeb")
	if !ok {
		return invalid()
	}
	post, ok := optionalLiteralBool(props, "isUfsRedirectMethodPostEnabled")
	if !ok {
		return invalid()
	}
	c := NewFrontendConfig(base, process, length, n, g, seamless, post)
	(*c.data).qrSize = 190
	for name, dest := range map[string]*string{"encryptionKeyId": &(*c.data).cardKeyID, "encryptionKeyValue": &(*c.data).cardKeyValue} {
		if _, present := props[name]; present {
			v, valid := literalString(props, name)
			if !valid || len(v) > 16384 || HasControls(v) {
				return invalid()
			}
			*dest = v
		}
	}
	if raw, present := props["qrConfig"]; present {
		qr, valid := objectProperties(raw)
		if !valid {
			return invalid()
		}
		if _, present = qr["useIdentifyScopeSbol"]; present {
			v, valid := literalString(qr, "useIdentifyScopeSbol")
			if !valid || len(v) > 128 || HasControls(v) {
				return invalid()
			}
			(*c.data).qrScope = v
		}
		if _, present = qr["size"]; present {
			v, valid := literalInt(qr, "size")
			if !valid || v < 64 || v > 2048 {
				return invalid()
			}
			(*c.data).qrSize = v
		}
	}
	return c, nil
}
func safeFrontendGroup(nHex, gHex string) bool {
	n, nOK := frontendHex(nHex)
	g, gOK := frontendHex(gHex)
	return nOK && gOK && n.BitLen() >= 2048 && n.BitLen() <= 8192 && g.Cmp(big.NewInt(1)) > 0 && g.Cmp(n) < 0
}

var frontendHexPattern = regexp.MustCompile(`^[0-9a-fA-F]+$`)
var frontendIntPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func frontendHex(value string) (*big.Int, bool) {
	if !frontendHexPattern.MatchString(value) {
		return nil, false
	}
	return new(big.Int).SetString(value, 16)
}
func literalString(props map[string]string, key string) (string, bool) {
	raw, exists := props[key]
	if !exists || !strings.HasPrefix(raw, `"`) {
		return "", false
	}
	var value string
	data := []byte(raw)
	if strictjson.Validate(data) != nil || json.Unmarshal(data, &value) != nil {
		return "", false
	}
	return value, true
}
func literalBool(props map[string]string, key string) (bool, bool) {
	switch props[key] {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}
func optionalLiteralBool(props map[string]string, key string) (bool, bool) {
	if _, ok := props[key]; !ok {
		return false, true
	}
	return literalBool(props, key)
}
func literalInt(props map[string]string, key string) (int, bool) {
	raw := props[key]
	if !frontendIntPattern.MatchString(raw) {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil
}

func frontendProperties(html string) (map[string]string, bool) {
	if html == "" || !utf8.ValidString(html) || utf8.RuneCountInString(html) > MaxFrontendHTMLCharacters {
		return nil, false
	}
	// Match the Python source guard against the entire document, including quoted
	// or commented occurrences: ambiguity is rejected, never heuristically ignored.
	matches := frontendAssignments(html)
	if len(matches) != 1 {
		return nil, false
	}
	start, ok := skipFrontendSpace(html, matches[0])
	if !ok || start >= len(html) || html[start] != '{' {
		return nil, false
	}
	end, ok := balancedFrontendObject(html, start)
	if !ok {
		return nil, false
	}
	return objectProperties(html[start:end])
}
func balancedFrontendObject(source string, start int) (int, bool) {
	depth := 0
	for i := start; i < len(source); {
		switch source[i] {
		case '"', '\'', '`':
			next, ok := skipFrontendString(source, i)
			if !ok {
				return 0, false
			}
			i = next
			continue
		case '/':
			if i+1 < len(source) && (source[i+1] == '/' || source[i+1] == '*') {
				next, ok := skipFrontendComment(source, i)
				if !ok {
					return 0, false
				}
				i = next
				continue
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
			if depth < 0 {
				return 0, false
			}
		}
		i++
	}
	return 0, false
}
func objectProperties(source string) (map[string]string, bool) {
	i, ok := skipFrontendSpace(source, 0)
	if !ok || i >= len(source) || source[i] != '{' {
		return nil, false
	}
	i++
	props := map[string]string{}
	for {
		i, ok = skipFrontendSpace(source, i)
		if !ok || i >= len(source) {
			return nil, false
		}
		if source[i] == '}' {
			end, ok := skipFrontendSpace(source, i+1)
			return props, ok && end == len(source)
		}
		key, next, ok := frontendPropertyKey(source, i)
		if !ok {
			return nil, false
		}
		i = next
		i, ok = skipFrontendSpace(source, i)
		if !ok || i >= len(source) || source[i] != ':' {
			return nil, false
		}
		i, ok = skipFrontendSpace(source, i+1)
		if !ok {
			return nil, false
		}
		start := i
		end, ok := frontendValueEnd(source, i)
		if !ok {
			return nil, false
		}
		value := strings.TrimFunc(source[start:end], pythonSpace)
		if value == "" {
			return nil, false
		}
		if _, exists := props[key]; exists {
			return nil, false
		}
		props[key] = value
		i, ok = skipFrontendSpace(source, end)
		if !ok || i >= len(source) {
			return nil, false
		}
		if source[i] == ',' {
			i++
			continue
		}
		if source[i] != '}' {
			return nil, false
		}
	}
}
func identifierStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '$'
}
func frontendPropertyKey(source string, i int) (string, int, bool) {
	if i >= len(source) {
		return "", 0, false
	}
	if source[i] == '"' {
		end, ok := skipFrontendString(source, i)
		if !ok {
			return "", 0, false
		}
		var key string
		data := []byte(source[i:end])
		if strictjson.Validate(data) != nil || json.Unmarshal(data, &key) != nil {
			return "", 0, false
		}
		return key, end, true
	}
	if !identifierStart(source[i]) {
		return "", 0, false
	}
	end := i + 1
	for end < len(source) && (identifierStart(source[end]) || source[end] >= '0' && source[end] <= '9') {
		end++
	}
	return source[i:end], end, true
}
func frontendValueEnd(source string, i int) (int, bool) {
	stack := make([]byte, 0, 8)
	for i < len(source) {
		c := source[i]
		if c == '"' || c == '\'' || c == '`' {
			next, ok := skipFrontendString(source, i)
			if !ok {
				return 0, false
			}
			i = next
			continue
		}
		if c == '/' && i+1 < len(source) && (source[i+1] == '/' || source[i+1] == '*') {
			next, ok := skipFrontendComment(source, i)
			if !ok {
				return 0, false
			}
			i = next
			continue
		}
		switch c {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '(':
			stack = append(stack, ')')
		case '}', ']', ')':
			if len(stack) > 0 {
				if stack[len(stack)-1] != c {
					return 0, false
				}
				stack = stack[:len(stack)-1]
			} else if c == '}' {
				return i, true
			} else {
				return 0, false
			}
		case ',':
			if len(stack) == 0 {
				return i, true
			}
		}
		i++
	}
	return 0, false
}
func pythonSpace(r rune) bool { return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f }

// Go regexp's ASCII \b/\s are not Python's Unicode source guards. Scan the
// assignment tokens with Python-equivalent word/space boundaries; do not skip
// quoted/commented assignments or allow comments between assignment tokens.
func frontendAssignments(source string) []int {
	ends := make([]int, 0, 2)
	for cursor := 0; cursor < len(source); {
		relative := strings.Index(source[cursor:], "window")
		if relative < 0 {
			break
		}
		start := cursor + relative
		cursor = start + len("window")
		if start > 0 {
			previous, _ := utf8.DecodeLastRuneInString(source[:start])
			if previous == '_' || unicode.IsLetter(previous) || unicode.IsNumber(previous) {
				continue
			}
		}
		index := assignmentSpaceEnd(source, cursor)
		if index >= len(source) || source[index] != '.' {
			continue
		}
		index = assignmentSpaceEnd(source, index+1)
		if !strings.HasPrefix(source[index:], "config") {
			continue
		}
		index = assignmentSpaceEnd(source, index+len("config"))
		if index >= len(source) || source[index] != '=' {
			continue
		}
		ends = append(ends, index+1)
		if len(ends) == 2 {
			break
		}
	}
	return ends
}
func assignmentSpaceEnd(source string, index int) int {
	for index < len(source) {
		r, width := utf8.DecodeRuneInString(source[index:])
		if !pythonSpace(r) {
			break
		}
		index += width
	}
	return index
}
func skipFrontendSpace(source string, i int) (int, bool) {
	for i < len(source) {
		r, width := utf8.DecodeRuneInString(source[i:])
		if pythonSpace(r) {
			i += width
			continue
		}
		if source[i] == '/' && i+1 < len(source) && (source[i+1] == '/' || source[i+1] == '*') {
			next, ok := skipFrontendComment(source, i)
			if !ok {
				return 0, false
			}
			i = next
			continue
		}
		break
	}
	return i, true
}
func skipFrontendString(source string, i int) (int, bool) {
	quote := source[i]
	i++
	for i < len(source) {
		if source[i] == '\\' {
			i += 2
		} else if source[i] == quote {
			return i + 1, true
		} else {
			i++
		}
	}
	return 0, false
}
func skipFrontendComment(source string, i int) (int, bool) {
	if source[i+1] == '/' {
		// ECMAScript // comments end at LF, CR, LS or PS, not every
		// Python-space rune. Escaped spellings are still comment text.
		end := strings.IndexFunc(source[i+2:], func(r rune) bool {
			return r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029'
		})
		if end < 0 {
			return len(source), true
		}
		_, width := utf8.DecodeRuneInString(source[i+2+end:])
		return i + 2 + end + width, true
	}
	end := strings.Index(source[i+2:], "*/")
	if end < 0 {
		return 0, false
	}
	return i + 2 + end + 2, true
}

func normalizeAuthBase(value string) (string, bool) {
	result, err := NormalizeAuthBase(value)
	return result, err == nil
}
