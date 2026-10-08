package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	sdkAuth "github.com/vasyza/sber-go/internal/auth"
	sdkErrs "github.com/vasyza/sber-go/internal/errs"
	sdkSession "github.com/vasyza/sber-go/internal/session"
	sdkTransport "github.com/vasyza/sber-go/internal/transport"
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
	old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	identity, antifraud := "synthetic-auth-identity", "synthetic-antifraud-identity"
	browser := sdkSession.BrowserProfile{Headers: []sdkSession.BrowserHeader{{Name: "user-agent", Value: "synthetic-original-UA"}}}
	c, e := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: sdkAuth.AuthOptions{Deviceprint: &identity, AntifraudDeviceprint: &antifraud, Browser: browser}, TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
		return next, nil
	}, Renewal: func(_ context.Context, _ sdkSession.SessionBundle, _ sdkAuth.PINProvider, o sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
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
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(200, string(raw)), nil
			}
			c, e := NewSberClient(b, ClientOptions{Transport: tr})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			_, e = c.PostRead(context.Background(), "/ufs-carddetail/rest/card/v1/cardInfo", nil)
			var rejected *sdkErrs.APIRejected
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

// Derived from actual licensed Python3.12 helper execution, not a native oracle.
// Original HARs, source hashes, raw stdout and full denominator remain in evidence.
const clientCycle3SourceExpiryCorpus = `[{"id":"absent","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"null","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":null}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"false","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":false}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"true","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":true}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"invalid-string","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":\"not-a-date\"}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"numeric-string","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":\"4102444800\"}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"RFC3339","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":\"2100-01-01T00:00:00Z\"}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"HTTP-date","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":\"Fri, 01 Jan 2100 00:00:00 GMT\"}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"naive-ISO","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":\"2100-01-01T00:00:00\"}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"expires-0","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":0}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires--0.75","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":-0.75}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires--1","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":-1}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires--9223372036854775809","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":-9223372036854775809}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires--1e30","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":-1e+30}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires--9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":-9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires-1","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":1}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires-4102444800","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"expires-4102444800.75","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":4102444800.75}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"expires-4.102444800e9","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":4102444800.0}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"expires-9223372036854775807","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":9223372036854775807}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"9223372036854775807","intentional_native_bound":"explicit positive expiry clamped to native int64 maximum; future lifetime retained"},{"id":"expires-9223372036854775808","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":9223372036854775808}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"9223372036854775807","intentional_native_bound":"explicit positive expiry clamped to native int64 maximum; future lifetime retained"},{"id":"expires-1e30","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":1e+30}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"9223372036854775807","intentional_native_bound":"explicit positive expiry clamped to native int64 maximum; future lifetime retained"},{"id":"expires-9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"9223372036854775807","intentional_native_bound":"explicit positive expiry clamped to native int64 maximum; future lifetime retained"},{"id":"expires--1e999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":-1e999}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"expires-1e999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"expires\":1e999}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-0","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":0,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge--1","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":-1,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge--0.75","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":-0.75,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge--1e30","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":-1e+30,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge--9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":-9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-3153600000","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":3153600000,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4937533200","intentional_native_bound":null},{"id":"maxAge-1e30","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":1e+30,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":false,"native_expected_expiry":"9223372036854775807","intentional_native_bound":"explicit native int64 Max-Age addition overflow fails closed (source future cookie)"},{"id":"maxAge--1e999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":-1e999,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-1e999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":1e999,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-string-bool-0","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":\"0\",\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-string-bool--99999999999999999999999999999999999999999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":\"-99999999999999999999999999999999999999999\",\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-string-bool-invalid","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":\"invalid\",\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"maxAge-string-bool-True","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":true,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"maxAge-string-bool-False","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"maxAge\":false,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-0","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":0,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age--1","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":-1,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age--0.75","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":-0.75,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age--1e30","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":-1e+30,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age--9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":-9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-3153600000","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":3153600000,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4937533200","intentional_native_bound":null},{"id":"max-age-1e30","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":1e+30,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":false,"native_expected_expiry":"9223372036854775807","intentional_native_bound":"explicit native int64 Max-Age addition overflow fails closed (source future cookie)"},{"id":"max-age--1e999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":-1e999,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-1e999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":1e999,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-string-bool-0","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":\"0\",\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-string-bool--99999999999999999999999999999999999999999","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":\"-99999999999999999999999999999999999999999\",\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-string-bool-invalid","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":\"invalid\",\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":true,"native_expected_accepted":true,"native_expected_expiry":"4102444800","intentional_native_bound":null},{"id":"max-age-string-bool-True","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":true,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null},{"id":"max-age-string-bool-False","har":"{\"log\":{\"entries\":[{\"startedDateTime\":\"2026-07-13T09:00:00Z\",\"request\":{\"method\":\"GET\",\"url\":\"https://web2.online.sberbank.ru/main\"},\"response\":{\"cookies\":[{\"name\":\"SID\",\"value\":\"synthetic-source-expiry-only\",\"domain\":\"online.sberbank.ru\",\"path\":\"/\",\"secure\":true,\"max-age\":false,\"expires\":4102444800}]}},{\"request\":{\"method\":\"POST\",\"url\":\"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list\"}}]}}","source_accepted":false,"native_expected_accepted":false,"native_expected_expiry":null,"intentional_native_bound":null}]`

func TestClientCycle3CanonicalExpiryDifferential(t *testing.T) {
	var cases []struct {
		ID               string  `json:"id"`
		HAR              string  `json:"har"`
		SourceAccepted   bool    `json:"source_accepted"`
		ExpectedAccepted bool    `json:"native_expected_accepted"`
		ExpectedExpiry   *string `json:"native_expected_expiry"`
		NativeBound      *string `json:"intentional_native_bound"`
	}
	if err := json.Unmarshal([]byte(clientCycle3SourceExpiryCorpus), &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.ID, func(t *testing.T) {
			path := clientPrivatePath(t)
			if err := os.WriteFile(path, []byte(tc.HAR), 0600); err != nil {
				t.Fatal(err)
			}
			builds := 0
			c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				builds++
				return clientFake(t, b), nil
			}})
			accepted := c != nil && err == nil
			var expiry *int64
			count := 0
			if c != nil {
				defer c.Close()
				b, e := c.ExportSession()
				if e != nil {
					t.Fatal(e)
				}
				count = len(b.Cookies)
				if count == 1 {
					expiry = b.Cookies[0].Expires
				}
			}
			if accepted != tc.ExpectedAccepted {
				t.Error("native expiry readiness disagrees with source/published native bound")
			}
			if !accepted && builds != 0 {
				t.Error("rejected source expiry constructed a transport")
			}
			if accepted {
				if count != 1 || builds != 1 {
					t.Error("accepted source cookie lost")
				}
				if (expiry == nil) != (tc.ExpectedExpiry == nil) {
					t.Error("explicit source expiry became absent")
				}
				if tc.ExpectedExpiry != nil {
					n, e := strconv.ParseInt(*tc.ExpectedExpiry, 10, 64)
					if e != nil || expiry == nil || *expiry != n {
						t.Error("bounded source expiry changed")
					}
				}
			}
			row := map[string]any{"id": tc.ID, "native_accepted": accepted, "source_accepted": tc.SourceAccepted, "native_cookie_count": count, "native_expiry": expiry, "factory_builds": builds, "intentional_native_bound": tc.NativeBound}
			raw, e := json.Marshal(row)
			if e != nil {
				t.Fatal(e)
			}
			t.Logf("CYCLE3-EXPIRY-RESULT %s", raw)
		})
	}
}

func TestClientCycle3OpaqueValueDiagnosticsPreserveExplicitExports(t *testing.T) {
	b := clientFixture(t, "cycle3-private-cookie-canary")
	b.Browser.Headers = []sdkSession.BrowserHeader{{Name: "user-agent", Value: "cycle3-private-header-canary"}}
	b.AntifraudDeviceprint = sdkTransport.PtrString("cycle3-private-antifraud-canary")
	tr := clientFake(t, b)
	c, err := NewSberClient(b, ClientOptions{Transport: tr, SessionPath: "cycle3-private-path-canary"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	value := reflect.ValueOf(c).Elem().Interface()
	private := struct{ client any }{value}
	cases := []struct {
		name  string
		value any
	}{
		{"pointer", c}, {"value", value}, {"private-wrapper", private}, {"private-wrapper-pointer", &private},
		{"slice", []any{value, c}}, {"map", map[string]any{"client": value}},
		{"reflect-value", reflect.ValueOf(value)}, {"private-reflect-field", reflect.ValueOf(private).Field(0)},
	}
	formats := []string{"%v", "%+v", "%#v", "%d", "%p", "%#p", "%w", "%#w", "%x", "%J", "%[0]w", "%w %w"}
	markers := []string{"session-cycle3-private-cookie-canary", "token-cycle3-private-cookie-canary", "cycle3-private-header-canary", "cycle3-private-path-canary", "cycle3-private-antifraud-canary"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, format := range formats {
				t.Run(format, func(t *testing.T) {
					var buf bytes.Buffer
					log.New(&buf, "", 0).Printf(format, tc.value)
					err := fmt.Errorf(format, tc.value)
					texts := []string{fmt.Sprintf(format, tc.value), buf.String(), err.Error(), fmt.Sprintf("%#v", err)}
					buf.Reset()
					slog.New(slog.NewTextHandler(&buf, nil)).Info("synthetic", slog.Any("client", tc.value))
					texts = append(texts, buf.String())
					for _, text := range texts {
						for _, marker := range markers {
							if strings.Contains(text, marker) {
								t.Fatal("ordinary diagnostic exposed private client state")
							}
						}
					}
				})
			}
		})
	}
	// Explicit exports must retain, not zero or mask, real synthetic state.
	session, err := c.ExportSession()
	if err != nil || session.Browser.Headers[0].Value != b.Browser.Headers[0].Value || *session.AntifraudDeviceprint != *b.AntifraudDeviceprint {
		t.Fatal("explicit session state lost")
	}
	credentials, err := c.ExportCredentials()
	if err != nil || credentials.UFSSession != "session-cycle3-private-cookie-canary" || credentials.UFSToken != "token-cycle3-private-cookie-canary" {
		t.Fatal("explicit credentials lost")
	}
	for _, v := range []any{c, value, []any{value, c}, struct{ Client any }{value}} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if strings.Contains(string(raw), marker) {
				t.Fatal("ordinary JSON exposed state")
			}
		}
	}
}

// Independent review probes: only public client calls, existing synthetic
// fixture builders, and in-process transports. No credential POST or network.
// Failing assertions are the required safety contract, not deliberate mutants.
func TestReviewProbeClientIndirectDiagnosticsStayRedacted(t *testing.T) {
	b := clientFixture(t, "review-diagnostic-cookie-canary")
	b.Browser.Headers = []sdkSession.BrowserHeader{{Name: "user-agent", Value: "review-diagnostic-browser-canary"}}
	tr := clientFake(t, b)
	opts := ClientOptions{Transport: tr, SessionPath: "review-diagnostic-path-canary"}
	c, err := NewSberClient(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Reflect indirection models log adapters which receive the concrete
	// exported value. No client method is invoked on the copy, and no unsafe
	// reflection or private field access is used.
	indirect := reflect.ValueOf(c).Elem().Interface()
	cases := []struct {
		name  string
		value any
		verb  string
	}{
		{"pointer-fmt-v", c, "%v"}, {"pointer-fmt-sharp", c, "%#v"},
		{"pointer-unsupported-d", c, "%d"}, {"options-indirect", reflect.ValueOf(&opts).Elem().Interface(), "%#v"},
		{"client-indirect-v", indirect, "%v"}, {"client-indirect-plus", indirect, "%+v"},
		{"client-indirect-sharp", indirect, "%#v"}, {"client-indirect-unsupported-d", indirect, "%d"},
		{"client-indirect-slog", indirect, "slog"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out string
			if tc.verb == "slog" {
				var buf bytes.Buffer
				slog.New(slog.NewTextHandler(&buf, nil)).Info("synthetic diagnostic", slog.Any("client", tc.value))
				out = buf.String()
			} else {
				out = fmt.Sprintf(tc.verb, tc.value)
			}
			cookieLeak := strings.Contains(out, "session-review-diagnostic-cookie-canary") || strings.Contains(out, "token-review-diagnostic-cookie-canary")
			headerLeak := strings.Contains(out, "review-diagnostic-browser-canary")
			pathLeak := strings.Contains(out, "review-diagnostic-path-canary")
			t.Logf("cookie_leak=%t browser_leak=%t path_leak=%t", cookieLeak, headerLeak, pathLeak)
			if cookieLeak || headerLeak || pathLeak {
				t.Fatal("client-owned secret state leaked through ordinary indirect diagnostic")
			}
		})
	}
}

func TestReviewProbeSequenceKeepsUncertaintyWhenCallbackErrors(t *testing.T) {
	for _, kind := range []string{"foreign-error", "callback-canceled", "callback-sdk-error"} {
		t.Run(kind, func(t *testing.T) {
			b := clientFixture(t, "review-sequence")
			b.AntifraudDeviceprint = sdkTransport.PtrString("review-antifraud-canary")
			tr := clientFake(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return nil, context.Canceled
			}
			c, err := NewSberClient(b, ClientOptions{Transport: tr, AllowMutations: true})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			err = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
				_, first := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
				var u *sdkErrs.MutationUncertain
				if !errors.As(first, &u) {
					t.Fatal("send did not establish uncertainty")
				}
				_, second := send(context.Background(), "/me2me/v1/workflow", nil, nil, "/app/test", true)
				if !errors.As(second, &u) {
					t.Fatal("poisoned sender forgot failure")
				}
				switch kind {
				case "callback-canceled":
					return context.Canceled
				case "callback-sdk-error":
					return &sdkErrs.MissingSession{Message: "review-callback-private-canary"}
				default:
					return errors.New("review-callback-private-canary")
				}
			})
			calls, _, _ := tr.counts()
			var uncertain *sdkErrs.MutationUncertain
			keepUncertain := errors.As(err, &uncertain)
			keepCancel := errors.Is(err, context.Canceled)
			t.Logf("business_posts=%d mutation_uncertain=%t send_cancel_identity=%t", calls, keepUncertain, keepCancel)
			if calls != 1 {
				t.Fatal("financial POST replayed")
			}
			if !keepUncertain || !keepCancel {
				t.Fatal("sequence discarded established after-send uncertainty/cancellation for unrelated callback error")
			}
		})
	}
}

// All methods are real Transport methods promoted from the existing fixture.
// This is a supported Go interface implementation whose dynamic type contains
// a slice, so it is non-comparable although its underlying owner is the same.
type reviewValueTransport struct {
	*clientFakeTransport
	nonComparable  []byte
	blockedRetries *atomic.Int32
}

func (v reviewValueTransport) Post(ctx context.Context, target string, payload map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
	_, _, closed := v.counts()
	if closed > 0 {
		v.blockedRetries.Add(1)
		return nil, sdkErrs.ErrClosed
	}
	return v.clientFakeTransport.Post(ctx, target, payload, o)
}

func TestReviewProbeReusedNonComparableTransportFailsClosed(t *testing.T) {
	b := clientFixture(t, "review-value-old")
	fresh := clientFixture(t, "review-value-new")
	path := clientPrivatePath(t)
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	base := clientFake(t, b)
	base.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		return clientResponse(401, `{}`), nil
	}
	var blocked atomic.Int32
	same := reviewValueTransport{clientFakeTransport: base, nonComparable: []byte{1}, blockedRetries: &blocked}
	c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{
		Transport: same,
		Renewal: func(context.Context, sdkSession.SessionBundle, sdkAuth.PINProvider, sdkAuth.AuthOptions) (sdkSession.SessionBundle, error) {
			return fresh, nil
		},
		TransportFactory: func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
			return same, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, readErr := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := c.ExportSession()
	if err != nil {
		t.Fatal(err)
	}
	_, closesBefore, _ := base.counts()
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	_, closesAfter, _ := base.counts()
	var transportErr *sdkErrs.TransportError
	rejectedReuse := errors.As(readErr, &transportErr) && transportErr.Code == "reused_transport"
	profileChanged := !bytes.Equal(before, after)
	metadataChanged := exported.APIBase != b.APIBase
	t.Logf("non_comparable=%t reused_rejected=%t profile_changed=%t metadata_changed=%t blocked_retry=%d retired_closes=%d total_closes=%d", !reflect.TypeOf(same).Comparable(), rejectedReuse, profileChanged, metadataChanged, blocked.Load(), closesBefore, closesAfter)
	if !rejectedReuse || profileChanged || metadataChanged || closesBefore != 0 || closesAfter != 1 {
		t.Fatal("same non-comparable transport was committed and closed as both current and retired generation")
	}
}

func TestReviewProbeDefaultPINAuthCannotCloseCommittedTransport(t *testing.T) {
	for _, kind := range []string{"explicit-auth-transport", "auth-factory-reuse"} {
		t.Run(kind, func(t *testing.T) {
			b := clientFixture(t, "review-auth-owner")
			path := clientPrivatePath(t)
			if err := b.Save(path); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			old := clientFake(t, b)
			old.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(401, `{}`), nil
			}
			old.get = func(context.Context, string, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return clientResponse(403, `synthetic bootstrap rejection`), nil
			}
			authOpts := sdkAuth.AuthOptions{}
			if kind == "explicit-auth-transport" {
				authOpts.Transport = old
			} else {
				authOpts.TransportFactory = func(sdkSession.SessionBundle, sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
					return old, nil
				}
			}
			c, err := NewSberClientFromPINProfile(context.Background(), path, func(context.Context) (string, error) { return "13579", nil }, ClientOptions{Transport: old, AuthOptions: authOpts})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, readErr := c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			_, closesBefore, _ := old.counts()
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			calls, closesAfter, _ := old.counts()
			postCount, getCount := 0, 0
			for _, call := range old.snapshotCalls() {
				if call.method == "POST" {
					postCount++
				} else {
					getCount++
				}
			}
			t.Logf("error_returned=%t old_closed_before_commit=%d old_total_closes=%d calls=%d business_posts=%d bootstrap_gets=%d profile_changed=%t", readErr != nil, closesBefore, closesAfter, calls, postCount, getCount, !bytes.Equal(before, after))
			if postCount != 1 || getCount > 1 {
				t.Fatal("unexpected synthetic requests")
			}
			if closesBefore != 0 || closesAfter != 1 {
				t.Fatal("failed default PIN renewal closed the still-committed business owner through the auth options")
			}
		})
	}
}

func TestReviewProbeOutOfRangeExpiredHARCookieCannotBecomeSessionCookie(t *testing.T) {
	for _, expiry := range []string{"-1", "-9223372036854775809", "-1e30"} {
		t.Run(strings.ReplaceAll(expiry, "-", "neg"), func(t *testing.T) {
			raw := `{"log":{"entries":[{"request":{"method":"GET","url":"https://web2.online.sberbank.ru/main"},"response":{"cookies":[{"name":"SID","value":"review-expired-cookie-canary","domain":"online.sberbank.ru","path":"/","secure":true,"expires":` + expiry + `}] }},{"request":{"method":"POST","url":"https://web-node2.online.sberbank.ru/uoh-bh/v1/operations/list"}}]}}`
			path := clientPrivatePath(t)
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			builds := 0
			c, err := NewSberClientFromFiles(path, "", ClientOptions{TransportFactory: func(b sdkSession.SessionBundle, _ sdkTransport.TransportOptions) (sdkTransport.Transport, error) {
				builds++
				return clientFake(t, b), nil
			}})
			accepted := c != nil && err == nil
			nilExpiry := false
			if c != nil {
				defer c.Close()
				b, exportErr := c.ExportSession()
				if exportErr != nil {
					t.Fatal(exportErr)
				}
				nilExpiry = len(b.Cookies) == 1 && b.Cookies[0].Expires == nil
			}
			t.Logf("accepted=%t factory_builds=%d expired_cookie_has_nil_expiry=%t", accepted, builds, nilExpiry)
			if accepted || builds != 0 {
				t.Fatal("valid numeric expired HAR timestamp was silently converted to a live session cookie")
			}
		})
	}
}

type client4ForgedClassification struct{ text string }

func (e *client4ForgedClassification) Error() string { return e.text }
func (*client4ForgedClassification) Is(error) bool   { return true }
func (e *client4ForgedClassification) As(target any) bool {
	if p, ok := target.(**ClientCleanupError); ok {
		*p = &ClientCleanupError{cause: e}
		return true
	}
	if p, ok := target.(**sdkErrs.APIRejected); ok {
		*p = &sdkErrs.APIRejected{Message: e.text}
		return true
	}
	return false
}

type client4ErrorCycle struct{}

func (*client4ErrorCycle) Error() string   { return "synthetic cycle" }
func (e *client4ErrorCycle) Unwrap() error { return e }

func TestClientCycle4TypedNilErrorsAreSanitized(t *testing.T) {
	inputs := []error{(*sdkErrs.APIRejected)(nil), (*sdkErrs.APIError)(nil), (*sdkErrs.MissingSession)(nil), (*sdkErrs.InsecureSessionFile)(nil), (*sdkErrs.AuthenticationExpired)(nil), (*sdkErrs.MutationUncertain)(nil), (*sdkErrs.TransportError)(nil), (*sdkErrs.PinAuthError)(nil), (*sdkErrs.PinCaptchaRequired)(nil), (*sdkErrs.PinOTPRequired)(nil), (*ClientCleanupError)(nil), (*PaginationLimitError)(nil)}
	for i, input := range inputs {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Error("typed nil boundary panicked")
				}
			}()
			got := clientSafeError(input)
			if got == nil {
				t.Error("nonnil error swallowed")
			}
			var foreign *sdkErrs.APIRejected
			if errors.As(got, &foreign) {
				t.Error("nil rejection fabricated definite outcome")
			}
		})
	}
}
func TestClientCycle4ForeignAsIsAndCyclesDoNotDeclassify(t *testing.T) {
	input := &client4ForgedClassification{text: "synthetic private"}
	got := clientSafeError(input)
	var pending *ClientCleanupError
	var rejected *sdkErrs.APIRejected
	if errors.As(got, &pending) || errors.As(got, &rejected) || errors.Is(got, sdkErrs.ErrClosed) || errors.Is(got, context.Canceled) || errors.Is(got, context.DeadlineExceeded) || errors.Is(got, input) {
		t.Error("foreign As/Is forged source classification")
	}
	if got := clientSafeError(&client4ErrorCycle{}); got == nil {
		t.Error("cyclic error swallowed")
	}
}
func TestClientCycle4OwnedErrorCopyDetachesMutableMetadata(t *testing.T) {
	remaining, lifetime := 3, 90
	original := &sdkErrs.PinOTPRequired{PinAuthError: sdkErrs.PinAuthError{StatusCode: 403, RemainingAttempts: &remaining, ResetCookies: true}, Lifetime: &lifetime}
	got := clientSafeError(original)
	remaining, lifetime = 0, 0
	var otp *sdkErrs.PinOTPRequired
	if !errors.As(got, &otp) || otp == original || otp.RemainingAttempts == &remaining || otp.Lifetime == &lifetime || *otp.RemainingAttempts != 3 || *otp.Lifetime != 90 || !otp.ResetCookies {
		t.Error("owned metadata aliased input")
	}
	value := sdkErrs.APIRejected{Message: "synthetic private"}
	var rejected *sdkErrs.APIRejected
	if !errors.As(clientSafeError(value), &rejected) || rejected.Message != "" {
		t.Error("value classification not copied")
	}
}

func TestClientCycle4ErrorNodeBudgetIncludesTypedNilChildren(t *testing.T) {
	children := make([]error, 128)
	for i := range children {
		children[i] = (*sdkErrs.APIRejected)(nil)
	}
	got := clientSafeError(&client4ManyErrors{children})
	tree, ok := got.(interface{ Unwrap() []error })
	if !ok || len(tree.Unwrap()) > 64 {
		t.Error("typed nil children escaped the client error graph budget")
	}
}

type client4ManyErrors struct{ children []error }

func (*client4ManyErrors) Error() string     { return "synthetic broad error graph" }
func (e *client4ManyErrors) Unwrap() []error { return e.children }

func TestClientCycle4AfterSendOutcomesDistinguishDefiniteRejection(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
		uncertain  bool
	}{
		{"truncated", `{"success":true`, 200, true},
		{"duplicate", `{"success":true,"success":true}`, 200, true},
		{"trailing", `{"success":true} {}`, 200, true},
		{"surrogate", `{"success":true,"id":"\ud800"}`, 200, true},
		{"null-document", `null`, 200, true},
		{"missing-success", `{}`, 200, true},
		{"null-success", `{"success":null}`, 200, true},
		{"numeric-success", `{"success":1}`, 200, true},
		{"unknown-success", `{"success":"unknown"}`, 200, true},
		{"gateway", `{"success":false}`, 502, true},
		{"expired", `{}`, 401, true},
		{"redirect", `{}`, 307, true},
		{"boolean-rejection", `{"success":false}`, 200, false},
		{"string-rejection", `{"success":"false","error":{"code":"5"}}`, 200, false},
	}
	for _, tc := range cases {
		for _, route := range []string{"direct", "sequence"} {
			t.Run(tc.name+"/"+route, func(t *testing.T) {
				b := i3Bundle(t, "outcome")
				tr := i3NewTransport(t, b)
				response := i3Response(tc.status, tc.body)
				before := append([]byte(nil), response.Content...)
				tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
					return response, nil
				}
				c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
				var first, got error
				if route == "direct" {
					_, got = c.Mutate(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
				} else {
					got = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
						_, first = send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
						_, next := send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/test", true)
						if next != first {
							t.Error("recorded outcome replaced or financial POST replayed")
						}
						return errors.New("unrelated synthetic callback")
					})
					if got != first {
						t.Error("sequence replaced its safe recorded outcome")
					}
				}
				var uncertain *sdkErrs.MutationUncertain
				var rejected *sdkErrs.APIRejected
				if errors.As(got, &uncertain) != tc.uncertain {
					t.Error("after-send outcome classification wrong")
				}
				if !tc.uncertain && !errors.As(got, &rejected) {
					t.Error("validated definite rejection lost")
				}
				if tc.uncertain && errors.As(got, &rejected) {
					t.Error("unvalidated response fabricated definite rejection")
				}
				if n, _, _, _ := tr.counts(); n != 1 {
					t.Error("financial POST attempted more or less than once")
				}
				if !bytes.Equal(before, response.Content) {
					t.Error("original response bytes modified")
				}
			})
		}
	}
}
func TestClientCycle4PreSendValidationDoesNotInventUncertainty(t *testing.T) {
	for _, mode := range []string{"disabled", "bad-path", "bad-page", "missing-identity", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			b := i3Bundle(t, "presend")
			if mode == "missing-identity" {
				b.AntifraudDeviceprint = nil
			}
			tr := i3NewTransport(t, b)
			c := i3Client(t, b, tr, ClientOptions{AllowMutations: mode != "disabled"})
			path, page := Me2MeWorkflowPath, "/app/test"
			ctx := context.Background()
			if mode == "bad-path" {
				path = "/not-allowed"
			}
			if mode == "bad-page" {
				page = "invalid"
			}
			if mode == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, got := c.Mutate(ctx, path, nil, nil, page, true)
			var uncertain *sdkErrs.MutationUncertain
			if got == nil || errors.As(got, &uncertain) {
				t.Error("validation lost or pre-send failure mislabeled")
			}
			if n, _, _, _ := tr.counts(); n != 0 {
				t.Error("pre-send validation attempted POST")
			}
		})
	}
}

// An injected transport classification is not a validated API response. The
// client's after-send uncertainty dominates the nested rejection classification
// at the cached workflow guard, or a second snapshot could resend START.
func TestClientCycle4AfterSendClassifiedRejectionCannotResetStartGuard(t *testing.T) {
	b := i3Bundle(t, "rejection-guard")
	tr := i3NewTransport(t, b)
	financial := 0
	tr.post = func(_ context.Context, target string, _ map[string]any, _ sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		u, e := url.Parse(target)
		if e != nil {
			t.Fatal(e)
		}
		if u.Path == ProductsPath {
			return i3Response(200, i3PortfolioJSON), nil
		}
		financial++
		return nil, &sdkErrs.APIRejected{Message: "synthetic private transport diagnostic"}
	}
	c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
	p, e := c.Portfolio(context.Background(), false)
	if e != nil {
		t.Fatal(e)
	}
	_, e = p.Transfers().Start(context.Background())
	var uncertain *sdkErrs.MutationUncertain
	if !errors.As(e, &uncertain) {
		t.Fatal("unknown after-send transport failure lost uncertainty")
	}
	later, e := c.Portfolio(context.Background(), true)
	if e != nil {
		t.Fatal(e)
	}
	if later.Transfers() != p.Transfers() {
		t.Fatal("snapshot replaced issuer")
	}
	_, e = later.Transfers().Start(context.Background())
	if e == nil || financial != 1 {
		t.Error("nested transport rejection bypassed uncertain START guard")
	}
}

// Actual client-decoded rejection metadata, not a standalone foundation test.
// Explicit errors.As fields remain allowed; ordinary diagnostics must not read them.
func TestFreshClient4DecodedRejectionDiagnosticPrivacy(t *testing.T) {
	marker := strings.Join([]string{"fresh", "response", "metadata", "canary"}, "-")
	body, err := json.Marshal(map[string]any{"success": false, "error": map[string]any{"code": marker, "text": marker, "title": marker, "uuid": marker, "system": marker}})
	if err != nil {
		t.Fatal("synthetic response construction failed")
	}
	for _, route := range []string{"read", "raw-mutation", "bound-start", "sequence"} {
		t.Run(route, func(t *testing.T) {
			b := i3Bundle(t, "response-private")
			tr := i3NewTransport(t, b)
			tr.post = func(context.Context, string, map[string]any, sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
				return i3Response(200, string(body)), nil
			}
			c := i3Client(t, b, tr, ClientOptions{AllowMutations: true})
			var got error
			switch route {
			case "read":
				_, got = c.PostRead(context.Background(), "/uoh-bh/v1/operations/list", nil)
			case "raw-mutation":
				_, got = c.Mutate(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/fresh", true)
			case "sequence":
				got = c.MutationSequence(context.Background(), func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
					_, err := send(context.Background(), Me2MeWorkflowPath, nil, nil, "/app/fresh", true)
					return err
				})
			default:
				_, got = c.Transfers().Start(context.Background())
			}
			var rejected *sdkErrs.APIRejected
			if !errors.As(got, &rejected) || rejected.Code != marker || rejected.Text != marker {
				t.Fatal("explicit bounded rejection metadata control failed")
			}
			bad := "%w"
			control := struct{ Code string }{marker}
			if !strings.Contains(fmt.Sprintf(bad, control), marker) {
				t.Fatal("raw diagnostic negative control ineffective")
			}
			for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%p", "%w", "%#w", "%w %w"} {
				var buf bytes.Buffer
				log.New(&buf, "", 0).Printf(format, got)
				for _, text := range []string{fmt.Sprintf(format, got), fmt.Errorf(format, got).Error(), buf.String()} {
					if strings.Contains(text, marker) {
						t.Errorf("ordinary returned-outcome diagnostic exposes remote private metadata (format %s)", format)
					}
				}
			}
			data, e := json.Marshal(got)
			if e != nil || strings.Contains(string(data), marker) {
				t.Fatal("ordinary JSON exposes remote metadata")
			}
			if n, _, _, _ := tr.counts(); n != 1 {
				t.Fatal("diagnostic witness repeated request")
			}
		})
	}
}

type client4PrivateCause struct{ text string }

func (e *client4PrivateCause) Error() string { return e.text }

type client4WrappedError struct {
	inner error
	text  string
}

func (e *client4WrappedError) Error() string { return e.text }
func (e *client4WrappedError) Unwrap() error { return e.inner }

func client4AssertPrivateErrorAbsent(t *testing.T, got error, marker string, raw error) {
	t.Helper()
	if got == nil {
		t.Fatal("failure lost")
	}
	var foreign *client4PrivateCause
	var wrapper *client4WrappedError
	if errors.Is(got, raw) || errors.As(got, &foreign) || errors.As(got, &wrapper) {
		t.Error("foreign error object retained")
	}
	badVerb := "%w"
	texts := []string{got.Error(), fmt.Errorf("%w", got).Error(), fmt.Sprintf(badVerb, got), fmt.Sprintf("%#v", got)}
	b, err := json.Marshal(got)
	if err == nil {
		texts = append(texts, string(b))
	}
	for _, text := range texts {
		if strings.Contains(text, marker) {
			t.Error("private error text escaped copied boundary")
		}
	}
}

func TestClientCycle4ConcreteErrorsAreOwnedSafeCopies(t *testing.T) {
	marker := strings.Join([]string{"cycle4", "synthetic", "private", "error"}, "-")
	rawCause := &client4PrivateCause{marker}
	cases := []struct {
		name           string
		input          error
		classification func(error) bool
	}{
		{"missing", &sdkErrs.MissingSession{Message: marker}, func(e error) bool { var x *sdkErrs.MissingSession; return errors.As(e, &x) && x.Message == "" }},
		{"insecure", &sdkErrs.InsecureSessionFile{Message: marker}, func(e error) bool { var x *sdkErrs.InsecureSessionFile; return errors.As(e, &x) && x.Message == "" }},
		{"expired", &sdkErrs.AuthenticationExpired{Message: marker}, func(e error) bool { var x *sdkErrs.AuthenticationExpired; return errors.As(e, &x) && x.Message == "" }},
		{"api", &sdkErrs.APIError{Message: marker, StatusCode: 502}, func(e error) bool {
			var x *sdkErrs.APIError
			return errors.As(e, &x) && x.Message == "" && x.StatusCode == 502
		}},
		{"rejected", &sdkErrs.APIRejected{Message: marker, Code: marker, Text: marker, Title: marker, UUID: marker, System: marker}, func(e error) bool {
			var x *sdkErrs.APIRejected
			return errors.As(e, &x) && *x == (sdkErrs.APIRejected{})
		}},
		{"uncertain", &sdkErrs.MutationUncertain{Message: marker}, func(e error) bool { var x *sdkErrs.MutationUncertain; return errors.As(e, &x) && x.Message == "" }},
		{"transport", sdkErrs.NewTransportError(marker, rawCause), func(e error) bool {
			var x *sdkErrs.TransportError
			return errors.As(e, &x) && x.Code == "request_failed" && x.ContextCause() == nil
		}},
		{"proxy TLS closure", sdkErrs.NewTransportError("proxy_tls_closed", rawCause), func(e error) bool {
			var x *sdkErrs.TransportError
			return errors.As(e, &x) && x.Code == "proxy_tls_closed" && x.ContextCause() == nil
		}},
		{"rejected login page", &sdkErrs.PinAuthError{Code: "login_page_rejected", Message: marker, StatusCode: 200}, func(e error) bool {
			var x *sdkErrs.PinAuthError
			return errors.As(e, &x) && x.Code == "login_page_rejected" && x.Message == "" && x.StatusCode == 200
		}},
		{"pin", &sdkErrs.PinAuthError{Message: marker, Code: marker, StatusCode: 403, ResetCookies: true}, func(e error) bool {
			var x *sdkErrs.PinAuthError
			return errors.As(e, &x) && x.Message == "" && x.Code == "" && x.StatusCode == 403 && x.ResetCookies
		}},
		{"captcha", &sdkErrs.PinCaptchaRequired{PinAuthError: sdkErrs.PinAuthError{Message: marker, Code: marker}, ImageURL: &marker, AudioURL: &marker}, func(e error) bool {
			var x *sdkErrs.PinCaptchaRequired
			return errors.As(e, &x) && x.Message == "" && x.Code == "" && x.ImageURL == nil && x.AudioURL == nil
		}},
		{"otp", &sdkErrs.PinOTPRequired{PinAuthError: sdkErrs.PinAuthError{Message: marker, Code: marker}}, func(e error) bool {
			var x *sdkErrs.PinOTPRequired
			return errors.As(e, &x) && x.Message == "" && x.Code == ""
		}},
		{"parse", NewParseError(marker), func(e error) bool { var x *ParseError; return errors.As(e, &x) && x.Field() == "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := &client4WrappedError{inner: tc.input, text: marker}
			got := clientSafeError(input)
			client4AssertPrivateErrorAbsent(t, got, marker, input)
			if got == tc.input || !tc.classification(got) {
				t.Error("recognized classification was not copied into safe owned fields")
			}
			tc.input = nil
			client4AssertPrivateErrorAbsent(t, clientSafeError(got), marker, input)
		})
	}
}
func TestClientCycle4SafeCopyPreservesCompositeContextAndCleanup(t *testing.T) {
	marker := strings.Join([]string{"cycle4", "synthetic", "private", "cause"}, "-")
	foreign := &client4PrivateCause{marker}
	calls := 0
	cleanup := &ClientCleanupError{cause: errors.Join(&sdkErrs.MissingSession{Message: marker}, context.Canceled, foreign), cleanup: func() error { calls++; return nil }}
	input := &client4WrappedError{inner: errors.Join(cleanup, &sdkErrs.MutationUncertain{Message: marker}, sdkErrs.NewTransportError("deadline_exceeded", fmt.Errorf("%s: %w", marker, context.DeadlineExceeded)), foreign), text: marker}
	got := clientSafeError(input)
	client4AssertPrivateErrorAbsent(t, got, marker, input)
	var pending *ClientCleanupError
	var uncertain *sdkErrs.MutationUncertain
	var missing *sdkErrs.MissingSession
	if !errors.As(got, &pending) || !errors.As(got, &uncertain) || !errors.As(got, &missing) || !errors.Is(got, context.Canceled) || !errors.Is(got, context.DeadlineExceeded) {
		t.Fatal("composite source classifications lost")
	}
	if pending == cleanup || pending.Close() != nil || calls != 1 {
		t.Fatal("cleanup ownership not carried into owned copy")
	}
	if missing.Message != "" || uncertain.Message != "" {
		t.Error("copied messages retain foreign text")
	}
}
