package bank

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

func TestHARSeamlessMainResponseImportsEffectiveOriginWithoutNetwork(t *testing.T) {
	path := clientPrivatePath(t)
	entry := syntheticHARSeamlessMain("https://web2.online.sberbank.ru/main", 200)
	entry["request"].(map[string]any)["cookies"] = []any{map[string]any{
		"name": "SID", "value": "synthetic", "domain": "online.sberbank.ru", "path": "/", "secure": true, "httpOnly": true, "sameSite": "Lax",
	}}
	raw, err := json.Marshal(map[string]any{"log": map[string]any{"entries": []any{entry}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var tr *clientFakeTransport
	client, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		tr = clientFake(t, b)
		return tr, nil
	}})
	if err != nil {
		t.Fatal("seamless main response was not imported")
	}
	defer client.Close()
	b, err := client.ExportSession()
	if err != nil || b.WebBase != "https://web2.online.sberbank.ru" || b.APIBase != "https://web-node2.online.sberbank.ru" || len(b.Cookies) != 1 {
		t.Fatal("effective main response origin or runtime API origin lost")
	}
	if b.Cookies[0].SameSite == nil || *b.Cookies[0].SameSite != "Lax" || !b.Cookies[0].HTTPOnly {
		t.Fatal("cookie metadata lost")
	}
	if calls, _, _ := tr.counts(); calls != 0 {
		t.Fatal("HAR import performed a request")
	}
}

func TestHARSeamlessMainResponseRejectsUnsafeEffectiveURLs(t *testing.T) {
	for _, target := range []string{
		"http://web2.online.sberbank.ru/main", "https://example.test/main", "https://web2.online.sberbank.ru/main/other",
		"https://web2.online.sberbank.ru/main?ticket=synthetic", "https://web2.online.sberbank.ru/main#fragment",
		"https://user@web2.online.sberbank.ru/main", "https://web-node2.online.sberbank.ru/main",
	} {
		t.Run(target, func(t *testing.T) {
			b, err := clientBundleFromHAR(map[string]any{"log": map[string]any{"entries": []any{syntheticHARSeamlessMain(target, 200)}}})
			if err == nil && b.WebBase != "" {
				t.Fatal("unsafe effective URL imported")
			}
		})
	}
	for _, status := range []int{302, 403, 500} {
		b, err := clientBundleFromHAR(map[string]any{"log": map[string]any{"entries": []any{syntheticHARSeamlessMain("https://web2.online.sberbank.ru/main", status)}}})
		if err == nil && b.WebBase != "" {
			t.Fatal("unsuccessful response selected a main origin")
		}
	}
	entry := syntheticHARSeamlessMain("https://web2.online.sberbank.ru/main", 200)
	response := entry["response"].(map[string]any)
	response["headers"] = append(response["headers"].([]any), map[string]any{"name": "x-response-url", "value": "https://web3.online.sberbank.ru/main"})
	b, err := clientBundleFromHAR(map[string]any{"log": map[string]any{"entries": []any{entry}}})
	if err == nil && b.WebBase != "" {
		t.Fatal("ambiguous effective origin imported")
	}
}

func syntheticHARSeamlessMain(target string, status int) map[string]any {
	return map[string]any{
		"request":  map[string]any{"method": "POST", "url": "https://web2.online.sberbank.ru/seamlessLogin?ticket=synthetic", "headers": []any{}},
		"response": map[string]any{"status": json.Number(strconv.Itoa(status)), "headers": []any{map[string]any{"name": "X-Response-URL", "value": target}}, "content": map[string]any{"text": `{"ufs.block.root.url":"https://web-node2.online.sberbank.ru"}`}},
	}
}
