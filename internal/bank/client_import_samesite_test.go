package bank

import (
	"encoding/json"
	"os"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestHARCookieSameSiteCaseRetainsPolicyAndRejectsUnknownValues(t *testing.T) {
	for _, tc := range []struct {
		policy any
		want   string
	}{
		{"lax", "Lax"}, {"strict", "Strict"}, {"none", "None"}, {"nOnE", "None"},
		{"unspecified", ""}, {" none", ""}, {"None ", ""}, {"", ""}, {true, ""},
	} {
		t.Run(tc.want+"-policy", func(t *testing.T) {
			entries := []any{
				map[string]any{"request": map[string]any{"method": "GET", "url": "https://web2.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{map[string]any{"name": "SID", "value": "synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true, "httpOnly": true, "sameSite": tc.policy}}}},
				map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}},
			}
			raw, err := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
			if err != nil {
				t.Fatal(err)
			}
			path := clientPrivatePath(t)
			if err = os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			builds := 0
			client, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				builds++
				return clientFake(t, b), nil
			}})
			if tc.want == "" {
				if client != nil {
					client.Close()
				}
				if err == nil || builds != 0 {
					t.Fatal("unknown cookie policy accepted")
				}
				return
			}
			if err != nil {
				t.Fatal("valid cookie policy spelling rejected")
			}
			defer client.Close()
			bundle, err := client.ExportSession()
			if err != nil || len(bundle.Cookies) != 1 {
				t.Fatal("cookie lost")
			}
			cookie := bundle.Cookies[0]
			if cookie.SameSite == nil || *cookie.SameSite != tc.want || !cookie.Secure || !cookie.HTTPOnly || cookie.HostOnly || cookie.Path != "/" {
				t.Fatal("cookie policy or scope changed")
			}
		})
	}
}
