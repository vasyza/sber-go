package sber

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Only audited, synthetic frontend fixture records are loaded here.
func frontendFixture(t *testing.T, id string) (string, map[string]any) {
	t.Helper()
	raw, err := os.ReadFile("testdata/compat/frontend-config.json")
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
		var typed *PinAuthError
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
