package sber

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestBrowserBootstrapSnapshotContractAndRedaction(t *testing.T) {
	result := BrowserBootstrapResult{HTML: `<script>window.config = {processId:"synthetic-html-process-secret"};</script>`, URL: PublicBootstrapURL, Cookies: syntheticBundle().Cookies, Browser: syntheticBundle().Browser}
	normalized, err := NewBrowserBootstrapResult(result)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Cookies[0].Domain != "online.sberbank.ru" || normalized.HTML != result.HTML {
		t.Fatal("snapshot contract changed")
	}
	for _, s := range []string{fmt.Sprintf("%#v", normalized), fmt.Sprintf("%+v", normalized)} {
		if strings.Contains(s, "synthetic") {
			t.Fatal("snapshot formatting leaked")
		}
	}
	raw, _ := json.Marshal(normalized)
	if strings.Contains(string(raw), "synthetic") {
		t.Fatal("snapshot JSON leaked")
	}
	bad := result
	bad.Cookies = append(bad.Cookies, bad.Cookies[0])
	if _, err := NewBrowserBootstrapResult(bad); err == nil {
		t.Fatal("duplicate cookies accepted")
	}
	bad = result
	bad.HTML = ""
	if _, err := NewBrowserBootstrapResult(bad); err == nil {
		t.Fatal("empty HTML accepted")
	}
	calls := 0
	provider := BrowserBootstrapFunc(func(ctx context.Context, b SessionBundle, u string) (BrowserBootstrapResult, error) {
		calls++
		return normalized, nil
	})
	returned, err := provider.Bootstrap(context.Background(), unreadyBundle(), PublicBootstrapURL)
	if err != nil || calls != 1 || returned.HTML != normalized.HTML {
		t.Fatal("provider adapter failed")
	}
}
func TestRecognizableBrowserCheckOnly(t *testing.T) {
	shell := `<script src="/TSPD/synthetic.js"></script><noscript>Please enable JavaScript</noscript>`
	if !IsBrowserCheck(shell) {
		t.Fatal("observed JS shell not recognized")
	}
	for _, html := range []string{"", `<script>enable JavaScript</script>`, shell + `<script>window . config = {};</script>`} {
		if IsBrowserCheck(html) {
			t.Fatal("normal/malformed config treated as JS check")
		}
	}
}
