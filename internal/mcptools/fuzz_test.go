package mcptools

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/vasyza/sber-go/internal/strictjson"
)

func FuzzArgumentsOriginalIntegrity(f *testing.F) {
	for _, raw := range [][]byte{[]byte(`{}`), []byte(`{"limit":1.0}`), []byte(`{"resource":null}`), []byte(`{"limit":1,"\u006cimit":2}`), []byte(`{"resource":"\ud800"}`), {0xff}} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := append([]byte(nil), raw...)
		err := ValidateArguments("sber_operations", raw, false)
		if !bytes.Equal(before, raw) {
			t.Fatal("caller bytes mutated")
		}
		if err != nil && err != ErrInvalidArguments {
			t.Fatal("nonstatic unexpected failure")
		}
		if err == nil {
			if len(raw) > MaximumArgumentBytes || strictjson.Validate(raw) != nil {
				t.Fatal("accepted invalid original document")
			}
			var object map[string]json.RawMessage
			if json.Unmarshal(raw, &object) != nil || object == nil {
				t.Fatal("accepted nonobject")
			}
			allowed := map[string]bool{"session_id": true, "resource": true, "from_date": true, "to_date": true, "limit": true, "max_pages": true}
			for key := range object {
				if !allowed[key] {
					t.Fatal("accepted unregistered argument")
				}
			}
		}
		// Negative controls retain the exact original spelling. No permissive
		// decode/marshal oracle can erase duplicates or repair these bytes first.
		for _, bad := range [][]byte{[]byte(`{"limit":1,"\u006cimit":2}`), []byte(`{"resource":"\ud800"}`), append([]byte(`{"resource":"`), []byte{0xff, '"', '}'}...)} {
			if ValidateArguments("sber_operations", bad, false) != ErrInvalidArguments {
				t.Fatal("original evidence gate missing")
			}
		}
	})
}

func FuzzExactIntegerAgainstRationalOracle(f *testing.F) {
	for _, value := range []string{"0", "-0.000", "1.0", "1.5", "10e-1", "100.00e-2", "9007199254740993.1", "1e000000000000000000000000001"} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		// Keep the independent big.Rat oracle bounded. Larger exponents are
		// tested separately by exact lexical controls, never materialized here.
		token := strings.Trim(value, " 	\r\n") // Strip only JSON framing whitespace for the rational oracle.
		if len(token) == 0 || len(token) > 2048 || (token[0] != '-' && (token[0] < '0' || token[0] > '9')) || !json.Valid([]byte(value)) {
			return
		}
		if i := strings.IndexAny(token, "eE"); i >= 0 {
			exponent, err := strconv.ParseInt(token[i+1:], 10, 32)
			if err != nil || exponent < -1000 || exponent > 1000 {
				return
			}
		}
		oracle, ok := new(big.Rat).SetString(token)
		if !ok {
			t.Fatal("valid bounded JSON number not recognized by oracle")
		}
		err := ValidateArguments("sber_operations", []byte(`{"limit":`+value+`}`), false)
		if (err == nil) != oracle.IsInt() {
			t.Fatal("integer classification disagrees with exact rational oracle")
		}
	})
}
