package sber

import (
	"os"
	"path/filepath"
	"testing"
)

// The HAR/Netscape structures below are copied from the audited synthetic
// test_sber_client.py fixtures; never captured browser or owner state.
func TestClientFromFilesImportsObservedHostsAndPrivateCookieMetadata(t *testing.T) {
	d := t.TempDir()
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
	c, e := NewSberClientFromFiles(har, cookies, ClientOptions{TransportOptions: TransportOptions{CABundle: "synthetic-ca"}, TransportFactory: func(b SessionBundle, o TransportOptions) (Transport, error) {
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
	c, e := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b SessionBundle, _ TransportOptions) (Transport, error) { return clientFake(t, b), nil }})
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
