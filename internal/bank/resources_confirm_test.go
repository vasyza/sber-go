package bank

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
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
