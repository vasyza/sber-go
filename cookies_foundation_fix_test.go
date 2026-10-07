package sber

import (
	"net/url"
	"testing"
)

func TestFoundationMaxAgePrecedence(t *testing.T) {
	u, _ := url.Parse("https://online.sberbank.ru/CSAFront/index.do")
	const now int64 = 2000000000
	for _, age := range []string{"0", "-1", "-60"} {
		for _, first := range []string{"audit=removed", "audit"} {
			change, err := parseResponseCookie(u, first+"; Path=/; Secure; Max-Age="+age+"; Expires=Fri, 01 Jan 2100 00:00:00 GMT", now)
			if err != nil || change == nil || !change.deleted || change.record.Expires != nil {
				t.Errorf("Max-Age=%s deletion overridden by future Expires", age)
			}
			jar, err := NewCookieJar([]CookieRecord{{Name: "audit", Value: "old", Domain: "online.sberbank.ru", Path: "/", Secure: true, HostOnly: true}})
			if err != nil {
				t.Fatal(err)
			}
			if err := jar.ApplySetCookie(u, []string{first + "; Path=/; Secure; Max-Age=" + age + "; Expires=Fri, 01 Jan 2100 00:00:00 GMT"}); err != nil {
				t.Fatal(err)
			}
			if len(jar.Snapshot()) != 0 {
				t.Errorf("Max-Age=%s cookie retained", age)
			}
		}
	}
	for _, tt := range []struct {
		age    string
		expiry int64
	}{
		{"60", now + 60}, {"bad", 4102444800}, {"01", 4102444800}, {"", 4102444800},
	} {
		change, err := parseResponseCookie(u, "audit=value; Max-Age="+tt.age+"; Expires=Fri, 01 Jan 2100 00:00:00 GMT", now)
		if err != nil || change == nil || change.deleted || change.record.Expires == nil || *change.record.Expires != tt.expiry {
			t.Errorf("positive/invalid Max-Age semantics changed: %q", tt.age)
		}
	}
}

func TestFoundationValuelessCookieAttributes(t *testing.T) {
	u, err := url.Parse("https://web-node2.online.sberbank.ru/CSAFront/index.do")
	if err != nil {
		t.Fatal(err)
	}
	const now int64 = 2000000000
	for _, tt := range []struct {
		attrs  string
		expiry int64
	}{
		{"Max-Age=60; Expires=Fri, 01 Jan 2100 00:00:00 GMT", now + 60},
		{"Expires=Fri, 01 Jan 2100 00:00:00 GMT", 4102444800},
	} {
		change, err := parseResponseCookie(u, "TSaudit; Domain=online.sberbank.ru; Path=/specific; Secure; HttpOnly; SameSITE=sTrIcT; "+tt.attrs, now)
		if err != nil || change == nil {
			t.Fatalf("valid valueless cookie rejected: %v", err)
		}
		c := change.record
		if c.Name != "TSaudit" || c.Value != "" || c.Domain != "online.sberbank.ru" || c.Path != "/specific" || !c.Secure || !c.HTTPOnly || c.HostOnly || c.SameSite == nil || *c.SameSite != "Strict" || c.Expires == nil || *c.Expires != tt.expiry || change.deleted {
			t.Error("valueless cookie lost routing/security/expiry attributes")
		}
	}
	jar, err := NewCookieJar([]CookieRecord{{Name: "TSaudit", Value: "old", Domain: "online.sberbank.ru", Path: "/specific", Secure: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := jar.ApplySetCookie(u, []string{"TSaudit; Domain=online.sberbank.ru; Path=/specific; Secure; Max-Age=0"}); err != nil {
		t.Fatal(err)
	}
	if len(jar.Snapshot()) != 0 {
		t.Error("valueless Max-Age=0 did not delete scoped cookie")
	}
}
