package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

// Deviceprint is an explicitly accessed device identity. Ordinary fmt, logs and
// JSON redact it. Value is the caller's deliberate wire/persistence access.
// The immutable backing string is indirect: fmt's unsupported-verb diagnostics
// bypass Formatter but render a nested pointer rather than its secret contents.
// Copies retain the same identity; compare Value() for content equality between
// independently constructed values. The zero value deliberately reads empty.
type Deviceprint struct{ value *string }

func (d Deviceprint) Value() string {
	if d.value == nil {
		return ""
	}
	return *d.value
}
func (d Deviceprint) String() string               { return "Deviceprint(<redacted>)" }
func (d Deviceprint) GoString() string             { return d.String() }
func (d Deviceprint) Format(f fmt.State, v rune)   { sdkErrs.FormatError(f, d.String()) }
func (d Deviceprint) MarshalJSON() ([]byte, error) { return json.Marshal("<redacted>") }

// GenerateDeviceprint reproduces the audited Python deviceprint.py browser
// layout and ordered fields. These are synthetic data, not observed browser
// headers, protection cookies, a supported-browser claim or bank verification.
// Generate once and explicitly retain the identity when reuse is intended.
func GenerateDeviceprint() (Deviceprint, error) { return generateDeviceprint(rand.Reader) }
func generateDeviceprint(entropy io.Reader) (Deviceprint, error) {
	raw := make([]byte, 64)
	if entropy == nil {
		return Deviceprint{}, &sdkErrs.MissingSession{Message: "device identity generation failed"}
	}
	if _, err := io.ReadFull(entropy, raw); err != nil {
		return Deviceprint{}, &sdkErrs.MissingSession{Message: "device identity generation failed"}
	}
	fixed := "version=5.3.0&os=Windows&osVersion=10.0&browser=Chrome&browserVersion=146.0.0.0&platform=Win32&screen=1920x1080&colorDepth=24&timezone=-180&language=ru-RU&cpuCores=8"
	var result strings.Builder
	result.WriteString(fixed)
	offset := 0
	for _, field := range []struct {
		name string
		size int
	}{{"canvas", 16}, {"webgl", 16}, {"fonts", 8}, {"audio", 8}} {
		result.WriteByte('&')
		result.WriteString(field.name)
		result.WriteByte('=')
		result.WriteString(hex.EncodeToString(raw[offset : offset+field.size]))
		offset += field.size
	}
	uuid := raw[offset:]
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(uuid)
	result.WriteString("&uuid=")
	result.WriteString(encoded[:8])
	result.WriteByte('-')
	result.WriteString(encoded[8:12])
	result.WriteByte('-')
	result.WriteString(encoded[12:16])
	result.WriteByte('-')
	result.WriteString(encoded[16:20])
	result.WriteByte('-')
	result.WriteString(encoded[20:])
	value := result.String()
	return Deviceprint{value: &value}, nil
}

// GenerateAntifraudDeviceprint implements urllib.parse.quote(source,safe="").
// Omitted input mints an independent identity; explicit empty input stays empty.
// Captured arrays/objects/hash extras are preserved byte-for-byte, not parsed.
// No policy for sending financial mutations is granted by generating this value.
func GenerateAntifraudDeviceprint(source ...string) (Deviceprint, error) {
	if len(source) > 1 {
		return Deviceprint{}, &sdkErrs.MissingSession{Message: "invalid device identity source"}
	}
	var value string
	if len(source) == 0 {
		generated, err := GenerateDeviceprint()
		if err != nil {
			return Deviceprint{}, err
		}
		value = generated.Value()
	} else {
		value = source[0]
	}
	if !utf8.ValidString(value) {
		return Deviceprint{}, &sdkErrs.MissingSession{Message: "invalid device identity source"}
	}
	if value == "" {
		return Deviceprint{}, nil
	}
	var encoded strings.Builder
	const alphabet = "0123456789ABCDEF"
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.IndexByte("_.-~", b) >= 0 {
			encoded.WriteByte(b)
		} else {
			encoded.WriteByte('%')
			encoded.WriteByte(alphabet[b>>4])
			encoded.WriteByte(alphabet[b&0x0f])
		}
	}
	result := encoded.String()
	return Deviceprint{value: &result}, nil
}
