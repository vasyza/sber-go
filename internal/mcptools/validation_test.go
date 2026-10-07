package mcptools

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestValidateArgumentsSizeBound(t *testing.T) {
	const maximum = 1 << 20
	prefix, suffix := `{"profile":"`, `"}`
	atLimit := []byte(prefix + strings.Repeat("x", maximum-len(prefix)-len(suffix)) + suffix)
	if len(atLimit) != maximum {
		t.Fatal("invalid exact-boundary fixture")
	}
	if err := ValidateArguments("sber_auth_start", atLimit, false); err != nil {
		t.Fatal("exact-boundary schema-valid arguments rejected")
	}
	tooLarge := []byte(prefix + strings.Repeat("x", maximum-len(prefix)-len(suffix)+1) + suffix)
	if err := ValidateArguments("sber_auth_start", tooLarge, false); err != ErrInvalidArguments {
		t.Fatal("oversized argument document accepted or error was nonstatic")
	}
}

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

func TestValidateArgumentsSourceKindsAndRequired(t *testing.T) {
	var source struct {
		Tools []struct {
			Name        string            `json:"name"`
			Arguments   []string          `json:"arguments"`
			Annotations map[string]string `json:"annotations"`
			Defaults    []string          `json:"defaults"`
		} `json:"tools"`
	}
	data, err := os.ReadFile("testdata/reference-tools.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	for _, tool := range source.Tools {
		t.Run(tool.Name, func(t *testing.T) {
			values := map[string]any{}
			for _, arg := range tool.Arguments {
				switch tool.Annotations[arg] {
				case "str", "str | None":
					values[arg] = "synthetic"
				case "bool":
					values[arg] = false
				case "int":
					values[arg] = json.Number("9007199254740993")
				default:
					t.Fatal("unhandled source annotation")
				}
			}
			valid, _ := json.Marshal(values)
			if err := ValidateArguments(tool.Name, valid, true); err != nil {
				t.Fatalf("valid source kinds rejected: %v", err)
			}
			for index, arg := range tool.Arguments {
				original := values[arg]
				values[arg] = []any{}
				raw, _ := json.Marshal(values)
				if err := ValidateArguments(tool.Name, raw, true); err == nil {
					t.Fatalf("array accepted for %s", arg)
				}
				values[arg] = nil
				raw, _ = json.Marshal(values)
				err := ValidateArguments(tool.Name, raw, true)
				if (err == nil) != (tool.Annotations[arg] == "str | None") {
					t.Fatalf("nullability mismatch for %s", arg)
				}
				delete(values, arg)
				raw, _ = json.Marshal(values)
				err = ValidateArguments(tool.Name, raw, true)
				optional := index >= len(tool.Arguments)-len(tool.Defaults)
				if (err == nil) != optional {
					t.Fatalf("requiredness mismatch for %s", arg)
				}
				values[arg] = original
			}
		})
	}
}

func TestValidateArgumentsOriginalIntegrity(t *testing.T) {
	invalid := [][]byte{
		[]byte(`{"password":"synthetic-marker"}`),
		[]byte(`{"Profile":"default"}`),
		[]byte(`{"profile":"a","pro\u0066ile":"b"}`),
		[]byte(`{"profile":"\ud800"}`),
		append(append([]byte(`{"profile":"`), 0xff), []byte(`"}`)...),
		[]byte(`{} {}`), []byte(`[]`), []byte(`null`),
	}
	for _, raw := range invalid {
		before := append([]byte(nil), raw...)
		if err := ValidateArguments("sber_auth_start", raw, false); err == nil || !strings.Contains(err.Error(), "invalid MCP arguments") {
			t.Fatal("malformed arguments were accepted or error was nonstatic")
		}
		if !bytes.Equal(raw, before) {
			t.Fatal("arguments mutated")
		}
	}
	if err := ValidateArguments("sber_auth_start", []byte(`{"profile":"default"}`), false); err != nil {
		t.Fatal(err)
	}
}
