// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/vasyza/sber-sdk/internal/strictjson"
)

// Decimal is an immutable base-ten value. Its zero value is zero. It preserves
// fractional scale (10.50), without binary-floating-point arithmetic.
type Decimal struct {
	digits   string
	exponent int
	negative bool
}

var decimalPattern = regexp.MustCompile(`^([+-]?)([0-9]*)(?:\.([0-9]*))?(?:[eE]([+-]?[0-9]+))?$`)

// JSON number lexemes are stricter than source-compatible decimal strings:
// no surrounding whitespace, leading plus/zeros, Unicode digits or separators.
var domainJSONNumberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// ParseDecimal accepts decimal strings (including a comma decimal separator),
// validated JSON number lexemes and native numbers. String normalization does
// not apply to json.Number. Decode JSON with DecodeJSON, not float64, when the
// original numeric lexeme must be preserved.
func ParseDecimal(value any) (Decimal, error) {
	var text string
	switch v := value.(type) {
	case Decimal:
		return v, nil
	case *Decimal:
		if v == nil {
			return Decimal{}, NewParseError("amount")
		}
		return *v, nil
	case string:
		text = v
	case json.Number:
		text = string(v)
		if !domainJSONNumberPattern.MatchString(text) {
			return Decimal{}, NewParseError("amount")
		}
	case int:
		text = strconv.FormatInt(int64(v), 10)
	case int8:
		text = strconv.FormatInt(int64(v), 10)
	case int16:
		text = strconv.FormatInt(int64(v), 10)
	case int32:
		text = strconv.FormatInt(int64(v), 10)
	case int64:
		text = strconv.FormatInt(v, 10)
	case uint:
		text = strconv.FormatUint(uint64(v), 10)
	case uint8:
		text = strconv.FormatUint(uint64(v), 10)
	case uint16:
		text = strconv.FormatUint(uint64(v), 10)
	case uint32:
		text = strconv.FormatUint(uint64(v), 10)
	case uint64:
		text = strconv.FormatUint(v, 10)
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return Decimal{}, NewParseError("amount")
		}
		text = strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Decimal{}, NewParseError("amount")
		}
		text = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return Decimal{}, NewParseError("amount")
	}
	text = strings.Map(func(r rune) rune {
		if r == '_' {
			return -1
		}
		if r == ',' {
			return '.'
		}
		if n, ok := domainDigitValue(r); ok {
			return rune("0123456789"[n])
		}
		return r
	}, strings.TrimSpace(text))
	m := decimalPattern.FindStringSubmatch(text)
	if m == nil || m[2]+m[3] == "" {
		return Decimal{}, NewParseError("amount")
	}
	exp := 0
	if m[4] != "" {
		n, err := strconv.Atoi(m[4])
		if err != nil {
			return Decimal{}, NewParseError("amount")
		}
		exp = n
	}
	// An overflowing exponent must not wrap into a different monetary value.
	if exp < -int(^uint(0)>>1)+len(m[3]) {
		return Decimal{}, NewParseError("amount")
	}
	exp -= len(m[3])
	digits := strings.TrimLeft(m[2]+m[3], "0")
	if digits == "" {
		digits = "0"
	}
	if exp > int(^uint(0)>>1)-len(digits) {
		return Decimal{}, NewParseError("amount")
	}
	return Decimal{digits: digits, exponent: exp, negative: m[1] == "-"}, nil
}

func (d Decimal) String() string {
	digits := d.digits
	if digits == "" {
		digits = "0"
	}
	sign := ""
	if d.negative {
		sign = "-"
	}
	adjusted := d.exponent + len(digits) - 1
	if d.exponent > 0 || adjusted < -6 {
		mantissa := digits[:1]
		if len(digits) > 1 {
			mantissa += "." + digits[1:]
		}
		e := strconv.Itoa(adjusted)
		if adjusted >= 0 {
			e = "+" + e
		}
		return sign + mantissa + "E" + e
	}
	point := len(digits) + d.exponent
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + digits
	}
	if point < len(digits) {
		return sign + digits[:point] + "." + digits[point:]
	}
	return sign + digits
}

func (d Decimal) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Decimal) UnmarshalJSON(data []byte) error {
	if !json.Valid(data) {
		return NewParseError("amount")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return NewParseError("amount")
	}
	parsed, err := ParseDecimal(v)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func (d Decimal) GoString() string { return d.String() }

// Sign returns -1, 0 or +1 without converting to floating point.
func (d Decimal) Sign() int {
	if d.digits == "" || d.digits == "0" {
		return 0
	}
	if d.negative {
		return -1
	}
	return 1
}

func (d Decimal) Abs() Decimal { d.negative = false; return d }

func (d Decimal) Neg() Decimal { d.negative = !d.negative; return d }

// Money deliberately exports financial data as exact decimal strings. It does
// not contain credentials and is not replaced by an unusable redacted blob.
// The wrapper is mutable Go data, unlike Python's frozen Money dataclass;
// Decimal itself has immutable, unexported base-ten components.
type Money struct {
	Amount   Decimal `json:"amount"`
	Currency string  `json:"currency"`
}

func (m *Money) UnmarshalJSON(data []byte) error {
	// Validate before decoding: maps otherwise overwrite duplicate evidence,
	// and encoding/json can silently replace malformed Unicode identifiers.
	if strictjson.Validate(data) != nil {
		return NewParseError("money")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return NewParseError("money")
	}
	if raw == nil {
		return NewParseError("money")
	}
	value, err := ParseMoney(raw)
	if err != nil {
		return err
	}
	if value == nil {
		return NewParseError("amount")
	}
	*m = *value
	return nil
}

// ParseMoney returns nil for nil money objects or an absent amount key. A
// nonnil value that is not a JSON object, or an explicit invalid/null amount,
// is a hard error. Optional parser metadata must use its separate soft path.
// currencyCode:null falls through to currency.code.
func ParseMoney(value any) (*Money, error) {
	if value == nil {
		return nil, nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, NewParseError("money")
	}
	if raw == nil {
		return nil, nil
	}
	amount, present := raw["amount"]
	if !present {
		return nil, nil
	}
	d, err := ParseDecimal(amount)
	if err != nil {
		return nil, err
	}
	currency := raw["currencyCode"]
	if !domainTruthy(currency) {
		currency = raw["currency"]
	}
	if obj, ok := currency.(map[string]any); ok {
		currency = obj["code"]
	}
	c, _ := currency.(string)
	return &Money{Amount: d, Currency: c}, nil
}

func domainTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		d, err := ParseDecimal(x)
		return err != nil || d.Sign() != 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Array, reflect.Map, reflect.Slice:
		return rv.Len() != 0
	case reflect.Pointer, reflect.Interface:
		return !rv.IsNil()
	default:
		return true
	}
}

// Decimal's reference parser accepts Unicode decimal digits, not just ASCII.
func domainDigitValue(r rune) (int, bool) {
	if r < 0 || r > unicode.MaxRune {
		return 0, false
	}
	if r >= '0' && r <= '9' {
		return int(r - '0'), true
	}
	n := uint32(r)
	for _, span := range unicode.Nd.R16 {
		if n >= uint32(span.Lo) && n <= uint32(span.Hi) && (n-uint32(span.Lo))%uint32(span.Stride) == 0 {
			return int((n - uint32(span.Lo)) / uint32(span.Stride) % 10), true
		}
	}
	for _, span := range unicode.Nd.R32 {
		if n >= span.Lo && n <= span.Hi && (n-span.Lo)%span.Stride == 0 {
			return int((n - span.Lo) / span.Stride % 10), true
		}
	}
	return 0, false
}

// Keep fmt's ordinary Money formatting useful, like the reference dataclass.
var _ fmt.Stringer = Decimal{}
