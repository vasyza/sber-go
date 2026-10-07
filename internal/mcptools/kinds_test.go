package mcptools

import (
	"encoding/json"
	"os"
	"testing"
)

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
