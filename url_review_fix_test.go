package sber

import "testing"

func TestReviewRuntimeOriginDecodedKeyCommentConflicts(t *testing.T) {
	for _, discover := range []struct {
		key   string
		parse func(string) (string, bool)
	}{{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML}} {
		for _, comment := range []string{"/* synthetic */", "// synthetic\n"} {
			t.Run(discover.key+"/"+comment, func(t *testing.T) {
				text := `{"` + discover.key + `":"https://online.sberbank.ru","\u0075` + discover.key[1:] + `"` + comment + `:"https://example.org"}`
				if value, ok := discover.parse(text); ok || value != "" {
					t.Fatal("comment between decoded property key and colon hid an origin conflict")
				}
			})
		}
	}
}

func TestReviewRuntimeOriginDecodedKeyConflicts(t *testing.T) {
	for _, discover := range []struct {
		key   string
		parse func(string) (string, bool)
	}{{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML}} {
		raw := `"` + discover.key + `":"https://online.sberbank.ru"`
		escapedKey := `"\u0075` + discover.key[1:] + `"`
		escapedSafe := escapedKey + `:"https://online.sberbank.ru/"`
		escapedUnsafe := escapedKey + `:"https://example.org"`
		for _, tc := range []struct {
			name, text string
			accepted   bool
		}{
			{"escaped_only", `{` + escapedSafe + `}`, true},
			{"matching_decoded_repeats", `{` + raw + `,` + escapedSafe + `}`, true},
			{"unsafe_decoded_repeat_last", `{` + raw + `,` + escapedUnsafe + `}`, false},
			{"unsafe_decoded_repeat_first", `{` + escapedUnsafe + `,` + raw + `}`, false},
			{"decoded_type_conflict", `{` + raw + `,` + escapedKey + `:null}`, false},
			{"decoded_origin_conflict", `{` + raw + `,` + escapedKey + `:"https://different.online.sberbank.ru"}`, false},
			{"decoded_expression_conflict", `{` + raw + `,` + escapedSafe + ` + ".evil.example"}`, false},
			{"escaped_backslash_is_not_the_key", `{` + raw + `,"\\u0075` + discover.key[1:] + `":"https://example.org"}`, true},
		} {
			t.Run(discover.key+"/"+tc.name, func(t *testing.T) {
				value, ok := discover.parse(tc.text)
				if ok != tc.accepted || (!ok && value != "") || (ok && value != "https://online.sberbank.ru") {
					t.Fatal("runtime discovery failed decoded-key conflict policy")
				}
			})
		}
	}
}

func TestReviewRuntimeOriginRejectsExpressionSuffix(t *testing.T) {
	for _, discover := range []struct {
		key   string
		parse func(string) (string, bool)
	}{{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML}} {
		for _, tc := range []struct{ name, suffix string }{
			{"external_suffix", ` + ".evil.example"`},
			{"path_suffix", ` + "/CSAFront"`},
			{"call", `("ignored")`},
			{"method", `.concat(".evil.example")`},
			{"index", `[0]`},
			{"comment_then_suffix", ` /* synthetic comment */ + ".evil.example"`},
			{"newline_then_suffix", "\n + \".evil.example\""},
		} {
			t.Run(discover.key+"/"+tc.name, func(t *testing.T) {
				text := `{"` + discover.key + `":"https://online.sberbank.ru"` + tc.suffix + `}`
				if value, ok := discover.parse(text); ok || value != "" {
					t.Fatal("runtime origin accepted a literal prefix of an expression")
				}
			})
		}
	}
}
