package bank

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

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
