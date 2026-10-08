package bank

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
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
			c, e := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return clientFake(t, b), nil
			}})
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

// The HAR/Netscape structures below are copied from the audited synthetic
// test_sber_client.py fixtures; never captured browser or owner state.
func TestClientFromFilesImportsObservedHostsAndPrivateCookieMetadata(t *testing.T) {
	d := testPrivateDir(t)
	if e := os.Chmod(d, 0700); e != nil {
		t.Fatal(e)
	}
	har := filepath.Join(d, "synthetic.har")
	cookies := filepath.Join(d, "synthetic-cookies.txt")
	raw := `{"log":{"entries":[{"request":{"method":"GET","url":"https://web2.online.sberbank.ru/main","headers":[]},"response":{"headers":[],"content":{"text":""}}},{"request":{"method":"POST","url":"https://web-node2.online.sberbank.ru/main-screen/rest/v2/m1/web/section/meta","headers":[{"name":"User-Agent","value":"Observed UA"}]},"response":{"headers":[],"content":{}}}]}}`
	if e := os.WriteFile(har, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(cookies, []byte("# Netscape HTTP Cookie File\n#HttpOnly_.online.sberbank.ru\tTRUE\t/\tTRUE\t4102444800\tUFS-SESSION\tsecret\n.sberbank.ru\tTRUE\t/\tFALSE\t1\tOLD\texpired\n"), 0600); e != nil {
		t.Fatal(e)
	}
	var tr *clientFakeTransport
	c, e := NewSberClientFromFiles(har, cookies, ClientOptions{TransportOptions: sdkTransport.TransportOptions{CABundle: "synthetic-ca"}, TransportFactory: func(b sdkSession.SessionBundle, o sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		if o.CABundle != "synthetic-ca" {
			t.Error("CA lost")
		}
		tr = clientFake(t, b)
		return tr, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	b, e := c.ExportSession()
	if e != nil || b.APIBase != "https://web-node2.online.sberbank.ru" || b.WebBase != "https://web2.online.sberbank.ru" || len(b.Cookies) != 1 || b.Browser.AsMap()["user-agent"] != "Observed UA" {
		t.Fatal("HAR import mismatch")
	}
	cookie := b.Cookies[0]
	if cookie.Name != "UFS-SESSION" || cookie.HostOnly || !cookie.Secure || !cookie.HTTPOnly || cookie.Expires == nil || *cookie.Expires != 4102444800 {
		t.Fatal("cookie metadata lost")
	}
	calls, _, _ := tr.counts()
	if calls != 0 {
		t.Fatal("import connected to bank")
	}
	if e := os.Chmod(har, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = NewSberClientFromFiles(har, cookies, ClientOptions{}); e == nil {
		t.Fatal("nonprivate HAR accepted")
	}
	if e := os.Chmod(har, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(cookies, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = NewSberClientFromFiles(har, cookies, ClientOptions{}); e == nil {
		t.Fatal("nonprivate cookies accepted")
	}
}
func TestClientFromFilesKeepsRotationDeletionRuntimeConfigAndMutationIdentity(t *testing.T) {
	path := clientPrivatePath(t)
	raw := `{"log":{"entries":[{"startedDateTime":"2026-07-13T09:00:00Z","request":{"method":"GET","url":"https://web2.online.sberbank.ru/main","headers":[]},"response":{"headers":[{"name":"Set-Cookie","value":"SID=old; Domain=online.sberbank.ru; Path=/; Secure; HttpOnly; SameSite=Lax"}],"content":{"text":"{\"ufs.block.root.url\":\"https://web-standin2.online.sberbank.ru\"}"}}},{"startedDateTime":"2026-07-13T09:01:00Z","request":{"method":"POST","url":"https://web-node2.online.sberbank.ru/ufs-productdetail/rest/v1/changeProductName","headers":[{"name":"RSA-Antifraud-Device-Print","value":"version%3Dfixture"}]},"response":{"cookies":[{"name":"SID","value":"new","domain":"online.sberbank.ru","path":"/","secure":true,"httpOnly":true,"sameSite":"Strict"}],"headers":[{"name":"Set-Cookie","value":"temporary=; Domain=online.sberbank.ru; Path=/; Max-Age=0; Secure"}],"content":{}}}]}}`
	if e := os.WriteFile(path, []byte(raw), 0600); e != nil {
		t.Fatal(e)
	}
	c, e := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return clientFake(t, b), nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	b, e := c.ExportSession()
	if e != nil || b.APIBase != "https://web-standin2.online.sberbank.ru" || len(b.Cookies) != 1 || b.Cookies[0].Value != "new" || b.AntifraudDeviceprint == nil || *b.AntifraudDeviceprint != "version%3Dfixture" {
		t.Fatal("observed import state lost")
	}
	if b.Cookies[0].SameSite == nil || *b.Cookies[0].SameSite != "Strict" {
		t.Fatal("structured SameSite silently discarded")
	}
}

func TestClientCycle3BooleanTimestampRetainsSourceExpiredSemantics(t *testing.T) {
	for _, value := range []bool{false, true} {
		t.Run(map[bool]string{false: "false", true: "true"}[value], func(t *testing.T) {
			builds := 0
			c, err := NewSberClientFromFiles(clientCycle3HAR(t, map[string]any{"expires": value}, false), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				builds++
				return clientFake(t, b), nil
			}})
			if c != nil {
				c.Close()
			}
			if err == nil || c != nil || builds != 0 {
				t.Fatal("source numeric boolean expiry became a live session cookie")
			}
		})
	}
}

func TestClientCycle3FutureNumericOverflowKeepsExplicitBoundedLifetime(t *testing.T) {
	for _, number := range []string{"1e30", "9223372036854775808", "999999999999999999999999999999999999999999999999999999"} {
		t.Run(number, func(t *testing.T) {
			c, err := NewSberClientFromFiles(clientCycle3HAR(t, map[string]any{"expires": json.Number(number)}, false), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return clientFake(t, b), nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			b, err := c.ExportSession()
			if err != nil || b.Cookies[0].Expires == nil || *b.Cookies[0].Expires != math.MaxInt64 {
				t.Fatal("explicit future lifetime erased or narrowed")
			}
		})
	}
}

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
				c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					builds++
					return clientFake(t, b), nil
				}})
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
			c, err := NewSberClientFromFiles(clientCycle3HAR(t, tc.metadata, false), "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				return clientFake(t, b), nil
			}})
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
				c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					builds++
					return clientFake(t, b), nil
				}})
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
