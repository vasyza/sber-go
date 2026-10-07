package sber

import (
	"fmt"
	"net/url"
	"reflect"
	"sync"
	"testing"
	"time"
)

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
	copy := jar.Snapshot()
	copy[0].Value = "mutated"
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
