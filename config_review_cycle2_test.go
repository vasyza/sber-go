package sber

import (
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

func reviewCycle2AssertFrontend(t *testing.T, parse func(string) (FrontendConfig, error), html string, want FrontendConfig, accepted bool) {
	t.Helper()
	t.Logf("synthetic_fixture_sha256=%x bytes=%d", sha256.Sum256([]byte(html)), len(html))
	config, err := parse(html)
	if !accepted {
		var typed *PinAuthError
		if !errors.As(err, &typed) || typed.Code != "invalid_frontend_config" || config != (FrontendConfig{}) {
			t.Fatal("nonliteral or malformed source did not fail with typed error and exact zero runtime")
		}
		return
	}
	// Opaque identity is deliberately not content equality.
	if err != nil || config.BaseURL() != want.BaseURL() || config.ProcessID() != want.ProcessID() || config.PINLength() != want.PINLength() || config.NHex() != want.NHex() || config.GHex() != want.GHex() || config.SeamlessWeb() != want.SeamlessWeb() || config.RedirectPost() != want.RedirectPost() {
		t.Fatal("line-comment scanning changed accepted literal runtime or flags")
	}
}

func TestReviewCycle2FrontendLineCommentsKeepLiteralContract(t *testing.T) {
	for _, parser := range []struct {
		fixture string
		parse   func(string) (FrontendConfig, error)
	}{{"pin_valid_literal", ParsePINConfig}, {"primary_valid_literal", ParsePrimaryConfig}} {
		base, _ := frontendFixture(t, parser.fixture)
		want, err := parser.parse(base)
		if err != nil {
			t.Fatal("invalid baseline synthetic fixture")
		}
		for _, line := range reviewCycle2LineEnds {
			comment := "// synthetic } { , :" + line.text
			for _, tc := range []struct {
				name, from, to string
				accepted       bool
			}{
				{"assignment_object", "window.config = {", "window.config = " + comment + "{", true},
				{"ordinary_key_colon", "processId:", "processId" + comment + ":", true},
				{"escaped_key_colon", "processId:", `"\u0070rocessId"` + comment + ":", true},
				{"colon_value", "processId:", "processId:" + comment, true},
				{"ignored_balanced_value", "window.config = {", "window.config = {ignoredComment: (function(){" + comment + `return ["}", "//", "\ud83d\ude00"]; }),`, true},
				{"decoded_duplicate", "processId:", `"\u0070rocessId"` + comment + `:"second", processId:`, false},
				{"expression_suffix", `processId: "process-fixture"`, `processId: "process-fixture"` + comment + ` + ""`, false},
				// Preserve the pinned conservative literal boundary: trailing source
				// comments are not stripped from security-relevant config values.
				{"after_literal_not_a_strict_literal", `processId: "process-fixture"`, `processId: "process-fixture"` + comment, false},
			} {
				t.Run(parser.fixture+"/"+line.name+"/"+tc.name, func(t *testing.T) {
					html := strings.Replace(base, tc.from, tc.to, 1)
					if html == base {
						t.Fatal("synthetic mutation did not apply")
					}
					reviewCycle2AssertFrontend(t, parser.parse, html, want, tc.accepted)
				})
			}
		}
	}
}

func TestReviewCycle2FrontendCommentsPreserveStringAndZeroBounds(t *testing.T) {
	for _, parser := range []struct {
		fixture string
		parse   func(string) (FrontendConfig, error)
	}{{"pin_valid_literal", ParsePINConfig}, {"primary_valid_literal", ParsePrimaryConfig}} {
		base, _ := frontendFixture(t, parser.fixture)
		want, err := parser.parse(base)
		if err != nil {
			t.Fatal("invalid baseline synthetic fixture")
		}
		for _, tc := range []struct {
			name, key string
			accepted  bool
		}{
			{"valid_pair", `"\ud83d\ude00"`, true},
			{"literal_replacement", `"�"`, true},
			{"escaped_replacement", `"\ufffd"`, true},
			{"escaped_backslash_text", `"\\ud800"`, true},
			{"unpaired_high", `"\ud800"`, false},
			{"unpaired_low", `"\udfff"`, false},
		} {
			for _, line := range reviewCycle2LineEnds {
				t.Run(parser.fixture+"/"+line.name+"/"+tc.name, func(t *testing.T) {
					html := strings.Replace(base, "window.config = {", "window.config = {"+tc.key+"// synthetic"+line.text+":true,", 1)
					reviewCycle2AssertFrontend(t, parser.parse, html, want, tc.accepted)
				})
			}
		}
		for _, line := range reviewCycle2LineEnds {
			for _, tc := range []struct {
				name, literal, process string
				accepted               bool
			}{
				{"process_valid_pair", `"\ud83d\ude00"`, "😀", true},
				{"process_pair_at_512_runes", `"` + strings.Repeat(`\ud83d\ude00`, 512) + `"`, strings.Repeat("😀", 512), true},
				{"process_pair_at_513_runes", `"` + strings.Repeat(`\ud83d\ude00`, 513) + `"`, "", false},
				{"process_unpaired", `"\ud800"`, "", false},
				{"unterminated_process_string", `"synthetic`, "", false},
			} {
				t.Run(parser.fixture+"/"+line.name+"/"+tc.name, func(t *testing.T) {
					html := strings.Replace(base, `processId: "process-fixture"`, "processId// synthetic"+line.text+":"+tc.literal, 1)
					adjusted := NewFrontendConfig(want.BaseURL(), tc.process, want.PINLength(), want.NHex(), want.GHex(), want.SeamlessWeb(), want.RedirectPost())
					reviewCycle2AssertFrontend(t, parser.parse, html, adjusted, tc.accepted)
				})
			}
		}
		for _, tc := range []struct {
			name, html string
			accepted   bool
		}{
			{"closed_block", strings.Replace(base, "processId:", "processId/* synthetic\r\n\u2028\u2029 } */:", 1), true},
			{"unterminated_block", strings.Replace(base, "processId:", "processId/* synthetic:", 1), false},
			{"EOF_comment_after_document", base + "// synthetic", true},
			{"EOF_comment_before_object", "window.config = // synthetic", false},
			{"literal_escaped_LS_is_not_terminator", strings.Replace(base, "processId:", `processId// synthetic\u2028:`, 1), false},
			{"literal_escaped_PS_is_not_terminator", strings.Replace(base, "processId:", `processId// synthetic\u2029:`, 1), false},
			{"invalid_UTF8", base + string([]byte{0xff}), false},
		} {
			t.Run(parser.fixture+"/"+tc.name, func(t *testing.T) { reviewCycle2AssertFrontend(t, parser.parse, tc.html, want, tc.accepted) })
		}
	}
}
