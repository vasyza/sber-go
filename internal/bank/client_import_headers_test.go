package bank

import (
	"encoding/json"
	"os"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestClientHARHeadersRetainSourceInsertionOrderAndLatestValues(t *testing.T) {
	entries := []any{
		map[string]any{"request": map[string]any{"method": "GET", "url": "https://web2.online.sberbank.ru/main"}},
		map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list", "headers": []any{map[string]any{"name": "User-Agent", "value": "initial UA"}, map[string]any{"name": "Accept", "value": "application/json"}, map[string]any{"name": "x-private-ignored", "value": "ignored"}}}},
		map[string]any{"request": map[string]any{"method": "POST", "url": "https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list", "headers": []any{map[string]any{"name": "User-Agent", "value": "latest UA"}, map[string]any{"name": "Priority", "value": "u=1"}, map[string]any{"name": "Cookie", "value": "SID=synthetic"}}, "cookies": []any{map[string]any{"name": "SID", "value": "synthetic", "domain": "online.sberbank.ru", "path": "/"}}}},
	}
	raw, e := json.Marshal(map[string]any{"log": map[string]any{"entries": entries}})
	if e != nil {
		t.Fatal(e)
	}
	path := clientPrivatePath(t)
	if e = os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 24; i++ {
		c, e := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			return clientFake(t, b), nil
		}})
		if e != nil {
			t.Fatal(e)
		}
		b, e := c.ExportSession()
		c.Close()
		if e != nil {
			t.Fatal(e)
		}
		h := b.Browser.Headers
		if len(h) != 3 || h[0].Name != "user-agent" || h[0].Value != "latest UA" || h[1].Name != "accept" || h[2].Name != "priority" {
			t.Fatal("observable source header order changed")
		}
	}
}
