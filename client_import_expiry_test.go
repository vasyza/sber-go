package sber

import (
	"encoding/json"
	"os"
	"testing"
)

func TestClientHARCookieNumericExpiryAndStringMaxAgeFollowSource(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cookie     map[string]any
		wantExpiry *int64
		reject     bool
	}{
		{name: "numeric-fraction", cookie: map[string]any{"expires": json.Number("4102444800.75")}, wantExpiry: func() *int64 { x := int64(4102444800); return &x }()},
		{name: "max-age-zero-string", cookie: map[string]any{"expires": json.Number("4102444800"), "maxAge": "0"}, reject: true},
		{name: "max-age-fraction", cookie: map[string]any{"expires": json.Number("4102444800"), "maxAge": json.Number("0.9")}, reject: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cookie := map[string]any{"name": "SID", "value": "synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true}
			for k, v := range tc.cookie {
				cookie[k] = v
			}
			entries := []any{map[string]any{"startedDateTime": "2026-07-13T09:00:00Z", "request": map[string]any{"method": "GET", "url": "https://web2.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{cookie}}}, map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}}}
			raw, e := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
			if e != nil {
				t.Fatal(e)
			}
			path := clientPrivatePath(t)
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			c, e := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { return clientFake(t, b), nil }})
			if tc.reject {
				if c != nil {
					c.Close()
				}
				if e == nil {
					t.Fatal("expired HAR cookie made profile ready")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			b, e := c.ExportSession()
			if e != nil || len(b.Cookies) != 1 || b.Cookies[0].Expires == nil || *b.Cookies[0].Expires != *tc.wantExpiry {
				t.Fatal("source numeric expiry lost")
			}
		})
	}
}
