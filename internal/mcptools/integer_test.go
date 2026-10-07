package mcptools

import (
	"strings"
	"testing"
)

func TestValidateArgumentsExactIntegerSemantics(t *testing.T) {
	accepted := []string{"0", "-0", "1.0", "10e-1", "0.001e3", "1e10000000000000000000000000000", "0e-10000000000000000000000000000", "9007199254740993", "100.00e-2"}
	for _, value := range accepted {
		raw := []byte(`{"limit":` + value + `}`)
		if err := ValidateArguments("sber_operations", raw, false); err != nil {
			t.Fatalf("exact integer %s rejected", value)
		}
	}
	rejected := []string{"1.5", "1e-1", "9007199254740993.1", "1e-10000000000000000000000000000", "0.01", "1.0000000000000001", `"1"`, "true"}
	for _, value := range rejected {
		if err := ValidateArguments("sber_operations", []byte(`{"limit":`+value+`}`), false); err == nil {
			t.Fatalf("noninteger %s accepted", value)
		}
	}
	// Large exponents must be classified lexically, without constructing powers
	// of ten, overflowing machine integers or coercing through float64.
	for _, negative := range []bool{false, true} {
		exponent := strings.Repeat("9", 20000)
		if negative {
			exponent = "-" + exponent
		}
		err := ValidateArguments("sber_operations", []byte(`{"limit":1e`+exponent+`}`), false)
		if (err == nil) == negative {
			t.Fatal("large exponent classification mismatch")
		}
	}
}
