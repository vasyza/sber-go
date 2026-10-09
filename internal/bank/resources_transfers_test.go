package bank

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

func resourceDoneOutput() map[string]any {
	return map[string]any{"document": map[string]any{"srcDocumentId": "fixture-document"}, "screens": []any{map[string]any{"header": []any{map[string]any{"properties": map[string]any{"level": "done"}}}}}}
}
func resourceConfirmCall(stage int) resourceCall {
	c := resourceCall{Kind: "sequence", Path: "/me2me/v1/workflow", Payload: map[string]any{}, Query: map[string]string{"cmd": "EVENT", "pid": resourceFixturePID}, PageID: "/app/payments/self/workflow?action=CREATE&pid=" + resourceFixturePID, Workflow: true}
	switch stage {
	case 0:
		c.Payload = map[string]any{"document": map[string]any{"flow": "me2meCreate", "state": "summary"}}
		c.Query["name"] = "summaryNext"
	case 1:
		c.Path = "/bh-confirmation/v3/workflow2"
		c.Query["name"] = "on-enter"
	case 2:
		c.Query["name"] = "on-return"
	}
	return c
}
func resourceConfirmResponses() []map[string]any {
	return []map[string]any{resourceWorkflowResponse("EXTERNAL_ENTER", "", "", "/bh-confirmation/v3/workflow2", nil), resourceWorkflowResponse("EXTERNAL_RETURN", "", "", "/me2me/v1/workflow", nil), resourceWorkflowResponse("SUCCESS", "me2meInfo", "showInfo", "", resourceDoneOutput())}
}
func resourceReadyTransfer(t *testing.T, steps ...resourceStep) (*TransfersAPI, *resourceScript, PreparedTransfer) {
	t.Helper()
	steps = append([]resourceStep{{Call: resourcePrepareCall(resourceFixtureSource, resourceFixtureDestination, "10.50", "RUB", ""), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)}}, steps...)
	a, r, d := resourcePreparedAPI(t, steps...)
	p, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "10.50"))
	if err != nil {
		t.Fatal(err)
	}
	return a, r, p
}
func TestResourceTransferConfirmExactThreePostSequence(t *testing.T) {
	var steps []resourceStep
	for i, p := range resourceConfirmResponses() {
		steps = append(steps, resourceStep{Call: resourceConfirmCall(i), Response: p})
	}
	a, r, p := resourceReadyTransfer(t, steps...)
	got, err := a.Confirm(context.Background(), p)
	if err != nil || got.PID() != resourceFixturePID || got.Flow() != "me2meInfo" || got.State() != "showInfo" || got.DocumentID() == nil || *got.DocumentID() != "fixture-document" || r.sequences != 1 {
		t.Fatalf("confirm: %v %v", got, err)
	}
	if _, err = a.Confirm(context.Background(), p); err == nil || len(r.calls) != 5 {
		t.Fatal("confirmed twice")
	}
	r.done()
}
func TestResourceTransferConfirmRejectsForeignPreparedAndDefaultGate(t *testing.T) {
	a, r, p := resourceReadyTransfer(t)
	forged := NewPreparedTransfer(p.PID(), p.Flow(), p.State(), p.SourceID(), p.DestinationID(), p.Amount(), p.PaymentPurpose())
	if _, err := a.Confirm(context.Background(), forged); err == nil || len(r.calls) != 2 || r.sequences != 0 {
		t.Fatal("forged prepared sent")
	}
	other := NewTransfersAPI(r)
	if _, err := other.Confirm(context.Background(), p); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("default confirm enabled")
	}
	r.done()
}
func TestResourceTransferConfirmFailsClosedAtTransitions(t *testing.T) {
	changes := []struct {
		stage  int
		change func(map[string]any)
	}{
		{0, func(p map[string]any) {
			p["body"].(map[string]any)["url"] = "https://fixture.invalid/bh-confirmation/v3/workflow2"
		}},
		{0, func(p map[string]any) { p["body"].(map[string]any)["result"] = "SUCCESS" }},
		{0, func(p map[string]any) { p["body"].(map[string]any)["pid"] = "other" }},
		{1, func(p map[string]any) { p["body"].(map[string]any)["url"] = "/unexpected" }},
		{1, func(p map[string]any) {
			p["body"].(map[string]any)["result"] = "SUCCESS"
			p["body"].(map[string]any)["flow"] = "otp"
		}},
		{2, func(p map[string]any) { p["body"].(map[string]any)["flow"] = "me2meCreate" }},
		{2, func(p map[string]any) { p["body"].(map[string]any)["state"] = "summary" }},
		{2, func(p map[string]any) { p["body"].(map[string]any)["pid"] = "other" }},
		{2, func(p map[string]any) { p["body"].(map[string]any)["output"] = nil }},
		{2, func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["document"] = map[string]any{"srcDocumentId": true}
		}},
		{2, func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["document"] = map[string]any{"srcDocumentId": ""}
		}},
		{2, func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["document"] = map[string]any{"srcDocumentId": strings.Repeat("a", 129)}
		}},
		{2, func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["document"] = map[string]any{"srcDocumentId": "bad\x7f"}
		}},
		{2, func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["screens"] = []any{map[string]any{"header": []any{map[string]any{"properties": map[string]any{"level": "DONE"}}}}}
		}},
		{2, func(p map[string]any) { p["success"] = false }},
	}
	for i, tc := range changes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			responses := resourceConfirmResponses()
			tc.change(responses[tc.stage])
			var steps []resourceStep
			for stage := 0; stage <= tc.stage; stage++ {
				steps = append(steps, resourceStep{Call: resourceConfirmCall(stage), Response: responses[stage]})
			}
			a, r, p := resourceReadyTransfer(t, steps...)
			_, err := a.Confirm(context.Background(), p)
			var uncertain *sdkErrs.MutationUncertain
			if !errors.As(err, &uncertain) {
				t.Fatalf("invalid confirm not uncertain %v", err)
			}
			before := len(r.calls)
			if _, err = a.Confirm(context.Background(), p); err == nil || len(r.calls) != before {
				t.Fatal("uncertain confirmation repeated")
			}
			r.done()
		})
	}
}
func TestResourceTransferConfirmErrorsAndCancellationAreOneShot(t *testing.T) {
	for stage := 0; stage < 3; stage++ {
		for i, cause := range []error{&sdkErrs.APIError{}, &sdkErrs.APIRejected{}, &sdkErrs.AuthenticationExpired{}, &sdkErrs.TransportError{}, context.Canceled, context.DeadlineExceeded} {
			t.Run(fmt.Sprintf("%d-%d", stage, i), func(t *testing.T) {
				responses := resourceConfirmResponses()
				var steps []resourceStep
				for n := 0; n <= stage; n++ {
					s := resourceStep{Call: resourceConfirmCall(n), Response: responses[n]}
					if n == stage {
						s.Err = cause
						s.Response = nil
					}
					steps = append(steps, s)
				}
				a, r, p := resourceReadyTransfer(t, steps...)
				_, err := a.Confirm(context.Background(), p)
				var uncertain *sdkErrs.MutationUncertain
				if !errors.As(err, &uncertain) {
					t.Fatal("not uncertain")
				}
				before := len(r.calls)
				if _, err = a.Confirm(context.Background(), p); err == nil || len(r.calls) != before {
					t.Fatal("error replayed")
				}
				r.done()
			})
		}
	}
}

func resourceAmount(t *testing.T, s string) Decimal {
	t.Helper()
	d, err := ParseDecimal(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
func resourcePrepareCall(source, destination, amount, currency, purpose string) resourceCall {
	return resourceCall{Kind: "mutation", Path: "/me2me/v1/workflow", Payload: map[string]any{"fields": map[string]any{"transfer:me2me:fromResource": source, "transfer:me2me:toResource": destination, "transfer:me2me:sum": amount, "transfer:me2me:sum:currency": currency, "transfer:me2me:paymentPurpose": purpose}, "document": map[string]any{"flow": "me2meCreate", "state": "transferRequisites"}}, Query: map[string]string{"cmd": "EVENT", "name": "next", "pid": resourceFixturePID}, PageID: "/app/payments/self/workflow?action=CREATE&pid=" + resourceFixturePID, Workflow: true}
}
func resourcePreparedAPI(t *testing.T, steps ...resourceStep) (*TransfersAPI, *resourceScript, TransferDraft) {
	t.Helper()
	r := &resourceScript{t: t, steps: append([]resourceStep{{Call: resourceStartCall(), Response: resourceStartResponse()}}, steps...)}
	api := NewTransfersAPI(r, ResourceOptions{AllowMutations: true})
	d, err := api.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return api, r, d
}
func TestResourceTransferPrepareExactDecimalWireNormalization(t *testing.T) {
	for _, tc := range [][2]string{{"10.50", "10.50"}, {"1.000", "1"}, {"1.200", "1.2"}, {"0.0100", "0.01"}, {"1.00", "1.00"}, {"1e2", "100"}, {"999999999999999.000", "999999999999999"}} {
		t.Run(tc[0], func(t *testing.T) {
			a, r, d := resourcePreparedAPI(t, resourceStep{Call: resourcePrepareCall(resourceFixtureSource, resourceFixtureDestination, tc[1], "RUB", "Между своими продуктами"), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)})
			amount := resourceAmount(t, tc[0])
			got, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, amount, TransferOptions{Currency: "RUB", PaymentPurpose: "Между своими продуктами"})
			if err != nil || got.PID() != resourceFixturePID || got.Flow() != "me2meCreate" || got.State() != "summary" || got.SourceID() != resourceFixtureSource || got.DestinationID() != resourceFixtureDestination || got.Amount().Amount.String() != amount.String() || got.PaymentPurpose() != "Между своими продуктами" {
				t.Fatalf("prepare: %v %v", got, err)
			}
			if _, err = a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "5000")); err == nil || len(r.calls) != 2 {
				t.Fatal("reprepared draft")
			}
			r.done()
		})
	}
}
func TestResourceTransferPrepareValidationBeforeNetwork(t *testing.T) {
	a, r, d := resourcePreparedAPI(t)
	for _, s := range []string{"0", "-1", "0.001", "1e15", "1e100000000", "1e-100000000", "1.000000000000000000", "999999999999999.0000"} {
		if _, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, s)); err == nil {
			t.Fatalf("accepted amount %s", s)
		}
	}
	for _, id := range []string{" account:bad", "card:not-in-start", "transactionAccount:", "account:" + strings.Repeat("a", 129)} {
		if _, err := a.Prepare(context.Background(), d, id, resourceFixtureDestination, resourceAmount(t, "1")); err == nil {
			t.Fatal("bad reference")
		}
	}
	if _, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureSource, resourceAmount(t, "1")); err == nil {
		t.Fatal("same resource")
	}
	for _, o := range []TransferOptions{{Currency: "rub"}, {Currency: "USD"}, {Currency: ""}, {Currency: "RUB", PaymentPurpose: strings.Repeat("a", 211)}, {Currency: "RUB", PaymentPurpose: "bad\ntext"}} {
		if _, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "1"), o); err == nil {
			t.Fatal("invalid currency/purpose")
		}
	}
	forged := NewTransferDraft(d.PID(), d.Flow(), d.State(), d.Sources(), d.Destinations())
	if _, err := a.Prepare(context.Background(), forged, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "1")); err == nil {
		t.Fatal("unissued draft")
	}
	if len(r.calls) != 1 {
		t.Fatal("invalid prepare reached mutation")
	}
	r.done()
}
func TestResourceTransferPrepareFailureIsOneShot(t *testing.T) {
	for i, cause := range []error{&sdkErrs.TransportError{}, context.Canceled, context.DeadlineExceeded, &sdkErrs.AuthenticationExpired{}, &sdkErrs.APIRejected{Code: "fixture"}} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			a, r, d := resourcePreparedAPI(t, resourceStep{Call: resourcePrepareCall(resourceFixtureSource, resourceFixtureDestination, "1", "RUB", ""), Err: cause})
			_, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "1"))
			if err == nil {
				t.Fatal("lost failure")
			}
			var rejected *sdkErrs.APIRejected
			var unknown *sdkErrs.MutationUncertain
			if errors.As(cause, &rejected) {
				if err != cause {
					t.Fatal("lost definite rejection")
				}
			} else if !errors.As(err, &unknown) {
				t.Fatal("not uncertain")
			}
			if _, err = a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "2")); err == nil || len(r.calls) != 2 {
				t.Fatal("failed prepare replayed")
			}
			r.done()
		})
	}
}
func TestResourceTransferPrepareDefaultDisabledEvenForImportedDraft(t *testing.T) {
	r := &resourceScript{t: t}
	d := NewTransferDraft(resourceFixturePID, "me2meCreate", "transferRequisites", nil, nil)
	_, err := NewTransfersAPI(r).Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "1"))
	if !errors.Is(err, ErrResourceMutationsDisabled) || len(r.calls) != 0 {
		t.Fatal("prepare gate")
	}
}

func TestResourceConcurrentPrepareIsOneShot(t *testing.T) {
	a, r, d := resourcePreparedAPI(t, resourceStep{Call: resourcePrepareCall(resourceFixtureSource, resourceFixtureDestination, "1", "RUB", ""), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)})
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Prepare(context.Background(), d, resourceFixtureSource, resourceFixtureDestination, resourceAmount(t, "1")); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || len(r.calls) != 2 {
		t.Fatal("concurrent prepare replay")
	}
	r.done()
}
func TestResourceConcurrentConfirmIsOneShot(t *testing.T) {
	var steps []resourceStep
	for i, p := range resourceConfirmResponses() {
		steps = append(steps, resourceStep{Call: resourceConfirmCall(i), Response: p})
	}
	a, r, p := resourceReadyTransfer(t, steps...)
	var wg sync.WaitGroup
	var successes atomic.Int64
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Confirm(context.Background(), p); err == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || len(r.calls) != 5 || r.sequences != 1 {
		t.Fatal("concurrent confirm replay")
	}
	r.done()
}
func TestResourceConfirmSequenceDoesNotReacquireMutationOrInterleaveRename(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var steps []resourceStep
	for i, p := range resourceConfirmResponses() {
		s := resourceStep{Call: resourceConfirmCall(i), Response: p}
		if i == 0 {
			s.Before = func(context.Context) { close(entered); <-release }
		}
		steps = append(steps, s)
	}
	steps = append(steps, resourceStep{Call: resourceRenameCall("mutation", 12345, "Valid"), Response: map[string]any{"success": true}})
	a, r, p := resourceReadyTransfer(t, steps...)
	confirmed := make(chan error, 1)
	go func() { _, err := a.Confirm(context.Background(), p); confirmed <- err }()
	<-entered
	cards := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
	renamed := make(chan error, 1)
	renameStarted := make(chan struct{})
	go func() { close(renameStarted); renamed <- cards.Rename(context.Background(), 12345, "Valid") }()
	<-renameStarted
	select {
	case err := <-renamed:
		t.Fatalf("rename interleaved: %v", err)
	default:
	}
	close(release)
	if err := <-confirmed; err != nil {
		t.Fatal(err)
	}
	if err := <-renamed; err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 6 || r.calls[5].Path != "/ufs-productdetail/rest/v1/changeProductName" {
		t.Fatal("sequence not contiguous")
	}
	r.done()
}
func TestResourceCanceledRenameAfterSenderBeginsCannotReplay(t *testing.T) {
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Valid"), Before: func(ctx context.Context) { close(entered); <-ctx.Done() }, Err: context.Canceled}}}
	cards := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
	done := make(chan error, 1)
	go func() { done <- cards.Rename(ctx, 12345, "Valid") }()
	<-entered
	cancel()
	err := <-done
	var unknown *sdkErrs.MutationUncertain
	if !errors.As(err, &unknown) {
		t.Fatal("cancel not uncertain")
	}
	if err = cards.Rename(context.Background(), 12345, "Valid"); !errors.As(err, &unknown) || len(r.calls) != 1 {
		t.Fatal("canceled rename repeated")
	}
	r.done()
}
func TestEntityConcurrentSnapshotAccessPreservesIdentityAndMoney(t *testing.T) {
	p := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	var wg sync.WaitGroup
	for n := 0; n < 24; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				raw := p.Raw()
				raw.Accounts[0].Balance.Currency = "USD"
				c := p.Cards()[0]
				v := c.Snapshot()
				v.Balance.Currency = "USD"
				if c.Account() != p.Accounts()[0] || c.Account().Balance().Currency != "RUB" {
					t.Error("snapshot race")
				}
				_ = fmt.Sprint(p)
			}
		}()
	}
	wg.Wait()
}

const resourceFixturePID = "fixture-workflow-pid"
const resourceFixtureSource = "transactionAccount:source-1"
const resourceFixtureDestination = "account:destination-1"

func resourceWorkflowResponse(result, flow, state, url string, output any) map[string]any {
	b := map[string]any{"pid": resourceFixturePID, "result": result}
	if flow != "" {
		b["flow"] = flow
	}
	if state != "" {
		b["state"] = state
	}
	if url != "" {
		b["url"] = url
	}
	if output != nil {
		b["output"] = output
	}
	return map[string]any{"success": true, "body": b}
}
func resourceStartResponse() map[string]any {
	item := func(id, kind string) map[string]any {
		return map[string]any{"value": id, "title": "Fixture product", "properties": map[string]any{"type": kind, "name": "Fixture product", "currency": "RUB"}}
	}
	refs := map[string]any{"fromResource": map[string]any{"items": []any{item(resourceFixtureSource, "payAccount"), item("card:source-card", "card")}}, "toResource": map[string]any{"items": []any{item(resourceFixtureDestination, "account"), item(resourceFixtureSource, "payAccount")}}}
	return resourceWorkflowResponse("SUCCESS", "me2meCreate", "transferRequisites", "", map[string]any{"references": refs})
}
func resourceStartCall() resourceCall {
	return resourceCall{Kind: "mutation", Path: "/me2me/v1/workflow", Payload: map[string]any{"document": map[string]any{"action": "CREATE"}}, Query: map[string]string{"cmd": "START", "name": "me2meMain_v2"}, PageID: "/app/payments/self/workflow?action=CREATE", Workflow: true}
}
func TestResourceTransferStartExactWorkflowAndReferences(t *testing.T) {
	response := resourceStartResponse()
	refs := response["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)
	refs["fromResource"].(map[string]any)["items"].([]any)[0].(map[string]any)["properties"].(map[string]any)["name"] = "Карта 4111 1111 1111 1111"
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: response}}}
	draft, err := NewTransfersAPI(r, ResourceOptions{AllowMutations: true}).Start(context.Background())
	if err != nil || draft.PID() != resourceFixturePID || draft.Flow() != "me2meCreate" || draft.State() != "transferRequisites" || len(draft.Sources()) != 2 || draft.Sources()[0].ID() != resourceFixtureSource || draft.Sources()[0].Currency() != "RUB" || strings.Contains(draft.Sources()[0].Name(), "4111 1111 1111") {
		t.Fatalf("start: %v %v", draft, err)
	}
	r.done()
}
func TestResourceTransferStartRejectsMalformedWorkflow(t *testing.T) {
	changes := []func(map[string]any){
		func(p map[string]any) { p["success"] = false },
		func(p map[string]any) { p["body"] = []any{} },
		func(p map[string]any) { p["body"].(map[string]any)["pid"] = "bad pid" },
		func(p map[string]any) { p["body"].(map[string]any)["flow"] = "other" },
		func(p map[string]any) { p["body"].(map[string]any)["state"] = "summary" },
		func(p map[string]any) { p["body"].(map[string]any)["result"] = "EXTERNAL_ENTER" },
		func(p map[string]any) { p["body"].(map[string]any)["output"] = nil },
		func(p map[string]any) { p["body"].(map[string]any)["output"].(map[string]any)["references"] = []any{} },
		func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)["fromResource"] = nil
		},
		func(p map[string]any) {
			p["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)["fromResource"].(map[string]any)["items"] = []any{map[string]any{"value": " card:bad", "properties": map[string]any{}}}
		},
		func(p map[string]any) {
			ref := p["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)["fromResource"].(map[string]any)
			items := ref["items"].([]any)
			ref["items"] = append(items, items[0])
		},
	}
	for i, change := range changes {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			p := resourceStartResponse()
			change(p)
			r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: p}}}
			a := NewTransfersAPI(r, ResourceOptions{AllowMutations: true})
			if _, err := a.Start(context.Background()); err == nil {
				t.Fatal("malformed workflow accepted")
			}
			if _, err := a.Start(context.Background()); err == nil || len(r.calls) != 1 {
				t.Fatal("malformed mutation replayed")
			}
			r.done()
		})
	}
}
func TestResourceTransferStartDefaultDisabled(t *testing.T) {
	r := &resourceScript{t: t}
	_, err := NewTransfersAPI(r).Start(context.Background())
	if !errors.Is(err, ErrResourceMutationsDisabled) || len(r.calls) != 0 {
		t.Fatal("start default gate")
	}
}
