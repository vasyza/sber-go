package sber

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func clientCycle3HAR(t *testing.T, cookie map[string]any, prior bool) string {
	t.Helper()
	item := map[string]any{"name": "SID", "value": "synthetic-expiry-canary", "domain": "online.sberbank.ru", "path": "/", "secure": true}
	for k, v := range cookie {
		item[k] = v
	}
	entries := []any{}
	if prior {
		entries = append(entries, map[string]any{"request": map[string]any{"url": "https://web2.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{map[string]any{"name": "SID", "value": "old-synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true}, map[string]any{"name": "OTHER", "value": "retained-synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true}}}})
	}
	entries = append(entries, map[string]any{"startedDateTime": "2026-07-13T09:00:00Z", "request": map[string]any{"url": "https://web2.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{item}}}, map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}})
	raw, err := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
	if err != nil {
		t.Fatal(err)
	}
	path := clientPrivatePath(t)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestClientCycle3ExpiredNumericOverflowDeletesPriorCookie(t *testing.T) {
	for _, field := range []string{"expires", "maxAge", "max-age"} {
		for _, v := range []any{json.Number("-1e30"), json.Number("-9223372036854775809"), json.Number("-" + strings.Repeat("9", 100))} {
			t.Run(field+"/"+string(v.(json.Number)), func(t *testing.T) {
				path := clientCycle3HAR(t, map[string]any{field: v}, true)
				builds := 0
				c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { builds++; return clientFake(t, b), nil }})
				if err != nil {
					t.Fatal("overflow deletion rejected an otherwise ready synthetic import", err)
				}
				defer c.Close()
				b, err := c.ExportSession()
				if err != nil || builds != 1 || len(b.Cookies) != 1 || b.Cookies[0].Name != "OTHER" {
					t.Fatal("expired overflow was ignored, resurrecting the prior SID")
				}
			})
		}
	}
}

func TestClientCycle3NumericExpiryNormalFormsRemainSupported(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata map[string]any
		expiry   *int64
	}{
		{"absent", nil, nil}, {"null", map[string]any{"expires": nil}, nil},
		{"integer", map[string]any{"expires": json.Number("4102444800")}, func() *int64 { x := int64(4102444800); return &x }()},
		{"fraction", map[string]any{"expires": json.Number("4102444800.75")}, func() *int64 { x := int64(4102444800); return &x }()},
		{"scientific", map[string]any{"expires": json.Number("4.102444800e9")}, func() *int64 { x := int64(4102444800); return &x }()},
		{"RFC3339", map[string]any{"expires": "2100-01-01T00:00:00Z"}, func() *int64 { x := int64(4102444800); return &x }()},
		{"HTTP-date", map[string]any{"expires": "Fri, 01 Jan 2100 00:00:00 GMT"}, func() *int64 { x := int64(4102444800); return &x }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewSberClientFromFiles(clientCycle3HAR(t, tc.metadata, false), "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { return clientFake(t, b), nil }})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			b, err := c.ExportSession()
			if err != nil || len(b.Cookies) != 1 {
				t.Fatal("normal import lost")
			}
			expiry := b.Cookies[0].Expires
			if (expiry == nil) != (tc.expiry == nil) || expiry != nil && *expiry != *tc.expiry {
				t.Fatal("normal source expiry changed")
			}
		})
	}
}

func TestClientCycle3NonFiniteNumericExpiryFailsBeforeConstruction(t *testing.T) {
	for _, field := range []string{"expires", "maxAge"} {
		for _, v := range []string{"-1e999", "1e999"} {
			t.Run(field+"/"+v, func(t *testing.T) {
				path := clientCycle3HAR(t, map[string]any{field: json.Number(v)}, false)
				builds := 0
				c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { builds++; return clientFake(t, b), nil }})
				if c != nil {
					c.Close()
				}
				if err == nil || c != nil || builds != 0 {
					t.Fatal("unsupported explicit numeric expiry became session lifetime")
				}
			})
		}
	}
}
