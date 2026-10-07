package session

import (
	"errors"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

func TestReviewFrontendConfigRejectsMalformedPropertyKeyUnicode(t *testing.T) {
	for _, parser := range []struct {
		fixture string
		parse   func(string) (FrontendConfig, error)
	}{{"pin_valid_literal", ParsePINConfig}, {"primary_valid_literal", ParsePrimaryConfig}} {
		base, _ := frontendFixture(t, parser.fixture)
		for _, scope := range []string{"window.config = {", "pinConfig: {"} {
			for _, tc := range []struct {
				name, key string
				accepted  bool
			}{
				{"unpaired_high", `"\ud800"`, false},
				{"unpaired_low", `"\udfff"`, false},
				{"high_then_non_surrogate", `"\ud800\u0061"`, false},
				{"literal_replacement_character", `"�"`, true},
				{"escaped_replacement_character", `"\ufffd"`, true},
				{"valid_surrogate_pair", `"\ud83d\ude00"`, true},
				{"escaped_backslash_text", `"\\ud800"`, true},
			} {
				t.Run(parser.fixture+"/"+scope+"/"+tc.name, func(t *testing.T) {
					html := strings.Replace(base, scope, scope+tc.key+`: true,`, 1)
					if html == base {
						t.Fatal("synthetic property key mutation did not apply")
					}
					config, err := parser.parse(html)
					if tc.accepted {
						if err != nil || config.ProcessID() != "process-fixture" {
							t.Fatal("valid Unicode property key rejected")
						}
						return
					}
					var typed *sdkErrs.PinAuthError
					if !errors.As(err, &typed) || typed.Code != "invalid_frontend_config" || config != (FrontendConfig{}) {
						t.Fatal("malformed quoted property key was rewritten or accepted")
					}
				})
			}
		}
	}
}

func TestReviewFrontendConfigRejectsMalformedProcessIDUnicode(t *testing.T) {
	for _, parser := range []struct {
		fixture string
		parse   func(string) (FrontendConfig, error)
	}{{"pin_valid_literal", ParsePINConfig}, {"primary_valid_literal", ParsePrimaryConfig}} {
		base, _ := frontendFixture(t, parser.fixture)
		for _, tc := range []struct{ name, literal, want string }{
			{"unpaired_high", `"\ud800"`, ""},
			{"unpaired_low", `"\udfff"`, ""},
			{"high_then_non_surrogate", `"\ud800\u0061"`, ""},
			{"reversed_pair", `"\udfff\ud800"`, ""},
			{"literal_replacement_character", `"�"`, "�"},
			{"escaped_replacement_character", `"\ufffd"`, "�"},
			{"valid_surrogate_pair", `"\ud83d\ude00"`, "😀"},
			{"escaped_backslash_text", `"\\ud800"`, `\ud800`},
		} {
			t.Run(parser.fixture+"/"+tc.name, func(t *testing.T) {
				html := strings.Replace(base, `processId: "process-fixture"`, `processId: `+tc.literal, 1)
				if html == base {
					t.Fatal("synthetic process ID mutation did not apply")
				}
				config, err := parser.parse(html)
				if tc.want != "" {
					if err != nil || config.ProcessID() != tc.want {
						t.Fatal("valid process ID Unicode changed or was rejected")
					}
					return
				}
				var typed *sdkErrs.PinAuthError
				if !errors.As(err, &typed) || typed.Code != "invalid_frontend_config" || config != (FrontendConfig{}) {
					t.Fatal("malformed Unicode process ID was rewritten or published")
				}
				if strings.Contains(err.Error(), tc.literal) {
					t.Fatal("frontend diagnostic included the untrusted literal")
				}
			})
		}
	}
}
