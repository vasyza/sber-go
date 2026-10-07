package sber

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func resourceRenameCall(kind string, id int64, name string) resourceCall {
	return resourceCall{Kind: kind, Path: "/ufs-productdetail/rest/v1/changeProductName", Payload: map[string]any{"id": id, "name": name, "type": "card"}, Query: map[string]string{}, PageID: "/app/cards/details/12345"}
}
func TestResourceRenameDefaultDisabledBeforeAnyMutation(t *testing.T) {
	r := &resourceScript{t: t}
	err := NewCardsAPI(r).Rename(context.Background(), 12345, "Valid")
	if !errors.Is(err, ErrResourceMutationsDisabled) || len(r.calls) != 0 {
		t.Fatalf("default writes enabled: %v", err)
	}
}
func TestResourceRenameExactPayloadAndValidation(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Новая карта, 1.-"), Response: map[string]any{"success": true}}}}
	api := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
	for _, name := range []string{"", "   ", strings.Repeat("a", 57), "slash/name", "underscore_name", "bad\nname", "é", "\x7f"} {
		if err := api.Rename(context.Background(), 12345, name); err == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
	if err := api.Rename(context.Background(), "0012345", "Новая карта, 1.-"); err != nil {
		t.Fatal(err)
	}
	r.done()
}
func TestResourceRenamePreservesDefiniteRejection(t *testing.T) {
	rejected := &APIRejected{Code: "5", Title: "Fixture", Text: "Fixture text", UUID: "fixture", System: "fixture"}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Valid"), Err: rejected}}}
	err := NewCardsAPI(r, ResourceOptions{AllowMutations: true}).Rename(context.Background(), 12345, "Valid")
	if err != rejected {
		t.Fatalf("lost definite rejection %v", err)
	}
	r.done()
}
func TestResourceRenameUncertaintyAndCancellationNeverReplay(t *testing.T) {
	for _, cause := range []error{&APIError{}, &AuthenticationExpired{}, &TransportError{}, context.Canceled, context.DeadlineExceeded} {
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceRenameCall("mutation", 12345, "Valid"), Err: cause}}}
		api := NewCardsAPI(r, ResourceOptions{AllowMutations: true})
		var uncertain *MutationUncertain
		if err := api.Rename(context.Background(), 12345, "Valid"); !errors.As(err, &uncertain) {
			t.Fatalf("not uncertain: %v", err)
		}
		if err := api.Rename(context.Background(), 12345, "Another"); !errors.As(err, &uncertain) || len(r.calls) != 1 {
			t.Fatal("repeated uncertain rename")
		}
		r.done()
	}
}
