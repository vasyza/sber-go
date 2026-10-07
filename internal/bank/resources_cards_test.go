package bank

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func resourceCardInfoResponse() map[string]any {
	return resourceMap(`{"success":true,"body":{"cardDetails":{"cards":[{"id":12345,"name":"Fixture card","number":"4111 1111 1111 1111","state":"ACTIVE","cardHolder":"FIXTURE HOLDER","paySystemType":"VISA","expireDate":"12/28","limits":{"purchaseLimit":{"amount":"300000.00","currency":{"code":"RUB"}},"availableLimit":{"amount":"84250.17","currency":{"code":"RUB"}},"availableTotalLimit":{"amount":"84250.17","currency":{"code":"RUB"}}},"creditType":{"creditLimit":{"amount":"120000.00","currency":{"code":"RUB"}},"creditDebt":{"amount":"15000.00","currency":{"code":"RUB"}},"creditMinPaymentDate":"2026-09-01"}}]}}}`)
}
func TestResourceCardsInfoCanonicalNumericIDsAndTypedFields(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/ufs-carddetail/rest/card/v1/cardInfo", Payload: map[string]any{"cardIds": []int64{12345, 12345, 9007199254740991}}}, Response: resourceCardInfoResponse()}}}
	got, err := NewCardsAPI(r).Info(context.Background(), 12345, "٠٠١٢٣٤٥", int64(9007199254740991))
	if err != nil || len(got) != 1 || got[0].Last4 != "1111" || got[0].Limits.Available.Amount.String() != "84250.17" || got[0].Credit.Debt.Amount.String() != "15000.00" {
		t.Fatalf("card info: %#v %v", got, err)
	}
	r.done()
}
func TestResourceCardsLimitsReturnsFirstOrUnknown(t *testing.T) {
	for _, empty := range []bool{false, true} {
		response := resourceCardInfoResponse()
		if empty {
			response = resourceMap(`{"success":true,"body":{"cardDetails":{"cards":[]}}}`)
		}
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/ufs-carddetail/rest/card/v1/cardInfo", Payload: map[string]any{"cardIds": []int64{12345}}}, Response: response}}}
		got, err := NewCardsAPI(r).Limits(context.Background(), 12345)
		if err != nil || empty != (got == nil) {
			t.Fatalf("limits: %#v %v", got, err)
		}
		if !empty && (got.AvailableTotal.Amount.String() != "84250.17" || got.Purchase.Amount.String() != "300000.00") {
			t.Fatal("limits fields lost")
		}
		r.done()
	}
}
func TestResourceCardsInfoRejectsMalformedIDBeforeCanonicalizing(t *testing.T) {
	r := &resourceScript{t: t}
	api := NewCardsAPI(r)
	invalid := []any{nil, true, false, 1.0, -1, 0, "", " 1", "1 ", "+1", "1.0", "１\n", "abc", strings.Repeat("0", 17) + "1", int64(9007199254740992), uint64(^uint64(0)), "²"}
	if _, err := api.Info(context.Background()); err == nil {
		t.Fatal("empty ID list")
	}
	for _, id := range invalid {
		if _, err := api.Info(context.Background(), id); err == nil {
			t.Fatalf("accepted ID %#v", id)
		}
	}
	if !reflect.DeepEqual(r.calls, []resourceCall(nil)) {
		t.Fatal("invalid ID sent")
	}
}
