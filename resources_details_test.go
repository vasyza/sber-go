package sber

import (
	"context"
	"reflect"
	"testing"
)

func TestResourceOperationsDetailsExactIDAndTypedResult(t *testing.T) {
	response := resourceMap(`{"success":true,"body":{"uohId":"op-uuid-1","header":{"title":"Оплата","operationAmount":{"amount":"-1234.56","currencyCode":"RUB"}},"state":{"category":"COMPLETED"},"statementAvailable":true,"fields":[{"name":"Сумма","type":"AMOUNT","value":{"amount":"-1234.56","currencyCode":"RUB"}}]}}`)
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceCall{Kind: "read", Path: "/uoh-bh/v1/operation/details", Payload: map[string]any{"uohId": "op-uuid-1"}}, Response: response}}}
	got, err := NewOperationsAPI(r).Details(context.Background(), "op-uuid-1")
	if err != nil || got.UOHID != "op-uuid-1" || got.Amount.Amount.String() != "-1234.56" || !got.StatementAvailable {
		t.Fatalf("detail: %#v %v", got, err)
	}
	if _, ok := got.Fields[0].Value.(*Money); !ok {
		t.Fatal("lost money type")
	}
	r.done()
}
func TestResourceOperationsDetailsRejectsLiteralMalformedIDs(t *testing.T) {
	r := &resourceScript{t: t}
	api := NewOperationsAPI(r)
	for _, id := range []string{"", "has space", "trim ", "\nvalid", "a/b", "é", "a" + string(make([]byte, 128))} {
		if _, err := api.Details(context.Background(), id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	if !reflect.DeepEqual(r.calls, []resourceCall(nil)) {
		t.Fatal("validation sent request")
	}
}
