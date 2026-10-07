package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
)

func TestAdditionalReadCommandsRouteAndPreserveExactMoney(t *testing.T) {
	for _, tt := range []struct {
		args     []string
		path     string
		response string
		body     map[string]any
	}{
		{[]string{"card-info", "--card-id", "00123", "--card-id", "456"}, sber.CardInfoPath, `{"success":true,"body":{"cardDetails":{"cards":[{"id":"123","number":"4111111111111111","limits":{"availableLimit":{"amount":"9007199254740993.10","currency":"RUB"}}}]}}}`, map[string]any{"cardIds": []int64{123, 456}}},
		{[]string{"card-limits", "--card-id", "123"}, sber.CardInfoPath, `{"success":true,"body":{"cardDetails":{"cards":[{"id":"123","limits":{"availableLimit":{"amount":"9007199254740993.10","currency":"RUB"}}}]}}}`, map[string]any{"cardIds": []int64{123}}},
		{[]string{"operation-details", "--operation-id", "fixture-operation"}, sber.OperationDetailsPath, `{"success":true,"body":{"uohId":"fixture-operation","header":{"operationAmount":{"amount":"9007199254740993.10","currency":"RUB"}}}}`, map[string]any{"uohId": "fixture-operation"}},
		{[]string{"analytics", "--from", "2026-08-01", "--to", "2026-08-31", "--income-type", "income", "--between-own=false", "--open-banking"}, sber.PFMAmountsPath, `{"success":true,"body":{"amounts":[{"nationalAmount":{"amount":"9007199254740993.10","currency":"RUB"}}]}}`, nil},
		{[]string{"portfolio"}, sber.ProductsPath, "", nil},
	} {
		t.Run(tt.args[0], func(t *testing.T) {
			client := &testutil.Client{}
			if tt.response != "" {
				client.Read = func(_ context.Context, path string, body map[string]any) (map[string]any, error) {
					if path != tt.path || tt.body != nil && !reflect.DeepEqual(body, tt.body) {
						t.Fatal("CLI changed the endpoint or request contract")
					}
					if tt.args[0] == "analytics" {
						filter := body["filter"].(map[string]any)
						if filter["incomeType"] != "income" || filter["betweenOwnFilter"] != "off" || filter["openBankingFilter"] != "on" || filter["from"] != "2026-08-01T00:00:00+03:00" || filter["to"] != "2026-08-31T23:59:59+03:00" {
							t.Fatal("analytics filters changed")
						}
					}
					return sber.DecodeJSON(strings.NewReader(tt.response))
				}
			}
			var output, diagnostics bytes.Buffer
			args := append(append([]string{}, tt.args...), "--profile", "synthetic-selected")
			code := RunWithOptions(context.Background(), args, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
			if code != 0 || client.Closes.Load() != 1 || len(client.Requests()) != 1 || client.Requests()[0].Path != tt.path {
				t.Fatalf("read command failed: status=%d", code)
			}
			if !strings.Contains(output.String(), "9007199254740993.10") || strings.Contains(output.String(), "4111111111111111") || !json.Valid(output.Bytes()) {
				t.Fatal("read command lost exact data or exposed PAN")
			}
		})
	}
}

func TestReadCommandArgumentsFailBeforeOpeningProfile(t *testing.T) {
	for _, args := range [][]string{
		{"card-info"}, {"card-info", "--card-id", "-1"}, {"card-info", "--card-id", "9007199254740992"},
		{"card-limits", "--card-id", "123", "--card-id", "456"}, {"operation-details", "--operation-id", "bad/id"},
		{"analytics", "--from", "2026-08-01"}, {"analytics", "--from", "2026-08-01", "--to", "2026-08-31", "--income-type", "both"},
		{"products", "--offset", "10"}, {"accounts", "--from", "2026-08-01"}, {"operations-page", "--max-pages", "3"},
		{"operations", "--resource", "unsupported:123"}, {"operations-page", "--resource", "card:bad/id"},
		{"operations-page", "--offset", strconv.Itoa(int(^uint(0) >> 1))},
	} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), append(args, "--profile", "synthetic-selected"), &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { t.Fatal("invalid command opened the profile"); return nil, nil }})
		if code != 2 || output.Len() != 0 {
			t.Fatalf("invalid arguments accepted for %s", args[0])
		}
	}
}

func TestSessionExportUsesPrivateNoReplacePublication(t *testing.T) {
	destination := filepath.Join(testPrivateDir(t), "export.json")
	for _, want := range []int{0, 3} {
		client := &testutil.Client{}
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), []string{"export-session", "--profile", "synthetic-selected", "--destination", destination}, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }})
		if code != want || client.Closes.Load() != 1 || len(client.Requests()) != 0 || strings.Contains(output.String(), "synthetic-private-cookie") {
			t.Fatalf("private export boundary failed: status=%d", code)
		}
		b, err := sber.LoadSessionBundle(destination)
		if err != nil || len(b.Cookies) != 1 || b.Cookies[0].Value != "synthetic-private-cookie" {
			t.Fatal("private export cannot be reopened")
		}
		info, err := os.Stat(destination)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("export permissions are not private")
		}
	}
}
