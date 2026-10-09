package session

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
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

func TestRuntimeOriginDiscovery(t *testing.T) {
	for _, discover := range []struct {
		key   string
		parse func(string) (string, bool)
	}{
		{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML},
	} {
		literal := `"` + discover.key + `": "https:\/\/web-fixture.online.sberbank.ru\/"`
		for _, text := range []string{"<script>{" + literal + "}</script>", literal + "," + literal, strings.ReplaceAll(literal, `\/`, `/`), strings.Replace(literal, `": "`, "\"\u00a0:\u2003\"", 1)} {
			value, ok := discover.parse(text)
			if !ok || value != "https://web-fixture.online.sberbank.ru" {
				t.Fatal("valid runtime origin not extracted")
			}
		}
		for _, text := range []string{"", `"` + discover.key + `": compute()`, literal + `,"` + discover.key + `": null`, literal + `,"` + discover.key + `": "https://different.online.sberbank.ru"`,
			`"` + discover.key + `": "https://example.org"`,
			`"` + discover.key + `": "https://user@online.sberbank.ru"`,
			`"` + discover.key + `": "http://online.sberbank.ru"`,
			`"` + discover.key + `": "https://online.sberbank.ru:444"`,
			`"` + discover.key + `": "https://online.sberbank.ru/CSAFront"`,
			`"` + discover.key + `": "https://online.sberbank.ru/?query=secret"`,
			`"` + discover.key + `": "https://online.sberbank.ru/#fragment"`,
			`"` + discover.key + `": "https://online.sberbank.ru\n"`,
			`"` + discover.key + `": "https://online.sberbank.ru\\@example.org"`,
			`"` + discover.key + `": "https://online.sberbank.ru` + string(byte(1)) + `"`,
		} {
			if _, ok := discover.parse(text); ok {
				t.Fatal("unsafe, ambiguous or nonliteral origin accepted")
			}
		}
	}
}
func TestNormalizeAuthBaseSafety(t *testing.T) {
	for _, c := range []struct{ input, want string }{
		{"CSAFront", AppOrigin + "/CSAFront"}, {"/CSAFront/", AppOrigin + "/CSAFront"}, {"//CSAFront//", ""},
		{"/CSAFront//nested///", AppOrigin + "/CSAFront/nested"}, {AppOrigin + "/CSAFront/", AppOrigin + "/CSAFront"},
		{"https://web-fixture.online.sberbank.ru/CSAFront/", "https://web-fixture.online.sberbank.ru/CSAFront"},
	} {
		value, err := NormalizeAuthBase(c.input)
		if (err == nil) != (c.want != "") || value != c.want {
			t.Fatal("auth base normalization mismatch")
		}
	}
	for _, value := range []string{"", "/", ".", "../CSAFront", "/CSAFront/..", AppOrigin + "/../CSAFront", AppOrigin + "/CSAFront/%2e%2e", AppOrigin + "/CSAFront/%252e%252e", "/CSAFront/%2e%2e%2fother", AppOrigin + "/CSAFront?x=1", AppOrigin + "/CSAFront#secret", AppOrigin + "/CSAFront?", AppOrigin + "/CSAFront#", " CSAFront", "CSAFront ", "CSAFront\n", `CSAFront\x`, `https://example.org/CSAFront`, "//example.org/CSAFront", AppOrigin + ":444/CSAFront", "https://online.sberbank.ru.evil/CSAFront", "https://user:secret@online.sberbank.ru/CSAFront"} {
		result, err := NormalizeAuthBase(value)
		if err == nil || result != "" || (value != "" && strings.Contains(err.Error(), value)) {
			t.Fatal("unsafe base accepted or leaked")
		}
	}
}
func TestSafeRedirectResolution(t *testing.T) {
	for _, c := range []struct{ value, base, want string }{
		{"/main?ticket=synthetic#drop", AppOrigin + "/CSAFront/", AppOrigin + "/main?ticket=synthetic"},
		{"../main", AppOrigin + "/app/path", AppOrigin + "/main"},
		{"https://web-fixture.online.sberbank.ru/main", AppOrigin + "/", "https://web-fixture.online.sberbank.ru/main"},
		{"//web-fixture.online.sberbank.ru/main", AppOrigin + "/", "https://web-fixture.online.sberbank.ru/main"},
		{"?ticket=synthetic", AppOrigin + "/main", AppOrigin + "/main?ticket=synthetic"},
	} {
		got, err := SafeOnlineURL(c.value, c.base)
		if err != nil || got != c.want {
			t.Fatal("safe redirect resolution differs")
		}
	}
	for _, value := range []string{"", "https://example.org/main", "https://online.sberbank.ru.evil/main", "http://online.sberbank.ru/main", "https://online.sberbank.ru:444/main", "https://user@online.sberbank.ru/main", "/bad\npath", `/bad\path`, "javascript:alert(1)"} {
		target, err := SafeOnlineURL(value, AppOrigin+"/")
		var typed *sdkErrs.PinAuthError
		if target != "" || !errors.As(err, &typed) || typed.Code != "unsafe_redirect" {
			t.Fatal("unsafe redirect not rejected")
		}
	}
	for _, status := range []int{301, 302, 303, 307, 308} {
		target, post, err := ResolveAuthRedirect(AppOrigin+"/entry", status, "/main", true)
		if err != nil || target != AppOrigin+"/main" || post != (status == 307 || status == 308) {
			t.Fatal("redirect method not preserved correctly")
		}
		_, post, err = ResolveAuthRedirect(AppOrigin+"/entry", status, "/main", false)
		if err != nil || post {
			t.Fatal("GET promoted to POST")
		}
	}
	for _, c := range []struct {
		status   int
		location string
	}{{302, ""}, {200, "/main"}, {399, "/main"}, {302, "https://example.org"}} {
		target, _, err := ResolveAuthRedirect(AppOrigin+"/entry", c.status, c.location, false)
		if err == nil || target != "" {
			t.Fatal("invalid redirect metadata accepted")
		}
	}
}
func TestAuthEndpointAndCaptchaScope(t *testing.T) {
	got, err := AuthEndpoint(AppOrigin+"/CSAFront/", "/api/v1/pin/begin")
	if err != nil || got != AppOrigin+"/CSAFront/api/v1/pin/begin" {
		t.Fatal("auth endpoint mismatch")
	}
	for _, value := range []string{"../outside", "api/%2e%2e/escaped", "https://example.org", "api?secret=1", `api\bad`} {
		if got, err := AuthEndpoint(AppOrigin+"/CSAFront", value); err == nil || got != "" {
			t.Fatal("unsafe endpoint accepted")
		}
	}
	for _, value := range []string{"captcha.png?noc=synthetic", AppOrigin + "/CSAFront/audio"} {
		if _, err := SafeCaptchaURL(value, AppOrigin+"/CSAFront"); err != nil {
			t.Fatal("safe captcha rejected")
		}
	}
	for _, value := range []string{"../outside", "/CSAFront-other/captcha", "/other/captcha", "/CSAFront/%2e%2e/other"} {
		if _, err := SafeCaptchaURL(value, AppOrigin+"/CSAFront"); err == nil {
			t.Fatal("captcha escaped auth path")
		}
	}
}
