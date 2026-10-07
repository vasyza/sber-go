// Package mcptools holds secret-free MCP argument contracts from the licensed
// source reference. It does not authenticate, read profiles, or call a bank.
package mcptools

import "encoding/json"

// Definition describes a tool, not an implemented handler. Registration policy
// is application-only and never restricts the bank session's privileges.
type Definition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Mutation    bool
}

type argument struct {
	name         string
	kind         string
	optional     bool
	defaultValue any
}

type contract struct {
	name      string
	mutation  bool
	arguments []argument
}

func sourceContracts() []contract {
	s := func(name string) argument { return argument{name: name, kind: "string"} }
	nullable := func(name string) argument { return argument{name: name, kind: "nullable_string", optional: true} }
	text := func(name, value string) argument {
		return argument{name: name, kind: "string", optional: true, defaultValue: value}
	}
	integer := func(name string, value int) argument {
		return argument{name: name, kind: "integer", optional: true, defaultValue: value}
	}
	boolean := func(name string) argument {
		return argument{name: name, kind: "boolean", optional: true, defaultValue: false}
	}
	return []contract{
		{"sber_setup_status", false, []argument{nullable("profile")}},
		{"sber_auth_start", false, []argument{text("mode", "auto"), text("profile", "default")}},
		{"sber_auth_continue", false, []argument{nullable("session_id"), integer("wait_seconds", 30), boolean("use_audio_captcha")}},
		{"sber_auth_resend_otp", false, []argument{nullable("session_id")}},
		{"sber_session_info", false, []argument{nullable("session_id"), boolean("check_live")}},
		{"sber_session_close", false, []argument{nullable("session_id")}},
		{"sber_products", false, []argument{nullable("session_id"), boolean("force_update")}},
		{"sber_operations", false, []argument{nullable("session_id"), nullable("resource"), nullable("from_date"), nullable("to_date"), integer("limit", 30), integer("max_pages", 3)}},
		{"sber_operations_page", false, []argument{nullable("session_id"), nullable("resource"), integer("offset", 0), integer("limit", 30), nullable("from_date"), nullable("to_date")}},
		{"sber_card_rename", true, []argument{s("card_id"), s("name"), nullable("session_id")}},
		{"sber_transfer_start", true, []argument{nullable("session_id")}},
		{"sber_transfer_prepare", true, []argument{s("draft_id"), s("source_id"), s("destination_id"), s("amount"), text("currency", "RUB"), text("payment_purpose", ""), nullable("session_id")}},
		{"sber_transfer_confirm", true, []argument{s("confirmation_token"), s("acknowledged_amount"), s("acknowledged_destination_id"), nullable("session_id")}},
		{"sber_transfer_resolve_uncertain", true, []argument{s("audit_id"), {name: "found", kind: "boolean"}, nullable("session_id")}},
	}
}

// Catalog returns independent descriptors in the source registration order.
// Financial descriptors are absent by default; enabling them is neither human
// approval nor authorization to execute any account-changing action.
func Catalog(allowWrites bool) []Definition {
	out := make([]Definition, 0, 14)
	for _, c := range sourceContracts() {
		if c.mutation && !allowWrites {
			continue
		}
		properties := make(map[string]any, len(c.arguments))
		required := make([]string, 0)
		for _, a := range c.arguments {
			var kind any = a.kind
			if a.kind == "nullable_string" {
				kind = []string{"string", "null"}
			}
			property := map[string]any{"type": kind}
			if a.optional {
				property["default"] = a.defaultValue
			} else {
				required = append(required, a.name)
			}
			properties[a.name] = property
		}
		// This closed schema rejects secret/unknown/case-aliased arguments. It
		// deliberately does not pretend to validate a bank workflow or consent.
		schema, _ := json.Marshal(map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false})
		out = append(out, Definition{Name: c.name, Description: c.name, InputSchema: schema, Mutation: c.mutation})
	}
	return out
}
