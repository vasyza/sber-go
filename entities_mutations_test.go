package sber

import (
	"context"
	"errors"
	"testing"
)

func entityTransferProducts() Products {
	return Products{Accounts: []Account{{ID: "source-1", Kind: "ctaccount"}, {ID: "destination-1", Kind: "account"}}, Cards: []Card{{ID: "source-card"}}}
}
func TestEntityTransferToPreparesButNeverConfirms(t *testing.T) {
	for _, cardSource := range []bool{false, true} {
		sourceID := resourceFixtureSource
		if cardSource {
			sourceID = "card:source-card"
		}
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: resourceStartResponse()}, {Call: resourcePrepareCall(sourceID, resourceFixtureDestination, "10.50", "RUB", "Fixture purpose"), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)}}}
		p := NewBankPortfolio(entityTransferProducts(), r, ResourceOptions{AllowMutations: true})
		var got PreparedTransfer
		var err error
		if cardSource {
			got, err = p.Cards()[0].TransferTo(context.Background(), p.Accounts()[1], resourceAmount(t, "10.50"), TransferOptions{Currency: "RUB", PaymentPurpose: "Fixture purpose"})
		} else {
			got, err = p.Accounts()[0].TransferTo(context.Background(), p.Accounts()[1], resourceAmount(t, "10.50"), TransferOptions{Currency: "RUB", PaymentPurpose: "Fixture purpose"})
		}
		if err != nil || got.State() != "summary" || got.SourceID() != sourceID || r.sequences != 0 {
			t.Fatal("transfer helper confirmed or lost binding")
		}
		if p.Transfers() == nil {
			t.Fatal("standalone portfolio exposes no confirmation API")
		}
		r.done()
	}
}
func TestEntityAccountTransferToCardAcrossSameRequesterSnapshots(t *testing.T) {
	start := resourceStartResponse()
	refs := start["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)
	refs["toResource"].(map[string]any)["items"] = []any{map[string]any{"value": "card:destination-card", "properties": map[string]any{"type": "card", "name": "Synthetic destination card", "currency": "RUB"}}}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: start}, {Call: resourcePrepareCall(resourceFixtureSource, "card:destination-card", "1", "RUB", ""), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)}}}
	source := NewBankPortfolio(entityTransferProducts(), r, ResourceOptions{AllowMutations: true})
	destination := NewBankPortfolio(Products{Cards: []Card{{ID: "destination-card"}}}, r)
	prepared, err := source.Accounts()[0].TransferTo(context.Background(), destination.Cards()[0], resourceAmount(t, "1"))
	if err != nil || prepared.DestinationID() != "card:destination-card" || prepared.SourceID() != resourceFixtureSource || r.sequences != 0 {
		t.Fatal("same-requester snapshot/card destination transfer failed")
	}
	r.done()
}

func TestEntityTransferToRejectsCrossClientAndInvalidValuesBeforeStart(t *testing.T) {
	r := &resourceScript{t: t}
	p := NewBankPortfolio(entityTransferProducts(), r, ResourceOptions{AllowMutations: true})
	other := NewBankPortfolio(entityTransferProducts(), &resourceScript{t: t}, ResourceOptions{AllowMutations: true})
	if _, err := p.Accounts()[0].TransferTo(context.Background(), other.Accounts()[1], resourceAmount(t, "1")); err == nil {
		t.Fatal("cross-client transfer")
	}
	for _, dest := range []BankProduct{nil, (*BankCard)(nil), p.Accounts()[0], NewBankAccount(Account{ID: "bad id", Kind: "account"}, r, nil)} {
		if _, err := p.Accounts()[0].TransferTo(context.Background(), dest, resourceAmount(t, "1")); err == nil {
			t.Fatal("invalid destination")
		}
	}
	if _, err := p.Cards()[0].TransferTo(context.Background(), p.Accounts()[1], resourceAmount(t, "0")); err == nil {
		t.Fatal("invalid amount started workflow")
	}
	disabled := NewBankPortfolio(entityTransferProducts(), r)
	if _, err := disabled.Accounts()[0].TransferTo(context.Background(), disabled.Accounts()[1], resourceAmount(t, "1")); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("helper default gate")
	}
	if len(r.calls) != 0 {
		t.Fatal("invalid transfer helper sent mutation")
	}
}
func TestEntityCardRenameUsesBoundGuardAndKeepsSnapshot(t *testing.T) {
	call := resourceRenameCall("mutation", 4004, "Main")
	call.PageID = "/app/cards/details/4004"
	r := &resourceScript{t: t, steps: []resourceStep{{Call: call, Response: map[string]any{"success": true}}}}
	p := NewBankPortfolio(entityFixtureProducts(t), r, ResourceOptions{AllowMutations: true})
	card := p.Cards()[0]
	if err := card.Rename(context.Background(), "Main"); err != nil {
		t.Fatal(err)
	}
	if card.Name() != "Fixture card" {
		t.Fatal("rename mutated immutable snapshot")
	}
	r.done()
	disabled := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	if err := disabled.Cards()[0].Rename(context.Background(), "Main"); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("bound rename default enabled")
	}
}
