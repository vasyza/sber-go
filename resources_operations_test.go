package sber

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func resourceOperationsResponse(ids ...string) map[string]any {
	ops := make([]any, 0, len(ids))
	for _, id := range ids {
		ops = append(ops, map[string]any{"uohId": id, "date": "01.07.2026T10:00:00", "operationAmount": map[string]any{"amount": "1.00", "currencyCode": "RUB"}})
	}
	return map[string]any{"success": true, "body": map[string]any{"operations": ops}}
}
func resourceHistoryCall(offset, size int, resource, from, to string) resourceCall {
	b := map[string]any{"paginationOffset": offset, "paginationSize": size, "showHidden": false, "showNotTransactionBonuses": true, "showOpenBanking": true}
	if resource != "" {
		b["usedResource"] = []string{resource}
	}
	if from != "" {
		b["from"] = from
	}
	if to != "" {
		b["to"] = to
	}
	return resourceCall{Kind: "read", Path: "/uoh-bh/v1/operations/list", Payload: b}
}
func TestResourceOperationsPageNPlusOneAndInclusiveBounds(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(7, 3, "card:4004", "01.07.2026T00:00:00", "31.07.2026T23:59:59"), Response: resourceOperationsResponse("1", "2", "sentinel")}}}
	api := NewOperationsAPI(r, ResourceOptions{Now: func() time.Time { return time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC) }})
	page, err := api.Page(context.Background(), OperationsPageOptions{Resource: "card:4004", Offset: 7, Limit: 2, From: "2026-07-01", To: "2026-07-31"})
	if err != nil || len(page.Operations) != 2 || page.NextOffset == nil || *page.NextOffset != 9 || page.Operations[0].ScopeCardIDs[0] != "4004" {
		t.Fatalf("N+1 page: %#v %v", page, err)
	}
	r.done()
}
func TestResourceOperationsIterDeduplicatesAndFreezesDefaultWindow(t *testing.T) {
	calls := 0
	clock := func() time.Time { calls++; return time.Date(2026, 10, 7, 10, 0, 0, 0, domainMoscow) }
	r := &resourceScript{t: t, steps: []resourceStep{
		{Call: resourceHistoryCall(0, 3, "", "09.06.2026T10:00:00", ""), Response: resourceOperationsResponse("1", "2", "sentinel")},
		{Call: resourceHistoryCall(2, 3, "", "09.06.2026T10:00:00", ""), Response: resourceOperationsResponse("2", "3")},
	}}
	var ids []string
	for op, err := range NewOperationsAPI(r, ResourceOptions{Now: clock}).Iter(context.Background(), OperationsQuery{Limit: 2, MaxPages: 3}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, op.ID)
	}
	if !reflect.DeepEqual(ids, []string{"1", "2", "3"}) || calls != 1 {
		t.Fatalf("dedup/default clock: %v calls=%d", ids, calls)
	}
	r.done()
}
func TestResourceOperationsListSortsDescendingStably(t *testing.T) {
	response := resourceOperationsResponse("old", "bad", "new", "same")
	ops := response["body"].(map[string]any)["operations"].([]any)
	for i, date := range []string{"2026-07-01T10:00:00+03:00", "bad date", "2026-07-02T10:00:00+03:00", "2026-07-02T10:00:00+03:00"} {
		ops[i].(map[string]any)["date"] = date
	}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 31, "", "01.07.2026T00:00:00", ""), Response: response}}}
	got, err := NewOperationsAPI(r).List(context.Background(), OperationsQuery{Limit: 30, MaxPages: 100, From: "2026-07-01"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, op := range got {
		ids = append(ids, op.ID)
	}
	if !reflect.DeepEqual(ids, []string{"new", "same", "old", "bad"}) {
		t.Fatal(ids)
	}
	r.done()
}
func TestResourceOperationsListDoesNotReturnPartialAsSuccess(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1", "sentinel")}}}
	got, err := NewOperationsAPI(r).List(context.Background(), OperationsQuery{Limit: 1, MaxPages: 1, From: "2026-07-01"})
	if err == nil || got != nil {
		t.Fatalf("partial list disguised: %v %v", got, err)
	}
	r.done()
}

func TestResourceOperationsIterCapYieldsErrorAfterPartial(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1", "sentinel")}}}
	var ids []string
	var gotErr error
	for op, err := range NewOperationsAPI(r).Iter(context.Background(), OperationsQuery{Limit: 1, MaxPages: 1, From: "2026-07-01"}) {
		if err != nil {
			gotErr = err
			break
		}
		ids = append(ids, op.ID)
	}
	var capErr *PaginationLimitError
	if !errors.As(gotErr, &capErr) || capErr.MaxPages != 1 || !reflect.DeepEqual(ids, []string{"1"}) {
		t.Fatalf("partial/error: %v %v", ids, gotErr)
	}
	r.done()
}
