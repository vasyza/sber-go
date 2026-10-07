package session

import (
	"errors"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

func TestRuntimeOriginDiscovery(t *testing.T) {
	for _, discover := range []struct {
		key   string
		parse func(string) (string, bool)
	}{
		{"ufsHost", UFSHostFromAppShell}, {"ufs.block.root.url", APIBaseFromMainHTML},
	} {
		literal := `"` + discover.key + `": "https:\/\/web-fixture.online.sberbank.ru\/"`
		for _, text := range []string{"<script>{" + literal + "}</script>", literal + "," + literal, strings.Replace(literal, `\/`, `/`, -1), strings.Replace(literal, `": "`, "\"\u00a0:\u2003\"", 1)} {
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
