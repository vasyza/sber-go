package session

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	"os"
	"strings"
	"testing"
)

func reviewCycle2AssertFrontend(t *testing.T, parse func(string) (FrontendConfig, error), html string, want FrontendConfig, accepted bool) {
	t.Helper()
	t.Logf("synthetic_fixture_sha256=%x bytes=%d", sha256.Sum256([]byte(html)), len(html))
	config, err := parse(html)
	if !accepted {
		var typed *sdkErrs.PinAuthError
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

// Only audited, synthetic frontend fixture records are loaded here.
func frontendFixture(t *testing.T, id string) (string, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/compat/frontend-config.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Synthetic bool `json:"synthetic_only"`
		Records   []struct {
			ID   string         `json:"id"`
			Data map[string]any `json:"data"`
		} `json:"records"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil || !fixture.Synthetic {
		t.Fatal("invalid synthetic fixture")
	}
	for _, r := range fixture.Records {
		if r.ID == id {
			return r.Data["html"].(string), r.Data
		}
	}
	t.Fatal("fixture missing")
	return "", nil
}
func checkFrontendGolden(t *testing.T, id string, primary bool) {
	t.Helper()
	html, data := frontendFixture(t, id)
	var config FrontendConfig
	var err error
	if primary {
		config, err = ParsePrimaryConfig(html)
	} else {
		config, err = ParsePINConfig(html)
	}
	if _, invalid := data["expected_error"]; invalid {
		var typed *sdkErrs.PinAuthError
		if !errors.As(err, &typed) || typed.Code != "invalid_frontend_config" || config != (FrontendConfig{}) {
			t.Fatal("did not reject invalid frontend config atomically")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	want := data["expected_runtime"].(map[string]any)
	if config.BaseURL() != want["base_url"] || config.ProcessID() != want["process_id"] || config.PINLength() != int(want["pin_length"].(float64)) || config.NHex() != want["n_hex"] || config.GHex() != want["g_hex"] || config.SeamlessWeb() != want["seamless_web"] || config.RedirectPost() != want["redirect_post"] {
		t.Fatal("frontend fields differ from audited golden")
	}
}
func TestParity_GoldenTestsTestPinAuthPinValidLiteral(t *testing.T) {
	checkFrontendGolden(t, "pin_valid_literal", false)
}
func TestParity_GoldenTestsTestPinAuthPinDuplicateBase(t *testing.T) {
	checkFrontendGolden(t, "pin_duplicate_base", false)
}
func TestParity_GoldenTestsTestPinAuthPinExternalAuthBase(t *testing.T) {
	checkFrontendGolden(t, "pin_external_auth_base", false)
}
func TestParity_GoldenTestsTestPinAuthPinWrongAuthMode(t *testing.T) {
	checkFrontendGolden(t, "pin_wrong_auth_mode", false)
}
func TestParity_GoldenTestsTestBrowserBootstrapFailuresPinUndersizedSrp(t *testing.T) {
	checkFrontendGolden(t, "pin_undersized_srp", false)
}
func TestParity_GoldenTestsTestBrowserBootstrapFailuresPinDuplicateAssignment(t *testing.T) {
	checkFrontendGolden(t, "pin_duplicate_assignment", false)
}

func TestFrontendPINLiteralSyntax(t *testing.T) {
	html, _ := frontendFixture(t, "pin_valid_literal")
	html = strings.Replace(html, "window.config = {", "window \n . config = /* ignored } */ { // ignored {\n", 1)
	html = strings.Replace(html, `ignored: {text: "comma, brace } and // are data"}`, "ignored: {text: \"brace } and \\\"\", fn: (function(){return ['a,}', `b{`];})}", 1)
	html = strings.Replace(html, "processId:", `"processId":`, 1)
	config, err := ParsePINConfig(html)
	if err != nil || config.ProcessID() != "process-fixture" {
		t.Fatal("comments, quoted keys or balanced ignored values rejected")
	}
	for _, input := range []string{
		"", "window.config = run()", "window.config = {", "window.config = {/* never closed",
		strings.Replace(html, `"processId": "process-fixture"`, `"processId": "process-fixture", processId: "duplicate"`, 1),
		strings.Replace(html, `"processId": "process-fixture"`, `"processId": 'not JSON'`, 1),
		strings.Replace(html, "enabled: true", "enabled: 1", 1),
		strings.Replace(html, "length: 5", "length: 05", 1),
		strings.Replace(html, "length: 5", "length: 5.0", 1),
		strings.Replace(html, "length: 5", "length: +5", 1),
		strings.Replace(html, "length: 5", "length: (5)", 1),
		strings.Replace(html, "isSeamlessWeb: true", "isSeamlessWeb: true /*not literal*/", 1),
		strings.Replace(html, "hasPin: true", "hasPin: false", 1),
		strings.Replace(html, `authTypeByCookie: "pin"`, `authTypeByCookie: "start"`, 1),
		strings.Replace(html, `"processId": "process-fixture"`, `"processId": "bad\nprocess"`, 1),
		strings.Replace(html, "srp_g: \"2\"", "srp_g: \"1\"", 1),
		strings.Replace(html, "fn: (function(){return ['a,}', `b{`];})", "fn: ([)]", 1),
		html + "<!-- window.config = {} -->",
	} {
		if _, err = ParsePINConfig(input); err == nil {
			t.Fatal("nonliteral, malformed or nonunique input accepted")
		}
	}
}

func TestParity_GoldenTestsTestPrimaryAuthPrimaryValidLiteral(t *testing.T) {
	checkFrontendGolden(t, "primary_valid_literal", true)
}
func TestParity_GoldenTestsTestBrowserBootstrapFailuresPrimaryUndersizedSrp(t *testing.T) {
	checkFrontendGolden(t, "primary_undersized_srp", true)
}
func TestParity_GoldenTestsTestBrowserBootstrapFailuresPrimaryDuplicateAssignment(t *testing.T) {
	checkFrontendGolden(t, "primary_duplicate_assignment", true)
}
func TestFrontendPythonWhitespaceAndSourceGuard(t *testing.T) {
	html, _ := frontendFixture(t, "pin_valid_literal")
	spaced := strings.Replace(html, "window.config =", "window\u00a0.\u2003config\u202f=", 1)
	if _, err := ParsePINConfig(spaced); err != nil {
		t.Fatal("Python Unicode assignment whitespace rejected")
	}
	spaced = strings.Replace(html, "enabled: true,", "enabled: true\x1c,", 1)
	if _, err := ParsePINConfig(spaced); err != nil {
		t.Fatal("Python value whitespace rejected")
	}
	for _, prefix := range []string{"x", "_", "é", "１"} {
		prefixed := strings.Replace(html, "window.config", prefix+"window.config", 1)
		if _, err := ParsePINConfig(prefixed); err == nil {
			t.Fatal("Unicode word-boundary source guard bypassed")
		}
	}
}

func TestFrontendPrimaryContract(t *testing.T) {
	html, _ := frontendFixture(t, "primary_valid_literal")
	config, err := ParsePrimaryConfig(html)
	if err != nil || config.PINLength() != 5 {
		t.Fatal("valid primary config rejected")
	}
	if _, err := ParsePINConfig(html); err == nil {
		t.Fatal("primary accepted as PIN")
	}
	pin, _ := frontendFixture(t, "pin_valid_literal")
	if _, err := ParsePrimaryConfig(pin); err == nil {
		t.Fatal("PIN accepted as primary")
	}
	for _, input := range []string{
		strings.Replace(html, "srpConfig: {enabled: true", "srpConfig: {enabled: false", 1),
		strings.Replace(html, "hasPin: false", "hasPin: true", 1),
		strings.Replace(html, "pinConfig: {enabled: true", "pinConfig: {enabled: false", 1),
		strings.Replace(html, "length: 5", "length: 13", 1),
		strings.Replace(html, "length: 5", "length: 3", 1),
		strings.Replace(html, "length: 5", "length: \"5\"", 1),
		strings.Replace(html, "validation:", "wrongValidation:", 1),
		strings.Replace(html, "process-fixture", strings.Repeat("x", 513), 1),
		strings.Replace(html, `processId: "process-fixture"`, `processId: ""`, 1),
		strings.Replace(html, `g: "2"`, `g: "0x2"`, 1),
	} {
		if _, err = ParsePrimaryConfig(input); err == nil {
			t.Fatal("invalid primary contract accepted")
		}
	}
	for _, fixtureID := range []string{"pin_valid_literal", "primary_valid_literal"} {
		base, _ := frontendFixture(t, fixtureID)
		parse := ParsePINConfig
		if fixtureID == "primary_valid_literal" {
			parse = ParsePrimaryConfig
		}
		for _, bits := range []int{2047, 2048, 8192, 8193} {
			// Synthetic group, not a bank or recommended cryptographic parameter set.
			n := strings.Repeat("f", (bits+3)/4)
			if bits%4 != 0 {
				n = string("137"[bits%4-1]) + n[1:]
			}
			mutated := strings.Replace(base, strings.Repeat("f", 512), n, 1)
			_, err := parse(mutated)
			if (err == nil) != (bits >= 2048 && bits <= 8192) {
				t.Fatal("SRP bit length boundary incorrect")
			}
		}
		for _, g := range []string{"0", "1", strings.Repeat("f", 512), "not-hex", "-2", "2 "} {
			key := `srp_g: "2"`
			if fixtureID == "primary_valid_literal" {
				key = `g: "2"`
			}
			if _, err := parse(strings.Replace(base, key, strings.Replace(key, `"2"`, `"`+g+`"`, 1), 1)); err == nil {
				t.Fatal("invalid generator accepted")
			}
		}
		defaults := strings.ReplaceAll(strings.ReplaceAll(base, "isSeamlessWeb: true,", ""), "isUfsRedirectMethodPostEnabled: true,", "")
		c, err := parse(defaults)
		if err != nil || c.SeamlessWeb() || c.RedirectPost() {
			t.Fatal("optional flag defaults differ")
		}
		if _, err := parse(strings.Repeat(" ", MaxFrontendHTMLCharacters) + base); err == nil {
			t.Fatal("oversized HTML accepted")
		}
	}
}

func TestFrontendAndDeviceprintConcurrent(t *testing.T) {
	pin, _ := frontendFixture(t, "pin_valid_literal")
	primary, _ := frontendFixture(t, "primary_valid_literal")
	for worker := 0; worker < 32; worker++ {
		t.Run(fmt.Sprintf("worker_%d", worker), func(t *testing.T) {
			t.Parallel()
			for iteration := 0; iteration < 4; iteration++ {
				p, err := ParsePINConfig(pin)
				if err != nil || p.PINLength() != 5 {
					t.Fatal("concurrent PIN parse failed")
				}
				c, err := ParsePrimaryConfig(primary)
				if err != nil || c.PINLength() != 5 {
					t.Fatal("concurrent primary parse failed")
				}
				if _, ok := APIBaseFromMainHTML(`"ufs.block.root.url":"https://web-fixture.online.sberbank.ru/"`); !ok {
					t.Fatal("concurrent discovery failed")
				}
				dp, err := GenerateDeviceprint()
				if err != nil || dp.Value() == "" {
					t.Fatal("concurrent generation failed")
				}
				encoded, err := GenerateAntifraudDeviceprint(dp.Value())
				if err != nil || encoded.Value() == "" {
					t.Fatal("concurrent encoding failed")
				}
			}
		})
	}
}
func FuzzFrontendConfigNeverPanics(f *testing.F) {
	f.Add(`window.config = {pinConfig: {}}`)
	f.Add(`window.config = {unknown: ([)]}`)
	f.Add("window.config = {a: `},/`}")
	f.Add(string([]byte{0xff, 0x00}))
	f.Fuzz(func(t *testing.T, html string) {
		for _, parse := range []func(string) (FrontendConfig, error){ParsePINConfig, ParsePrimaryConfig} {
			config, err := parse(html)
			if err != nil && config != (FrontendConfig{}) {
				t.Fatal("failed parse published partial runtime")
			}
		}
	})
}

func TestFrontendConfigRedaction(t *testing.T) {
	html, _ := frontendFixture(t, "pin_valid_literal")
	config, err := ParsePINConfig(html)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	value := any(config)
	for _, text := range []string{string(raw), fmt.Sprintf("%v %+v %#v %s %q", value, value, value, value, value)} {
		if strings.Contains(text, "process-fixture") || strings.Contains(text, config.NHex()) || strings.Contains(text, "CSAFront") {
			t.Fatal("config leaked in ordinary output")
		}
	}
	_, err = ParsePINConfig(strings.Replace(html, `srp_g: "2"`, `srp_g: "secret-marker"`, 1))
	if err == nil || strings.Contains(fmt.Sprintf("%+v", err), "secret-marker") {
		t.Fatal("unsafe parser diagnostic")
	}
}

func TestFrontendNewLoginFieldsUseOnlyValidatedLiterals(t *testing.T) {
	base, _ := frontendFixture(t, "primary_valid_literal")
	for _, tc := range []struct {
		name, fields string
		valid        bool
	}{
		{"valid", `encryptionKeyId:"synthetic-key",encryptionKeyValue:"synthetic-public-key",qrConfig:{size:130,useIdentifyScopeSbol:"synthetic-scope"},`, true},
		{"key-expression", `encryptionKeyValue:"synthetic"+"key",`, false},
		{"key-type", `encryptionKeyId:123,`, false},
		{"key-control", `encryptionKeyValue:"synthetic\nkey",`, false},
		{"qr-expression", `qrConfig:loadConfig(),`, false},
		{"qr-size-type", `qrConfig:{size:"130"},`, false},
		{"qr-size-small", `qrConfig:{size:63},`, false},
		{"qr-size-large", `qrConfig:{size:2049},`, false},
		{"qr-size-duplicate", `qrConfig:{size:130,size:131},`, false},
		{"qr-scope-type", `qrConfig:{useIdentifyScopeSbol:true},`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := strings.Replace(base, "window.config = {", "window.config = {"+tc.fields, 1)
			c, err := ParsePrimaryConfig(html)
			if !tc.valid {
				if err == nil || c != (FrontendConfig{}) {
					t.Fatal("unsafe new auth field accepted")
				}
				return
			}
			id, key := c.CardEncryptionKey()
			if err != nil || id != "synthetic-key" || key != "synthetic-public-key" || c.QRSize() != 130 || c.QRScope() != "synthetic-scope" {
				t.Fatal("new auth config fields lost")
			}
		})
	}
}
func TestFrontendQRDefaultDoesNotChangeZeroValueContract(t *testing.T) {
	if (FrontendConfig{}).QRSize() != 0 {
		t.Fatal("zero frontend config invented a QR size")
	}
	html, _ := frontendFixture(t, "primary_valid_literal")
	c, err := ParsePrimaryConfig(html)
	if err != nil || c.QRSize() != 190 {
		t.Fatal("validated config did not apply the QR protocol default")
	}
}
