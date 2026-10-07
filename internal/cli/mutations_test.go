package cli

import (
	"bytes"
	"context"
	"errors"
	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
	"github.com/vasyza/sber-go/internal/testutil"
	"github.com/vasyza/sber-go/mcp"
	"reflect"
	"strings"
	"testing"
)

var fixtureTransferArgs = []string{"transfer-own", "--profile", "synthetic-selected", "--source", "account:source", "--destination", "account:destination", "--amount", "10.50"}

func TestMutationCommandsDefaultToOfflinePlan(t *testing.T) {
	for _, args := range [][]string{{"card-rename", "--profile", "synthetic-selected", "--card-id", "123", "--name", "Fixture card"}, fixtureTransferArgs} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), args, &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { t.Fatal("preview opened a bank client"); return nil, nil }})
		if code != 0 || !strings.Contains(output.String(), `"bank_request_sent": false`) || !strings.Contains(output.String(), `"executed": false`) {
			t.Fatal("mutation preview did not describe its boundary")
		}
	}
}

func TestMutationConfirmationPrecedesBankClient(t *testing.T) {
	for _, response := range []string{"", "yes", "confirm"} {
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), append(append([]string{}, fixtureTransferArgs...), "--execute"), &output, &diagnostics, Options{
			OpenClient: func(string) (mcp.Client, error) { t.Fatal("unconfirmed action opened a bank client"); return nil, nil },
			Authentication: &Authentication{ReadSecret: func(_ context.Context, prompt ownerinput.Prompt) (string, error) {
				if prompt != ownerinput.ConfirmAction {
					t.Fatal("mutation requested login or payment secrets")
				}
				return response, nil
			}},
		})
		if code != 3 || output.Len() != 0 {
			t.Fatal("unconfirmed mutation accepted")
		}
	}
}

type mutationFixture struct {
	testutil.Client
	send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
}

func (m *mutationFixture) Mutate(ctx context.Context, path string, body map[string]any, query map[string]string, page string, workflow bool) (map[string]any, error) {
	return m.send(ctx, path, body, query, page, workflow)
}
func (m *mutationFixture) MutationSequence(ctx context.Context, action func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	return action(m.Mutate)
}

func TestRenameHasExplicitPolicyAndNoRetry(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		calls := 0
		client := &mutationFixture{}
		client.send = func(_ context.Context, path string, body map[string]any, _ map[string]string, _ string, workflow bool) (map[string]any, error) {
			calls++
			if path != sber.ChangeProductNamePath || workflow || !reflect.DeepEqual(body, map[string]any{"id": int64(123), "name": "Fixture card", "type": "card"}) {
				t.Fatal("rename request changed")
			}
			if uncertain {
				return nil, errors.New("synthetic-private-wire-error")
			}
			return map[string]any{"success": true}, nil
		}
		var output, diagnostics bytes.Buffer
		code := RunWithOptions(context.Background(), []string{"card-rename", "--profile", "synthetic-selected", "--card-id", "123", "--name", "Fixture card", "--execute"}, &output, &diagnostics, Options{
			Authentication: &Authentication{ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { return "CONFIRM", nil }},
			OpenClientWithOptions: func(_ context.Context, _ string, options sber.ClientOptions, provider sber.PINProvider) (mcp.Client, error) {
				if !options.AllowMutations || provider != nil || options.Renewal != nil {
					t.Fatal("mutation lacked explicit policy or enabled renewal")
				}
				return client, nil
			},
		})
		want := 0
		if uncertain {
			want = 4
		}
		if code != want || calls != 1 || client.Closes.Load() != 1 || uncertain && (output.Len() != 0 || !strings.Contains(diagnostics.String(), "Do not repeat the operation.")) || strings.Contains(diagnostics.String(), "synthetic-private-wire-error") {
			t.Fatalf("rename boundary failed: status=%d calls=%d", code, calls)
		}
	}
}

func TestTransferRunsOneWorkflowAndOneConfirmationSequence(t *testing.T) {
	calls := []string{}
	confirmations := 0
	client := &mutationFixture{}
	response := func(result, flow, state string) map[string]any {
		return map[string]any{"success": true, "body": map[string]any{"pid": "fixture-workflow", "result": result, "flow": flow, "state": state}}
	}
	client.send = func(_ context.Context, path string, body map[string]any, query map[string]string, _ string, workflow bool) (map[string]any, error) {
		calls = append(calls, query["name"])
		if !workflow {
			t.Fatal("transfer lost workflow policy")
		}
		switch query["name"] {
		case "me2meMain_v2":
			p := response("SUCCESS", "me2meCreate", "transferRequisites")
			item := func(id string) map[string]any {
				return map[string]any{"value": id, "properties": map[string]any{"type": "account", "currency": "RUB"}}
			}
			p["body"].(map[string]any)["output"] = map[string]any{"references": map[string]any{"fromResource": map[string]any{"items": []any{item("account:source")}}, "toResource": map[string]any{"items": []any{item("account:destination")}}}}
			return p, nil
		case "next":
			if body["fields"].(map[string]any)["transfer:me2me:sum"] != "10.50" {
				t.Fatal("transfer rounded money")
			}
			return response("SUCCESS", "me2meCreate", "summary"), nil
		case "summaryNext":
			if confirmations != 2 || path != sber.Me2MeWorkflowPath {
				t.Fatal("payment proceeded without final owner confirmation")
			}
			p := response("EXTERNAL_ENTER", "", "")
			p["body"].(map[string]any)["url"] = sber.ConfirmationWorkflowPath
			return p, nil
		case "on-enter":
			if path != sber.ConfirmationWorkflowPath {
				t.Fatal("confirmation endpoint changed")
			}
			p := response("EXTERNAL_RETURN", "", "")
			p["body"].(map[string]any)["url"] = sber.Me2MeWorkflowPath
			return p, nil
		case "on-return":
			p := response("SUCCESS", "me2meInfo", "showInfo")
			p["body"].(map[string]any)["output"] = map[string]any{"document": map[string]any{"srcDocumentId": "synthetic-private-document"}, "screens": []any{map[string]any{"header": []any{map[string]any{"properties": map[string]any{"level": "done"}}}}}}
			return p, nil
		}
		t.Fatal("unknown transfer stage")
		return nil, nil
	}
	var output, diagnostics bytes.Buffer
	code := RunWithOptions(context.Background(), append(append([]string{}, fixtureTransferArgs...), "--execute"), &output, &diagnostics, Options{OpenClient: func(string) (mcp.Client, error) { return client, nil }, Authentication: &Authentication{ReadSecret: func(context.Context, ownerinput.Prompt) (string, error) { confirmations++; return "CONFIRM", nil }}})
	if code != 0 || !reflect.DeepEqual(calls, []string{"me2meMain_v2", "next", "summaryNext", "on-enter", "on-return"}) || client.Closes.Load() != 1 || strings.Contains(output.String()+diagnostics.String(), "synthetic-private-document") || !strings.Contains(output.String(), `"transfer_confirmed": true`) {
		t.Fatalf("transfer workflow failed: status=%d stages=%d", code, len(calls))
	}
}
