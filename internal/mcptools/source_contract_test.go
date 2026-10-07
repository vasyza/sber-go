package mcptools

import (
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
)

func TestCatalogAllSourceArgumentContracts(t *testing.T) {
	var reference struct {
		ToolCount int `json:"tool_count"`
		Tools     []struct {
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
	if err = json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	definitions := Catalog(true)
	if len(definitions) != reference.ToolCount {
		t.Fatalf("got %d tools, want %d", len(definitions), reference.ToolCount)
	}
	writes := map[string]bool{"sber_card_rename": true, "sber_transfer_start": true, "sber_transfer_prepare": true, "sber_transfer_confirm": true, "sber_transfer_resolve_uncertain": true}
	for i, source := range reference.Tools {
		t.Run(source.Name, func(t *testing.T) {
			d := definitions[i]
			if d.Name != source.Name || d.Description != source.Name || d.Mutation != writes[source.Name] {
				t.Fatal("source registration mismatch")
			}
			var actual map[string]any
			if err := json.Unmarshal(d.InputSchema, &actual); err != nil {
				t.Fatal(err)
			}
			properties := map[string]any{}
			required := []any{}
			firstDefault := len(source.Arguments) - len(source.Defaults)
			for j, arg := range source.Arguments {
				var kind any
				switch source.Annotations[arg] {
				case "str":
					kind = "string"
				case "str | None":
					kind = []any{"string", "null"}
				case "int":
					kind = "integer"
				case "bool":
					kind = "boolean"
				default:
					t.Fatal("unaccounted source type")
				}
				property := map[string]any{"type": kind}
				if j >= firstDefault {
					s := source.Defaults[j-firstDefault]
					var value any
					switch s {
					case "None":
						value = nil
					case "False":
						value = false
					case "True":
						value = true
					default:
						if len(s) > 1 && s[0] == '\'' {
							value = s[1 : len(s)-1]
						} else {
							n, e := strconv.Atoi(s)
							if e != nil {
								t.Fatal(e)
							}
							value = float64(n)
						}
					}
					property["default"] = value
				} else {
					required = append(required, arg)
				}
				properties[arg] = property
			}
			want := map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("schema mismatch: got %s", d.InputSchema)
			}
		})
	}
}
