package sber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestClientCopiesAuthOptionIdentityBeforeDeferredRenewal(t *testing.T) {
	b := clientFixture(t, "option-old")
	fresh := clientFixture(t, "option-new")
	path := clientPrivatePath(t)
	if e := b.Save(path); e != nil {
		t.Fatal(e)
	}
	old := clientFake(t, b)
	next := clientFake(t, fresh)
	old.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
		return clientResponse(401, `{}`), nil
	}
	identity, antifraud := "synthetic-auth-identity", "synthetic-antifraud-identity"
	browser := BrowserProfile{Headers: []BrowserHeader{{Name: "user-agent", Value: "synthetic-original-UA"}}}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: AuthOptions{Deviceprint: &identity, AntifraudDeviceprint: &antifraud, Browser: browser}, TransportFactory: func(SessionBundle, TransportOptions) (Transport, error) { return next, nil }, Renewal: func(_ context.Context, _ SessionBundle, _ PINProvider, o AuthOptions) (SessionBundle, error) {
		if *o.Deviceprint != "synthetic-auth-identity" || *o.AntifraudDeviceprint != "synthetic-antifraud-identity" || o.Browser.Headers[0].Value != "synthetic-original-UA" {
			t.Error("caller aliased deferred auth identity")
		}
		return fresh, nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	identity = "changed"
	antifraud = "changed"
	browser.Headers[0].Value = "changed"
	if _, e = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil); e != nil {
		t.Fatal(e)
	}
}
func TestClientRejectionMetadataIsExplicitBoundedAndNotInDiagnostics(t *testing.T) {
	marker := "synthetic-private-remote-marker"
	for _, tc := range []struct {
		name, field string
		value       any
		want        string
	}{
		{"code-limit", "code", strings.Repeat("界", 128), strings.Repeat("界", 128)},
		{"code-long", "code", strings.Repeat("界", 129), ""}, {"title-limit", "title", strings.Repeat("界", 4096), strings.Repeat("界", 4096)},
		{"title-long", "title", strings.Repeat("a", 4097), ""}, {"text-tabs", "text", marker + "\t\n", marker + "\t\n"}, {"text-control", "text", marker + "\r", ""},
		{"uuid", "uuid", marker, marker}, {"uuid-long", "uuid", strings.Repeat("a", 257), ""}, {"system", "system", marker, marker}, {"system-del", "system", marker + "\x7f", ""},
		{"number", "code", json.Number("123"), ""}, {"empty", "title", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, e := json.Marshal(map[string]any{"success": false, "error": map[string]any{tc.field: tc.value}})
			if e != nil {
				t.Fatal(e)
			}
			b := clientFixture(t, "metadata")
			tr := clientFake(t, b)
			tr.post = func(context.Context, string, map[string]any, RequestOptions) (*Response, error) {
				return clientResponse(200, string(raw)), nil
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, e = c.PostRead(context.Background(), "/ufs-carddetail/rest/card/v1/cardInfo", nil)
			var rejected *APIRejected
			if !errors.As(e, &rejected) {
				t.Fatal("API rejection type lost")
			}
			got := map[string]string{"code": rejected.Code, "title": rejected.Title, "text": rejected.Text, "uuid": rejected.UUID, "system": rejected.System}[tc.field]
			if got != tc.want {
				t.Fatal("metadata bound changed")
			}
			if strings.Contains(rejected.Message, marker) {
				t.Fatal("dynamic message captured metadata")
			}
			for _, s := range []string{e.Error(), fmt.Sprintf("%v", e), fmt.Sprintf("%+v", e), fmt.Sprintf("%#v", e)} {
				if strings.Contains(s, marker) || strings.Contains(s, "界") {
					t.Fatal("error diagnostics exposed metadata")
				}
			}
			marshaled, err := json.Marshal(e)
			if err != nil || strings.Contains(string(marshaled), marker) {
				t.Fatal("JSON diagnostics exposed metadata")
			}
		})
	}
}
