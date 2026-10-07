package bank

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"sync"
	"testing"

	sdkTransport "github.com/vasyza/sber-go/internal/transport"
)

// Turn audited synthetic resource scripts into actual client transport calls.
// This exercises core policy, headers, response validation and serialization;
// it does not replace the client or requester with a permissive mock.
func clientBindingScript(t *testing.T, allow bool, steps []resourceStep) (*SberClient, *clientFakeTransport) {
	t.Helper()
	bundle := clientFixture(t, "binding-workflow")
	bundle.AntifraudDeviceprint = sdkTransport.PtrString("synthetic-observed-antifraud")
	tr := clientFake(t, bundle)
	var mu sync.Mutex
	next := 0
	tr.post = func(_ context.Context, target string, body map[string]any, o sdkTransport.RequestOptions) (*sdkTransport.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if next >= len(steps) {
			t.Error("unexpected/repeated synthetic request")
			return nil, fmt.Errorf("unexpected synthetic request")
		}
		step := steps[next]
		next++
		u, err := url.Parse(target)
		if err != nil {
			t.Error(err)
			return nil, err
		}
		query := map[string]string{}
		for key, values := range u.Query() {
			if len(values) != 1 {
				t.Error("repeated query key")
			}
			query[key] = values[0]
		}
		wantQuery := step.Call.Query
		if wantQuery == nil {
			wantQuery = map[string]string{}
		}
		if u.Scheme+"://"+u.Host != bundle.APIBase || u.Path != step.Call.Path || !reflect.DeepEqual(body, step.Call.Payload) || !reflect.DeepEqual(query, wantQuery) {
			t.Errorf("synthetic call %d differs from source script", next)
			return nil, fmt.Errorf("synthetic request mismatch")
		}
		if step.Call.Kind != "read" {
			if o.Headers["RSA-Antifraud-Device-Print"] == nil || *o.Headers["RSA-Antifraud-Device-Print"] != *bundle.AntifraudDeviceprint || o.Headers["RSA-Antifraud-Page-Id"] == nil || *o.Headers["RSA-Antifraud-Page-Id"] != step.Call.PageID {
				t.Error("mutation did not use core observed identity/page headers")
			}
			if step.Call.Workflow && (o.Headers["X-Workflow-Options"] == nil || *o.Headers["X-Workflow-Options"] != "3.0") {
				t.Error("workflow header missing")
			}
		}
		if step.Err != nil {
			return nil, step.Err
		}
		return clientBindingResponse(t, step.Response), nil
	}
	options := ClientOptions{Transport: tr, AllowMutations: allow}
	c, err := NewSberClient(bundle, options)
	if err != nil {
		t.Fatal(err)
	}
	// The public value options must not retroactively toggle either policy.
	options.AllowMutations = !allow
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c, tr
}

func clientBindingStartResponse() map[string]any {
	response := resourceStartResponse()
	refs := response["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)
	refs["fromResource"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "transactionAccount:1001"
	refs["toResource"].(map[string]any)["items"].([]any)[0].(map[string]any)["value"] = "card:4004"
	return response
}

func clientBindingSuccessfulWorkflowSteps() []resourceStep {
	steps := []resourceStep{
		{Call: resourceStartCall(), Response: clientBindingStartResponse()},
		{Call: resourcePrepareCall("transactionAccount:1001", "card:4004", "10.50", "RUB", ""), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)},
	}
	for i, response := range resourceConfirmResponses() {
		steps = append(steps, resourceStep{Call: resourceConfirmCall(i), Response: response})
	}
	return steps
}

func TestClientBindingOptInSharesIssuerAcrossEntityAndDirectAPIs(t *testing.T) {
	steps := []resourceStep{
		{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()},
		{Call: resourceProductsCall(true), Response: resourcePortfolioResponse()},
	}
	steps = append(steps, clientBindingSuccessfulWorkflowSteps()...)
	c, tr := clientBindingScript(t, true, steps)
	first, second := clientBindingPortfolio(t, c, false), clientBindingPortfolio(t, c, true)
	prepared, err := first.Accounts()[0].TransferTo(context.Background(), second.Cards()[0], resourceAmount(t, "10.50"))
	if err != nil {
		t.Fatalf("explicit client opt-in not propagated to bound entity workflow: %v", err)
	}
	if n, _, _ := tr.counts(); n != 4 {
		t.Fatal("TransferTo unexpectedly confirmed/retried")
	}
	foreign, foreignTransport := clientBindingScript(t, true, nil)
	if _, err := foreign.Transfers().Confirm(context.Background(), prepared); err == nil {
		t.Fatal("foreign client accepted prepared snapshot")
	}
	forged := NewPreparedTransfer(prepared.PID(), prepared.Flow(), prepared.State(), prepared.SourceID(), prepared.DestinationID(), prepared.Amount(), prepared.PaymentPurpose())
	if _, err := c.Transfers().Confirm(context.Background(), forged); err == nil {
		t.Fatal("reconstructed prepared snapshot accepted")
	}
	if n, _, _ := foreignTransport.counts(); n != 0 {
		t.Fatal("foreign prepared workflow caused request")
	}
	result, err := second.Transfers().Confirm(context.Background(), prepared)
	if err != nil || result.DocumentID() == nil || *result.DocumentID() != "fixture-document" {
		t.Fatalf("shared issuer confirmation: %v", err)
	}
	if _, err := c.Transfers().Confirm(context.Background(), prepared); err == nil {
		t.Fatal("reused prepared transfer sent")
	}
	if n, _, _ := tr.counts(); n != len(steps) {
		t.Fatal("not exact two reads + Start/Prepare + three confirmation POSTs")
	}
	clientBindingCheckSnapshot(t, c, first)
	clientBindingCheckSnapshot(t, c, second)
}

func TestClientBindingDefaultReadOnlyBlocksAllEntityWritesBeforeRequest(t *testing.T) {
	c, tr := clientBindingScript(t, false, []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}})
	p := clientBindingPortfolio(t, c, false)
	ctx := context.Background()
	if err := p.Cards()[0].Rename(ctx, "Fixture name"); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatalf("entity rename gate: %v", err)
	}
	if _, err := p.Accounts()[0].TransferTo(ctx, p.Cards()[0], resourceAmount(t, "10.50")); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatalf("entity transfer gate: %v", err)
	}
	if _, err := c.Transfers().Start(ctx); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("start default enabled")
	}
	if _, err := c.Transfers().Prepare(ctx, TransferDraft{}, "transactionAccount:1001", "card:4004", resourceAmount(t, "10.50")); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("prepare default enabled")
	}
	if _, err := c.Transfers().Confirm(ctx, PreparedTransfer{}); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("confirm default enabled")
	}
	// An explicitly enabled *different* resource bundle cannot elevate the core.
	if err := NewResources(c, ResourceOptions{AllowMutations: true}).Cards.Rename(ctx, 4004, "Fixture name"); err == nil {
		t.Fatal("resource gate bypass elevated core policy")
	}
	if n, _, _ := tr.counts(); n != 1 {
		t.Fatal("default entity mutation caused a REQUEST")
	}
}
