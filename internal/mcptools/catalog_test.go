package mcptools

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func TestCatalogReadOnlyRegistration(t *testing.T) {
	want := []string{"sber_setup_status", "sber_auth_start", "sber_auth_continue", "sber_auth_resend_otp", "sber_session_info", "sber_session_close", "sber_products", "sber_operations", "sber_operations_page"}
	definitions := Catalog(false)
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Mutation {
			t.Fatal("mutation registered by default")
		}
		names = append(names, definition.Name)
	}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("catalog %v, want %v", names, want)
	}
}

func TestCatalogDefensiveCopiesConcurrentUse(t *testing.T) {
	baseline := Catalog(false)
	want := append([]byte(nil), baseline[0].InputSchema...)
	var wait sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			definitions := Catalog(true)
			definitions[0].Name = "caller-modified"
			definitions[0].InputSchema[0] = '!'
			fresh := Catalog(false)
			if fresh[0].Name != "sber_setup_status" || !bytes.Equal(fresh[0].InputSchema, want) {
				t.Error("caller mutation leaked into later catalog")
			}
			if err := ValidateArguments("sber_operations", []byte(`{"limit":10e-1}`), false); err != nil {
				t.Error("concurrent validation failed")
			}
		}()
	}
	wait.Wait()
	if !bytes.Equal(baseline[0].InputSchema, want) {
		t.Fatal("prior snapshot changed")
	}
}

func TestValidateArgumentsUnregisteredToolCannotBypassPolicy(t *testing.T) {
	for _, name := range []string{"sber_card_rename", "sber_transfer_start", "sber_transfer_prepare", "sber_transfer_confirm", "sber_transfer_resolve_uncertain", "sber_other", "SBER_products"} {
		if err := ValidateArguments(name, []byte(`{}`), false); err == nil || err.Error() != "MCP tool unavailable" {
			t.Fatalf("unregistered tool %s reachable or wrong static error", name)
		}
	}
	if err := ValidateArguments("sber_transfer_start", []byte(`{}`), true); err != nil {
		t.Fatal(err)
	}
}

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
