package bank

import (
	"context"
	"testing"
)

func TestResourceAnalyticsAmountsExactDefaultFilter(t *testing.T) {
	response := resourceMap(`{"success":true,"body":{"amounts":[{"from":"2026-08-01T00:00:00+03:00","to":"2026-08-31T23:59:59+03:00","incomeType":"outcome","nationalAmount":{"amount":"50000.00","currency":"RUB"},"categoryAmounts":[{"id":1,"name":"Fixture category","externalId":"CAFE","visibleAmount":{"amount":"12000.00","currency":"RUB"},"countOperations":8}]}]}}`)
	expected := map[string]any{"filter": map[string]any{"from": "2026-08-01T00:00:00+03:00", "to": "2026-08-31T23:59:59+03:00", "incomeType": "outcome", "betweenOwnFilter": "on", "openBankingFilter": "off", "productFilters": []any{map[string]any{"type": "CARD", "filter": "custom"}, map[string]any{"type": "CT_ACCOUNT", "filter": "custom"}, map[string]any{"type": "MANUAL", "filter": "custom"}}}, "display": map[string]any{"showCategoryAmounts": true, "showProductAmounts": true}}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/pfpv_alf_mb/v1.00/alf/amounts", Payload: expected}, Response: response}}}
	got, err := NewAnalyticsAPI(r).Amounts(context.Background(), "2026-08-01", "2026-08-31")
	if err != nil || got.Periods[0].NationalAmount.Amount.String() != "50000.00" || got.Periods[0].Categories[0].CountOperations != 8 {
		t.Fatalf("analytics: %#v %v", got, err)
	}
	r.done()
}
func TestResourceAnalyticsFlagsAndAwareMoscowBounds(t *testing.T) {
	expected := map[string]any{"filter": map[string]any{"from": "2026-08-01T03:00:00+03:00", "to": "2026-08-02T04:00:00+03:00", "incomeType": "income", "betweenOwnFilter": "off", "openBankingFilter": "on", "productFilters": []any{map[string]any{"type": "CARD", "filter": "custom"}, map[string]any{"type": "CT_ACCOUNT", "filter": "custom"}, map[string]any{"type": "MANUAL", "filter": "custom"}}}, "display": map[string]any{"showCategoryAmounts": false, "showProductAmounts": false}}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/pfpv_alf_mb/v1.00/alf/amounts", Payload: expected}, Response: map[string]any{"success": true, "body": map[string]any{"amounts": []any{}}}}}}
	_, err := NewAnalyticsAPI(r).Amounts(context.Background(), "2026-08-01T00:00:00Z", "2026-08-02T01:00:00Z", AnalyticsOptions{IncomeType: "income", OpenBanking: true})
	if err != nil {
		t.Fatal(err)
	}
	r.done()
}
func TestResourceAnalyticsRejectsInvalidBoundsAndIncomeType(t *testing.T) {
	r := &resourceScript{t: t}
	a := NewAnalyticsAPI(r)
	for _, b := range [][2]string{{"", "2026-08-01"}, {"2026-08-01", ""}, {"2026-08-31", "2026-08-01"}, {"bad", "2026-08-01"}, {"0000-01-01", "2026-08-01"}} {
		if _, err := a.Amounts(context.Background(), b[0], b[1]); err == nil {
			t.Fatal("bad bounds accepted")
		}
	}
	if _, err := a.Amounts(context.Background(), "2026-08-01", "2026-08-31", AnalyticsOptions{IncomeType: "both"}); err == nil {
		t.Fatal("bad income type")
	}
	if len(r.calls) != 0 {
		t.Fatal("bad analytics requested")
	}
}
