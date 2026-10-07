package sber

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestClientReadDecodesOriginalBytesAndEnforcesSourceGate(t *testing.T) {
	cases := []struct {
		name, body, ctype, kind string
		status                  int
	}{
		{"boolean", `{"success":true,"n":9007199254740993}`, "application/json", "ok", 200},
		{"string", `{"success":"true"}`, "application/json", "ok", 200},
		{"numeric", `{"success":1}`, "application/json", "rejected", 200},
		{"false", `{"success":false}`, "application/json", "rejected", 200},
		{"missing", `{}`, "application/json", "rejected", 200},
		{"nested", `{"success":true,"value":{"x":1,"x":2}}`, "application/json", "api", 200},
		{"escaped-key", `{"success":false,"\u0073uccess":true}`, "application/json", "api", 200},
		{"surrogate", `{"success":true,"id":"\ud800"}`, "application/json", "api", 200},
		{"surrogate-low", `{"success":true,"id":"\udfff"}`, "application/json", "api", 200},
		{"invalid-utf8", string([]byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'}), "application/json", "api", 200},
		{"trailing", `{"success":true} {}`, "application/json", "api", 200},
		{"list", `[]`, "application/json", "api", 200},
		{"null", `null`, "application/json", "api", 200},
		{"malformed", `{`, "application/json", "api", 200},
		{"html", `<html>secret</html>`, "TEXT/HTML; charset=utf-8", "expired", 200},
		{"bad-gateway-html", `<html>secret</html>`, "text/html", "api", 502},
		{"no-content-read", ``, "application/json", "api", 204},
		{"301", ``, "text/html", "expired", 301}, {"302", ``, "", "expired", 302},
		{"303", ``, "", "expired", 303}, {"307", ``, "", "expired", 307}, {"308", ``, "", "expired", 308},
		{"401", ``, "", "expired", 401}, {"403", ``, "", "expired", 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := clientFixture(t, "read")
			tr := clientFake(t, b)
			r := clientResponse(tc.status, tc.body)
			r.Headers = http.Header{"cOnTeNt-TyPe": []string{tc.ctype}}
			original := append([]byte(nil), r.Content...)
			tr.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) { return r, nil }
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			got, e := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", map[string]any{"paginationOffset": 0})
			var api *APIError
			var rejected *APIRejected
			var expired *AuthenticationExpired
			switch tc.kind {
			case "ok":
				if e != nil || got["success"] == nil {
					t.Fatalf("expected success: %v", e)
				}
				if tc.name == "boolean" && got["n"] != json.Number("9007199254740993") {
					t.Fatal("number rounded")
				}
			case "api":
				if !errors.As(e, &api) {
					t.Fatalf("expected API error: %v", e)
				}
			case "rejected":
				if !errors.As(e, &rejected) {
					t.Fatalf("expected rejected: %v", e)
				}
			case "expired":
				if !errors.As(e, &expired) {
					t.Fatalf("expected expired: %v", e)
				}
			}
			if !bytes.Equal(r.Content, original) {
				t.Fatal("response rewritten")
			}
			calls := tr.snapshotCalls()
			if len(calls) != 1 || calls[0].method != "POST" || calls[0].target != b.APIBase+"/uoh-bh/v1/operations/list" || calls[0].options.AcceptEncoding != "gzip, deflate, br, zstd" {
				t.Fatal("request changed or repeated")
			}
		})
	}
	b := clientFixture(t, "allowlist")
	tr := clientFake(t, b)
	c, e := NewSberClient(b, ClientOptions{Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	paths := []string{"/main-screen/rest/v2/m1/web/section/meta", "/uoh-bh/v1/operations/list", "/uoh-bh/v1/operation/details", "/ufs-carddetail/rest/card/v1/cardInfo", "/pfpv_alf_mb/v1.00/alf/amounts"}
	for _, p := range paths {
		if _, e := c.PostRead(context.Background(), p, map[string]any{}); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []string{"/api/warmUpSession", "/me2me/v1/workflow", "/ufs-productdetail/rest/v1/changeProductName", "/bh-confirmation/v3/workflow2", "/uoh-bh/v1/operations/list?injected=x", "https://other.invalid/", ""} {
		if _, e := c.PostRead(context.Background(), p, nil); e == nil {
			t.Fatal("noncanonical path accepted")
		}
	}
	calls, _, _ := tr.counts()
	if calls != len(paths) {
		t.Fatal("allowlist side effect")
	}
}
