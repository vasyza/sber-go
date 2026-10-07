package bank

import (
	"encoding/json"
	"os"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientHARUnsupportedCookieScalarsFailClosedInsteadOfChangingIdentity(t *testing.T) {
	for _, tc := range []struct {
		field string
		value any
	}{{"secure", "false"}, {"httpOnly", "false"}, {"hostOnly", "false"}, {"name", 123}, {"value", 123}, {"domain", 123}, {"path", 123}} {
		t.Run(tc.field, func(t *testing.T) {
			cookie := map[string]any{"name": "SID", "value": "synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true}
			cookie[tc.field] = tc.value
			entries := []any{map[string]any{"request": map[string]any{"method": "GET", "url": "https://web2.online.sberbank.ru/main"}, "response": map[string]any{"cookies": []any{cookie}}}, map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}}}
			raw, e := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
			if e != nil {
				t.Fatal(e)
			}
			path := clientPrivatePath(t)
			if e = os.WriteFile(path, raw, 0600); e != nil {
				t.Fatal(e)
			}
			builds := 0
			c, e := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				builds++
				return clientFake(t, b), nil
			}})
			if c != nil {
				c.Close()
			}
			if e == nil || builds != 0 {
				t.Fatal("unsupported cookie scalar silently repaired")
			}
		})
	}
}
