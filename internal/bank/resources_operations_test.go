package bank

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

func TestResourceCollectionExplicitEmptyWindowIsNotBankCompletenessProof(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 31, "card:4004", "01.07.2026T00:00:00", "31.07.2026T23:59:59"), Response: resourceOperationsResponse()}}}
	got, err := NewOperationsAPI(r).Collect(context.Background(), OperationsQuery{Resource: "card:4004", Limit: 30, MaxPages: 100, From: "2026-07-01", To: "2026-07-31"})
	m := got.Metadata
	if err != nil || len(got.Operations) != 0 || !m.PaginationExhausted || m.BankCapProven || m.WindowCompleteness != "unknown" || !m.ExplicitFrom || !m.ExplicitTo || m.DefaultWindow || m.PagesRead != 1 || m.RequestedFrom != "01.07.2026T00:00:00" || m.RequestedTo != "31.07.2026T23:59:59" {
		t.Fatalf("false completeness: %#v %v", m, err)
	}
	r.done()
}
func TestResourceCollectionCapKeepsExplicitPartialAndUnknownCoverage(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1", "sentinel")}}}
	got, err := NewOperationsAPI(r).Collect(context.Background(), OperationsQuery{Limit: 1, MaxPages: 1, From: "2026-07-01"})
	var capErr *PaginationLimitError
	if !errors.As(err, &capErr) || len(got.Operations) != 1 || got.Metadata.PaginationExhausted || !got.Metadata.ClientCapReached || got.Metadata.WindowCompleteness != "unknown" {
		t.Fatal("cap reported complete")
	}
	r.done()
}

func TestResourceCanceledHistoryDoesNotReachRequester(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &resourceScript{t: t}
	api := NewOperationsAPI(r)
	if _, err := api.Page(ctx, OperationsPageOptions{Limit: 30, From: "2026-07-01"}); !errors.Is(err, context.Canceled) || len(r.calls) != 0 {
		t.Fatal("canceled page sent")
	}
}
func TestResourceConfirmCancellationBetweenStagesDoesNotSendNext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	steps := []resourceStep{{Call: resourceConfirmCall(0), Response: resourceConfirmResponses()[0], Before: func(context.Context) { cancel() }}}
	a, r, p := resourceReadyTransfer(t, steps...)
	_, err := a.Confirm(ctx, p)
	var uncertain *sdkErrs.MutationUncertain
	if !errors.As(err, &uncertain) || len(r.calls) != 3 {
		t.Fatal("canceled confirmation sent next stage")
	}
	if _, err = a.Confirm(context.Background(), p); err == nil || len(r.calls) != 3 {
		t.Fatal("canceled confirmation repeated")
	}
	r.done()
}
func TestResourceTransferStartRejectsRepeatedServerPID(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: resourceStartResponse()}, {Call: resourceStartCall(), Response: resourceStartResponse()}}}
	a := NewTransfersAPI(r, ResourceOptions{AllowMutations: true})
	if _, err := a.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Start(context.Background()); err == nil {
		t.Fatal("server reused workflow PID")
	}
	if _, err := a.Start(context.Background()); err == nil || len(r.calls) != 2 {
		t.Fatal("uncertain start repeated")
	}
	r.done()
}
func TestResourceHistoryValidationAndUnavailability(t *testing.T) {
	r := &resourceScript{t: t}
	api := NewOperationsAPI(r)
	cases := []OperationsPageOptions{{Limit: 0}, {Limit: -1}, {Limit: 101}, {Limit: 30, Offset: -1}, {Limit: 30, Offset: int(^uint(0) >> 1)}, {Limit: 30, Resource: "transactionAccount:1"}, {Limit: 30, Resource: " card:1"}, {Limit: 30, Resource: "card:1\n"}, {Limit: 30, Resource: "card:" + strings.Repeat("a", 129)}, {Limit: 30, From: "2026-08-31", To: "2026-08-01"}, {Limit: 30, From: "bad"}}
	for i, q := range cases {
		if _, err := api.Page(context.Background(), q); err == nil {
			t.Fatalf("bad query %d", i)
		}
	}
	for _, q := range []OperationsQuery{{Limit: 30, MaxPages: 0}, {Limit: 30, MaxPages: -1}} {
		for _, err := range api.Iter(context.Background(), q) {
			if err == nil {
				t.Fatal("invalid max_pages")
			}
		}
	}
	if len(r.calls) != 0 {
		t.Fatal("validation reached requester")
	}
	payloads := []map[string]any{resourceMap(`{"success":false,"error":{"code":"fixture"}}`), resourceMap(`{"success":true,"body":{}}`), resourceMap(`{"success":true,"body":{"operations":null}}`), resourceMap(`{"success":true,"body":{"operations":[{"date":"01.07.2026T10:00:00"}]}}`), resourceMap(`{"success":true,"body":{"operations":[{"uohId":"valid"},{}]}}`), resourceMap(`{"success":true,"body":{"sourceErrorResponse":{},"operations":[]}}`)}
	for i, payload := range payloads {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "", "01.07.2026T00:00:00", ""), Response: payload}}}
			got, err := NewOperationsAPI(r).Collect(context.Background(), OperationsQuery{Limit: 1, MaxPages: 1, From: "2026-07-01"})
			if err == nil || got.Metadata.PaginationExhausted || got.Metadata.WindowCompleteness != "unknown" {
				t.Fatal("unavailable history reported empty/complete")
			}
			r.done()
		})
	}
}
func TestResourceHistoryOnlyUpperBoundHasNoImplicitFrom(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 101, "", "", "01.07.2026T23:59:59"), Response: resourceOperationsResponse()}}}
	got, err := NewOperationsAPI(r).Collect(context.Background(), OperationsQuery{Limit: 100, MaxPages: 100, To: "2026-07-01"})
	if err != nil || got.Metadata.DefaultWindow || got.Metadata.RequestedFrom != "" {
		t.Fatal("hidden lower/default bound")
	}
	r.done()
}

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

// Each expected wire string is checked against the frozen, actually executed
// Python3.12 source corpus before this tracer runs. No runtime oracle is used.
func TestResourceCycle3PageUsesCanonicalSourceRequestYear(t *testing.T) {
	input := "0001-01-01T00:00:00Z"
	want := "01.01.1T02:30:17"
	requester := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "card:fixture", want, want), Response: resourceOperationsResponse()}}}
	result, err := NewOperationsAPI(requester).Page(context.Background(), OperationsPageOptions{Resource: "card:fixture", Limit: 1, From: input, To: input})
	if err != nil || len(result.Operations) != 0 || result.NextOffset != nil {
		t.Fatalf("canonical source page failed: %v", err)
	}
	requester.done()
}

func TestResourceCycle3CollectionMetadataUsesSameSourceRequestYear(t *testing.T) {
	input := "0001-01-01T00:00:00Z"
	want := "01.01.1T02:30:17"
	requester := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "card:fixture", want, want), Response: resourceOperationsResponse()}}}
	result, err := NewOperationsAPI(requester).Collect(context.Background(), OperationsQuery{Resource: "card:fixture", Limit: 1, MaxPages: 1, From: input, To: input})
	if err != nil {
		t.Fatal(err)
	}
	requester.done()
	m := result.Metadata
	if m.RequestedFrom != want || m.RequestedTo != want {
		t.Fatalf("history evidence describes different request bounds: %q/%q", m.RequestedFrom, m.RequestedTo)
	}
	if !m.ExplicitFrom || !m.ExplicitTo || m.DefaultWindow || m.WindowCompleteness != "unknown" || m.BankCapProven || !m.PaginationExhausted {
		t.Fatal("source wire adoption weakened history uncertainty")
	}
}
