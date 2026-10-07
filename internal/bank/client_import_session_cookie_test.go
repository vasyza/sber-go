package bank

import (
	"encoding/json"
	"os"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// Chromium's protocol uses -1 for a session cookie's unset expiry; DevTools
// serializes that as a Date one second before the Unix epoch in HAR exports.
func TestHARSessionCookieExpirySentinelDoesNotDeleteObservedCookies(t *testing.T) {
	for _, side := range []string{"request", "response"} {
		for _, tc := range []struct {
			name    string
			expires any
			maxAge  any
			reject  bool
		}{
			{"chromium-date", "1969-12-31T23:59:59.000Z", nil, false},
			{"numeric-negative-is-expired", json.Number("-1"), nil, true},
			{"explicit-deletion", "1969-12-31T23:59:59.000Z", "0", true},
			{"expired-date", "1969-12-31T23:59:58.000Z", nil, true},
			{"fractional-negative", json.Number("-1.5"), nil, true},
		} {
			t.Run(side+"/"+tc.name, func(t *testing.T) {
				cookie := map[string]any{"name": "SID", "value": "synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true, "httpOnly": true, "sameSite": "Strict", "expires": tc.expires}
				if tc.maxAge != nil {
					cookie["maxAge"] = tc.maxAge
				}
				entry := map[string]any{"request": map[string]any{"method": "GET", "url": "https://web2.online.sberbank.ru/main"}, "response": map[string]any{}}
				entry[side].(map[string]any)["cookies"] = []any{cookie}
				entries := []any{entry, map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}}}
				raw, err := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
				if err != nil {
					t.Fatal(err)
				}
				path := clientPrivatePath(t)
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				client, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					return clientFake(t, b), nil
				}})
				if tc.reject {
					if client != nil {
						client.Close()
					}
					if err == nil {
						t.Fatal("expired or deleted cookie made a usable session")
					}
					return
				}
				if err != nil {
					t.Fatal("session cookie sentinel deleted an observed cookie")
				}
				defer client.Close()
				bundle, err := client.ExportSession()
				if err != nil || len(bundle.Cookies) != 1 {
					t.Fatal("session cookie lost")
				}
				cookieRecord := bundle.Cookies[0]
				if cookieRecord.Expires != nil || !cookieRecord.HTTPOnly || !cookieRecord.Secure || cookieRecord.SameSite == nil || *cookieRecord.SameSite != "Strict" {
					t.Fatal("session cookie metadata changed")
				}
			})
		}
	}
}

func TestHARSessionExpirySentinelCannotOverrideSetCookieDeletion(t *testing.T) {
	path := clientPrivatePath(t)
	raw := `{"log":{"entries":[{"request":{"url":"https://web2.online.sberbank.ru/main","cookies":[{"name":"SID","value":"synthetic","domain":"online.sberbank.ru","path":"/","secure":true,"expires":"1969-12-31T23:59:59.000Z"}]},"response":{"headers":[{"name":"Set-Cookie","value":"SID=; Domain=online.sberbank.ru; Path=/; Max-Age=0; Secure"}]}},{"request":{"url":"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}}]}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	builds := 0
	client, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		builds++
		return clientFake(t, b), nil
	}})
	if client != nil {
		client.Close()
	}
	if err == nil || builds != 0 {
		t.Fatal("HAR sentinel overrode an explicit server deletion")
	}
}
