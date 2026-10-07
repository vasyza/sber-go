package sber

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

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
	var uncertain *MutationUncertain
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
