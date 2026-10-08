package session

import (
	"fmt"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestFoundationMaxAgePrecedence(t *testing.T) {
	u, _ := url.Parse("https://online.sberbank.ru/CSAFront/index.do")
	const now int64 = 2000000000
	for _, age := range []string{"0", "-1", "-60"} {
		for _, first := range []string{"audit=removed", "audit"} {
			change, err := ParseResponseCookie(u, first+"; Path=/; Secure; Max-Age="+age+"; Expires=Fri, 01 Jan 2100 00:00:00 GMT", now)
			if err != nil || change == nil || !change.Deleted || change.Record.Expires != nil {
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
		change, err := ParseResponseCookie(u, "audit=value; Max-Age="+tt.age+"; Expires=Fri, 01 Jan 2100 00:00:00 GMT", now)
		if err != nil || change == nil || change.Deleted || change.Record.Expires == nil || *change.Record.Expires != tt.expiry {
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
		change, err := ParseResponseCookie(u, "TSaudit; Domain=online.sberbank.ru; Path=/specific; Secure; HttpOnly; SameSITE=sTrIcT; "+tt.attrs, now)
		if err != nil || change == nil {
			t.Fatalf("valid valueless cookie rejected: %v", err)
		}
		c := change.Record
		if c.Name != "TSaudit" || c.Value != "" || c.Domain != "online.sberbank.ru" || c.Path != "/specific" || !c.Secure || !c.HTTPOnly || c.HostOnly || c.SameSite == nil || *c.SameSite != "Strict" || c.Expires == nil || *c.Expires != tt.expiry || change.Deleted {
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

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
func TestCookieJarRoutingAndAuthority(t *testing.T) {
	now := time.Now().Unix()
	seed := []CookieRecord{
		{Name: "host", Value: "synthetic", Domain: "online.sberbank.ru", Path: "/CSAFront", Secure: true, HTTPOnly: true, HostOnly: true, SameSite: stringPtr("Strict")},
		{Name: "domain", Value: "synthetic", Domain: "online.sberbank.ru", Path: "/", Secure: true, HostOnly: false},
		{Name: "expired", Value: "synthetic", Domain: "online.sberbank.ru", Path: "/", Secure: true, HostOnly: false, Expires: int64Ptr(now - 1)},
	}
	jar, err := NewCookieJar(seed)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		u     string
		names []string
	}{
		{"https://online.sberbank.ru/CSAFront/index.do", []string{"host", "domain"}},
		{"https://web2.online.sberbank.ru/CSAFront/index.do", []string{"domain"}},
		{"https://online.sberbank.ru/CSAFrontier", []string{"domain"}},
		{"http://online.sberbank.ru/CSAFront/index.do", nil},
		{"https://evil.invalid/", nil},
	} {
		records := jar.RecordsForURL(mustURL(t, tt.u))
		var names []string
		for _, c := range records {
			names = append(names, c.Name)
		}
		if !reflect.DeepEqual(names, tt.names) {
			t.Fatal("routing mismatch")
		}
	}
	u := mustURL(t, "https://online.sberbank.ru/CSAFront/index.do")
	if err := jar.ApplySetCookie(u, []string{"host=synthetic; Path=/CSAFront; Secure; SameSITE=lAx; HttpOnly"}); err != nil {
		t.Fatal(err)
	}
	records := jar.RecordsForURL(u)
	if records[0].SameSite == nil || *records[0].SameSite != "Lax" || !records[0].HTTPOnly {
		t.Fatal("case metadata lost")
	}
	if err := jar.ApplySetCookie(u, []string{"host=synthetic; Path=/CSAFront; Secure"}); err != nil {
		t.Fatal(err)
	}
	if jar.RecordsForURL(u)[0].SameSite != nil {
		t.Fatal("same-value authoritative reset lost")
	}
	if err := jar.ApplySetCookie(u, []string{"host=deleted; Path=/CSAFront; Secure; Max-Age=0"}); err != nil {
		t.Fatal(err)
	}
	if len(jar.Snapshot()) != 1 {
		t.Fatal("scoped deletion lost")
	}
	if err := jar.ApplySetCookie(u, []string{"new=synthetic; Secure; HttpOnly; SameSite=None"}); err != nil {
		t.Fatal(err)
	}
	records = jar.Snapshot()
	if len(records) != 2 || records[1].Path != "/CSAFront" || !records[1].HostOnly || !records[1].HTTPOnly || *records[1].SameSite != "None" {
		t.Fatal("default path or metadata lost")
	}
	cloned := jar.Snapshot()
	cloned[0].Value = "mutated"
	if jar.Snapshot()[0].Value == "mutated" {
		t.Fatal("snapshot aliases live jar")
	}
	// Incoming headers do not delete unrelated concurrent state; invalid/partition
	// metadata fails atomically rather than silently being dropped.
	before := jar.Snapshot()
	for _, header := range []string{"new=synthetic; Secure; SameSite=invalid", "new=synthetic; Secure; Partitioned"} {
		if err := jar.ApplySetCookie(u, []string{"valid=synthetic; Secure", header}); err == nil {
			t.Fatal("unsupported metadata accepted")
		}
		if !reflect.DeepEqual(before, jar.Snapshot()) {
			t.Fatal("invalid update partially adopted")
		}
	}
	if err := jar.ApplySetCookie(u, []string{"outsider=synthetic; Domain=evil.invalid; Secure"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, jar.Snapshot()) {
		t.Fatal("foreign-domain cookie accepted")
	}
}

func TestCookieJarConcurrentUpdatesDoNotPrune(t *testing.T) {
	jar, err := NewCookieJar(nil)
	if err != nil {
		t.Fatal(err)
	}
	u := mustURL(t, "https://online.sberbank.ru/CSAFront/index.do")
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("fixture-%d", i)
			if err := jar.ApplySetCookie(u, []string{name + "=synthetic; Secure; SameSite=Strict"}); err != nil {
				t.Error(err)
			}
			_ = jar.Snapshot()
			_ = jar.RecordsForURL(u)
		}(i)
	}
	wg.Wait()
	if len(jar.Snapshot()) != 32 {
		t.Fatal("concurrent cookie pruned")
	}
}
