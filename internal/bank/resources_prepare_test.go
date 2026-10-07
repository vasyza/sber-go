package bank

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

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
