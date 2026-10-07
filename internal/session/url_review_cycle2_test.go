package session

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"unicode/utf8"
)

// Minimal end-to-end tracer for the remaining cycle-1 comment-closure finding.
// All source text is synthetic; discovery must never execute it or fetch a URL.
func TestReviewCycle2RuntimeCommentCRConflictTracer(t *testing.T) {
	text := `{"ufsHost":"https://online.sberbank.ru","\u0075fsHost"// synthetic` + "\r" + `:"https://example.org"}`
	if origin, ok := UFSHostFromAppShell(text); ok || origin != "" {
		t.Fatalf("CR-terminated comment hid decoded-key conflict: got (%q, %t), want (\"\", false)", origin, ok)
	}
}

func TestReviewCycle2RuntimeCommentPSSuffixTracer(t *testing.T) {
	text := `{"ufs.block.root.url":"https://online.sberbank.ru"// synthetic` + "\u2029" + ` + ".evil.example"}`
	if origin, ok := APIBaseFromMainHTML(text); ok || origin != "" {
		t.Fatalf("PS-terminated comment hid expression suffix: got (%q, %t), want (\"\", false)", origin, ok)
	}
}

func TestReviewCycle2RuntimeCommentLFCRLFControls(t *testing.T) {
	for _, discover := range []struct {
		key   string
		parse func(string) (string, bool)
	}{{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML}} {
		for _, line := range []struct{ name, text string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			for _, shape := range []string{"key_conflict", "value_expression"} {
				t.Run(discover.key+"/"+line.name+"/"+shape, func(t *testing.T) {
					text := `{"` + discover.key + `":"https://online.sberbank.ru"// synthetic` + line.text + ` + ".evil.example"}`
					if shape == "key_conflict" {
						text = `{"` + discover.key + `":"https://online.sberbank.ru","\u0075` + discover.key[1:] + `"// synthetic` + line.text + `:"https://example.org"}`
					}
					if origin, ok := discover.parse(text); ok || origin != "" {
						t.Fatalf("baseline line-comment control accepted: got (%q, %t), want (\"\", false)", origin, ok)
					}
				})
			}
		}
	}
}

var reviewCycle2LineEnds = []struct{ name, text string }{
	{"LF", "\n"}, {"CR", "\r"}, {"CRLF", "\r\n"}, {"LS", "\u2028"}, {"PS", "\u2029"},
}

var reviewCycle2Discovery = []struct {
	key   string
	parse func(string) (string, bool)
}{
	{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML},
}

func reviewCycle2AssertOrigin(t *testing.T, parse func(string) (string, bool), text, want string, accepted bool) {
	t.Helper()
	t.Logf("synthetic_fixture_sha256=%x bytes=%d", sha256.Sum256([]byte(text)), len(text))
	origin, ok := parse(text)
	if origin != want || ok != accepted {
		t.Fatalf("runtime discovery got (%q, %t), want (%q, %t)", origin, ok, want, accepted)
	}
}

// These matrices strengthen the single scanner repair, not a new functional
// tranche. Each LF/CR/CRLF/LS/PS is an actual code point, not escape text.
func TestReviewCycle2RuntimeLineCommentConflictMatrix(t *testing.T) {
	for _, discover := range reviewCycle2Discovery {
		rawKey := `"` + discover.key + `"`
		for _, line := range reviewCycle2LineEnds {
			for _, spelling := range []struct{ name, key string }{
				{"ordinary", rawKey}, {"escaped_alias", `"\u0075` + discover.key[1:] + `"`},
			} {
				for _, placement := range []string{"key_colon", "colon_value", "literal_delimiter"} {
					for _, tc := range []struct {
						name, value, want string
						accepted          bool
					}{
						{"equivalent_safe_repeat", `"https:\/\/online.sberbank.ru\/"`, "https://online.sberbank.ru", true},
						{"external_conflict", `"https://example.org"`, "", false},
						{"online_host_conflict", `"https://different.online.sberbank.ru"`, "", false},
						{"type_conflict", `null`, "", false},
						{"expression_conflict", `"https://online.sberbank.ru" + ".evil.example"`, "", false},
					} {
						t.Run(discover.key+"/"+line.name+"/"+spelling.name+"/"+placement+"/"+tc.name, func(t *testing.T) {
							comment := "// synthetic } { , :" + line.text
							first := rawKey + `:"https://online.sberbank.ru"`
							second := spelling.key + ":" + tc.value
							if placement == "key_colon" {
								second = spelling.key + comment + ":" + tc.value
							}
							if placement == "colon_value" {
								second = spelling.key + ":" + comment + tc.value
							}
							if placement == "literal_delimiter" {
								first += comment
							}
							reviewCycle2AssertOrigin(t, discover.parse, "{"+first+","+second+"}", tc.want, tc.accepted)
						})
					}
				}
			}
		}
	}
}

func TestReviewCycle2RuntimeLineCommentSuffixMatrix(t *testing.T) {
	for _, discover := range reviewCycle2Discovery {
		for _, line := range reviewCycle2LineEnds {
			for _, spelling := range []struct{ name, key string }{
				{"ordinary", `"` + discover.key + `"`}, {"escaped_alias", `"\u0075` + discover.key[1:] + `"`},
			} {
				for _, suffix := range []struct{ name, text string }{
					{"external_concat", ` + ".evil.example"`},
					{"equivalent_concat_is_still_expression", ` + ""`},
					{"path_concat", ` + "/CSAFront"`},
					{"call", `("ignored")`},
					{"method", `.concat("")`},
					{"index", `[0]`},
					{"conditional", ` ? "https://example.org" : "https://online.sberbank.ru"`},
				} {
					t.Run(discover.key+"/"+line.name+"/"+spelling.name+"/"+suffix.name, func(t *testing.T) {
						// Both validation seams must inspect the active source after //.
						text := "{" + spelling.key + "// synthetic key" + line.text + `:"https://online.sberbank.ru"// synthetic literal` + line.text + suffix.text + "}"
						reviewCycle2AssertOrigin(t, discover.parse, text, "", false)
					})
				}
			}
		}
	}
}

func TestReviewCycle2RuntimeCommentsRetainExactLexicalBoundary(t *testing.T) {
	for _, discover := range reviewCycle2Discovery {
		rawKey := `"` + discover.key + `"`
		escapedKey := `"\u0075` + discover.key[1:] + `"`
		base := rawKey + `:"https://online.sberbank.ru"`
		for _, notEnd := range []struct{ name, text string }{
			{"VT", "\v"}, {"FF", "\f"}, {"NEL", "\u0085"},
			{"escaped_LF", `\n`}, {"escaped_CR", `\r`},
			{"escaped_LS", `\u2028`}, {"escaped_PS", `\u2029`},
		} {
			for _, shape := range []string{"commented_conflict", "commented_suffix"} {
				t.Run(discover.key+"/"+notEnd.name+"/"+shape, func(t *testing.T) {
					text := "{" + base + "," + escapedKey + "// synthetic" + notEnd.text + `:"https://example.org"}`
					if shape == "commented_suffix" {
						text = "{" + base + "// synthetic" + notEnd.text + ` + ".evil.example"}`
					}
					reviewCycle2AssertOrigin(t, discover.parse, text, "https://online.sberbank.ru", true)
				})
			}
		}
		for _, tc := range []struct {
			name, text, want string
			accepted         bool
		}{
			{"block_key_safe", "{" + rawKey + "/* synthetic\r\n\u2028\u2029 } */:" + `"https://online.sberbank.ru/"` + "}", "https://online.sberbank.ru", true},
			{"block_after_literal_safe", "{" + base + "/* synthetic\r\n\u2028\u2029 } */}", "https://online.sberbank.ru", true},
			{"block_key_conflict", "{" + base + "," + escapedKey + `/* synthetic */:"https://example.org"}`, "", false},
			{"block_then_suffix", "{" + base + `/* synthetic */ + ""}`, "", false},
			{"unclosed_block_before_colon", "{" + base + "," + escapedKey + "/* synthetic", "", false},
			{"unclosed_block_after_literal", "{" + base + "/* synthetic", "", false},
			{"line_EOF_after_literal", base + "// synthetic", "https://online.sberbank.ru", true},
			{"line_EOF_before_conflict_colon", "{" + base + "," + escapedKey + `// synthetic :"https://example.org"}`, "https://online.sberbank.ru", true},
			{"line_EOF_only_key", escapedKey + "// synthetic", "", false},
			{"escaped_backslash_not_alias", "{" + base + `,"\\u0075` + discover.key[1:] + `":"https://example.org"}`, "https://online.sberbank.ru", true},
			{"valid_surrogate_key", `{"\ud83d\ude00":true,` + base + "}", "https://online.sberbank.ru", true},
			{"invalid_surrogate_key", `{"\ud800":true,` + base + "}", "", false},
			{"unterminated_literal", rawKey + `:"https://online.sberbank.ru`, "", false},
			{"trailing_string_escape", rawKey + `:"https://online.sberbank.ru\`, "", false},
			{"raw_LF_in_literal", rawKey + `:"https://online.sberbank.ru` + "\n" + `"`, "", false},
			{"invalid_UTF8", base + string([]byte{0xff}), "", false},
		} {
			t.Run(discover.key+"/"+tc.name, func(t *testing.T) { reviewCycle2AssertOrigin(t, discover.parse, tc.text, tc.want, tc.accepted) })
		}
	}
}

func TestReviewCycle2RuntimeCommentDocumentBounds(t *testing.T) {
	for _, discover := range reviewCycle2Discovery {
		base := `"` + discover.key + `":"https://online.sberbank.ru"// synthetic`
		bounded := strings.Repeat(" ", MaxFrontendHTMLCharacters-utf8.RuneCountInString(base)) + base
		t.Run(discover.key+"/at_limit", func(t *testing.T) {
			reviewCycle2AssertOrigin(t, discover.parse, bounded, "https://online.sberbank.ru", true)
		})
		t.Run(discover.key+"/past_limit", func(t *testing.T) { reviewCycle2AssertOrigin(t, discover.parse, " "+bounded, "", false) })
	}
}

func FuzzReviewCycle2RuntimeCommentTerminators(f *testing.F) {
	for key := uint8(0); key < 2; key++ {
		for line := uint8(0); line < 5; line++ {
			for _, escaped := range []bool{false, true} {
				for kind := uint8(0); kind < 3; kind++ {
					f.Add(key, line, escaped, kind, "synthetic")
				}
			}
		}
	}
	f.Fuzz(func(t *testing.T, key, line uint8, escaped bool, kind uint8, noise string) {
		// Encode bounded generated comment bytes, preserving an exact static oracle.
		// No decode/marshal oracle can erase a conflicting original spelling.
		if len(noise) > 64 {
			noise = noise[:64]
		}
		discover := reviewCycle2Discovery[int(key)%len(reviewCycle2Discovery)]
		terminator := reviewCycle2LineEnds[int(line)%len(reviewCycle2LineEnds)].text
		name := `"` + discover.key + `"`
		if escaped {
			name = `"\u0075` + discover.key[1:] + `"`
		}
		comment := "// " + hex.EncodeToString([]byte(noise)) + terminator
		text := "{" + name + `:"https://online.sberbank.ru"` + comment + ` + ".evil.example"}`
		want, accepted := "", false
		switch kind % 3 {
		case 0:
			text = `{"` + discover.key + `":"https://online.sberbank.ru",` + name + comment + `:"https://example.org"}`
		case 2:
			text = `{"` + discover.key + `":"https://online.sberbank.ru"` + comment + "," + name + comment + `:"https:\/\/online.sberbank.ru\/"}`
			want, accepted = "https://online.sberbank.ru", true
		}
		if origin, ok := discover.parse(text); origin != want || ok != accepted {
			t.Fatalf("static comment oracle got (%q, %t), want (%q, %t)", origin, ok, want, accepted)
		}
	})
}
