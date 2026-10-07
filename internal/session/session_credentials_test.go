package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestCredentialsAndCookieSeedReadiness(t *testing.T) {
	credentials, err := NewSberCredentials("synthetic-ufs-session", "synthetic-ufs-token")
	if err != nil {
		t.Fatal(err)
	}
	b, err := credentials.ToBundle(CredentialsBundleOptions{APIBase: "https://web-node2.online.sberbank.ru", WebBase: "https://web2.online.sberbank.ru"})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := b.ToSeed(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(seed.Cookies.Snapshot()) != 2 {
		t.Fatal("two minimum UFS cookies not created")
	}
	for _, c := range b.Cookies {
		if !c.Secure || !c.HTTPOnly || c.HostOnly || c.Path != "/" || c.Domain != AuthCookieDomain {
			t.Fatal("credentials cookie metadata changed")
		}
	}
	got, err := CredentialsFromBundle(b)
	if err != nil || got.UFSSession != credentials.UFSSession || got.UFSToken != credentials.UFSToken {
		t.Fatal("auth cookies not extracted")
	}
	for _, value := range []any{credentials, seed} {
		raw, _ := json.Marshal(value)
		for _, s := range []string{fmt.Sprintf("%#v", value), string(raw)} {
			if strings.Contains(s, "synthetic-ufs") {
				t.Fatal("credential or seed leaked")
			}
		}
	}
	empty := SessionBundle{APIBase: AppOrigin, WebBase: AppOrigin}
	if _, err := empty.ToSeed(false); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.ToSeed(true); err == nil {
		t.Fatal("empty ready seed accepted")
	}
	b.Cookies[0].HostOnly = true
	b.Cookies[1].HostOnly = true
	if _, err := b.ToSeed(true); err == nil {
		t.Fatal("hostonly cookies incorrectly ready for subdomains")
	}
	if _, err := CredentialsFromBundle(b); err == nil {
		t.Fatal("hostonly cookies treated as cross-subdomain credentials")
	}
	b.Cookies[0].HostOnly = false
	b.Cookies[1].HostOnly = false
	b.Cookies[0].Path = "/api"
	b.Cookies[1].Path = "/api"
	if _, err := b.ToSeed(true); err == nil {
		t.Fatal("warmup-only cookies counted as ready")
	}
}
func TestWithCookieJarPreservesSourceHttpOnlyAndDeletion(t *testing.T) {
	b, _ := NewSessionBundle(syntheticBundle())
	jar, _ := b.ToSeed(false)
	u := mustURL(t, "https://online.sberbank.ru/CSAFront/index.do")
	if err := jar.Cookies.ApplySetCookie(u, []string{"UFS-SESSION=rotated; Domain=online.sberbank.ru; Path=/; Secure; SameSite=Lax"}); err != nil {
		t.Fatal(err)
	}
	updated, err := b.WithCookieJar(jar.Cookies)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Cookies) != 1 || !updated.Cookies[0].HTTPOnly || *updated.Cookies[0].SameSite != "Lax" || updated.CapturedAt == nil {
		t.Fatal("source bundle reconciliation changed")
	}
	if err := jar.Cookies.ApplySetCookie(u, []string{"UFS-SESSION=deleted; Domain=online.sberbank.ru; Path=/; Max-Age=0; Secure"}); err != nil {
		t.Fatal(err)
	}
	updated, err = b.WithCookieJar(jar.Cookies)
	if err != nil || len(updated.Cookies) != 0 {
		t.Fatal("deleted cookie resurrected")
	}
}
