package sber

import (
	"context"
	"testing"
)

func TestClientBindingAnalyticsFunctionalAccess(t *testing.T) {
	c, tr := clientBindingSyntheticClient(t, resourceMap(`{"success":true,"body":{"amounts":[{"nationalAmount":{"amount":"50000.00","currency":"RUB"}}]}}`))
	api := clientBindingGetter[*AnalyticsAPI](t, c, "Analytics")
	if api != clientBindingGetter[*AnalyticsAPI](t, c, "Analytics") || !clientBindingOwnsRequester(c, api.requester) {
		t.Fatal("getter changed native API/requester identity")
	}
	amounts, err := api.Amounts(context.Background(), "2026-08-01", "2026-08-31")
	if err != nil || len(amounts.Periods) != 1 || amounts.Periods[0].NationalAmount.Amount.String() != "50000.00" {
		t.Fatalf("native analytics: %v", err)
	}
	calls := tr.snapshotCalls()
	if len(calls) != 1 || calls[0].target != c.core().bundle.APIBase+PFMAmountsPath {
		t.Fatal("analytics requester changed")
	}
	filter := calls[0].body["filter"].(map[string]any)
	if filter["from"] != "2026-08-01T00:00:00+03:00" || filter["to"] != "2026-08-31T23:59:59+03:00" || filter["betweenOwnFilter"] != "on" {
		t.Fatal("analytics source filter changed")
	}
}
