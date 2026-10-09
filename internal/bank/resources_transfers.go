package bank

import (
	"context"
	"errors"
	"regexp"
	"sync"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

const (
	Me2MeWorkflowPath        = "/me2me/v1/workflow"
	ConfirmationWorkflowPath = "/bh-confirmation/v3/workflow2"
)

var resourceTransferGrammar = regexp.MustCompile(`^(?:transactionAccount|card|account):[A-Za-z0-9_-]{1,128}$`)

// TransfersAPI is a one-shot, explicit-confirmation workflow owner. Constructors
// default to read-only. It never reauthenticates or replays a failed mutation.
type TransfersAPI struct {
	requester            BusinessRequester
	options              ResourceOptions
	mu                   sync.Mutex
	startUncertain       bool
	drafts               map[string]TransferDraft
	prepareAttempts      map[string]bool
	prepared             map[string]PreparedTransfer
	confirmationAttempts map[string]bool
}

func NewTransfersAPI(requester BusinessRequester, options ...ResourceOptions) *TransfersAPI {
	return &TransfersAPI{requester: requester, options: resourceOptions(options), drafts: map[string]TransferDraft{}}
}
func resourceWorkflowBody(payload map[string]any, result, pid, flow, state string) (map[string]any, error) {
	if err := domainEnvelope(payload); err != nil {
		return nil, err
	}
	b, err := domainMapping(payload["body"], "body")
	if err != nil {
		return nil, err
	}
	for key, want := range map[string]string{"result": result, "pid": pid, "flow": flow, "state": state} {
		if want != "" && b[key] != want {
			return nil, NewParseError("workflow." + key)
		}
	}
	return b, nil
}
func resourceWorkflowPID(value any) (string, error) {
	s, ok := value.(string)
	if !ok || !resourceOpaqueID.MatchString(s) {
		return "", NewParseError("workflow.pid")
	}
	return s, nil
}
func resourceTransferResources(references map[string]any, name string) ([]TransferResource, error) {
	ref, err := domainMapping(references[name], "references."+name)
	if err != nil {
		return nil, err
	}
	items, ok := ref["items"].([]any)
	if !ok {
		return nil, NewParseError("references." + name + ".items")
	}
	resources := make([]TransferResource, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		raw, err := domainMapping(item, "reference.item")
		if err != nil {
			return nil, err
		}
		id, ok := raw["value"].(string)
		if !ok || !resourceTransferGrammar.MatchString(id) || seen[id] {
			return nil, NewParseError("reference.id")
		}
		seen[id] = true
		properties, err := domainMapping(raw["properties"], "reference.properties")
		if err != nil {
			return nil, err
		}
		name := properties["name"]
		if !domainTruthy(name) {
			name = raw["title"]
		}
		if !domainTruthy(name) {
			name = ""
		}
		kind, currency := properties["type"], properties["currency"]
		if !domainTruthy(kind) {
			kind = ""
		}
		if !domainTruthy(currency) {
			currency = ""
		}
		resources = append(resources, NewTransferResource(id, domainID(kind), domainText(name), domainID(currency)))
	}
	return resources, nil
}
func (a *TransfersAPI) Start(ctx context.Context) (TransferDraft, error) {
	if !a.options.AllowMutations {
		return TransferDraft{}, ErrResourceMutationsDisabled
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.startUncertain {
		return TransferDraft{}, &sdkErrs.MutationUncertain{Message: "workflow creation already uncertain; do not repeat"}
	}
	if err := ctx.Err(); err != nil {
		return TransferDraft{}, err
	}
	payload, err := a.requester.Mutate(ctx, Me2MeWorkflowPath, map[string]any{"document": map[string]any{"action": "CREATE"}}, map[string]string{"cmd": "START", "name": "me2meMain_v2"}, "/app/payments/self/workflow?action=CREATE", true)
	if err != nil {
		var rejected *sdkErrs.APIRejected
		var uncertain *sdkErrs.MutationUncertain
		if errors.As(err, &rejected) && !errors.As(err, &uncertain) {
			return TransferDraft{}, err
		}
		a.startUncertain = true
		return TransferDraft{}, errors.Join(&sdkErrs.MutationUncertain{Message: "workflow creation result unknown"}, clientSafeError(err))
	}
	draft, err := resourceParseDraft(payload)
	if err != nil {
		a.startUncertain = true
		return TransferDraft{}, &sdkErrs.MutationUncertain{Message: "workflow creation response invalid; result unknown"}
	}
	if _, exists := a.drafts[draft.PID()]; exists {
		a.startUncertain = true
		delete(a.drafts, draft.PID())
		delete(a.prepared, draft.PID())
		return TransferDraft{}, &sdkErrs.MutationUncertain{Message: "server reused workflow PID; do not repeat"}
	}
	a.drafts[draft.PID()] = draft
	return draft, nil
}
func resourceParseDraft(payload map[string]any) (TransferDraft, error) {
	body, err := resourceWorkflowBody(payload, "SUCCESS", "", "me2meCreate", "transferRequisites")
	if err != nil {
		return TransferDraft{}, err
	}
	pid, err := resourceWorkflowPID(body["pid"])
	if err != nil {
		return TransferDraft{}, err
	}
	output, err := domainMapping(body["output"], "body.output")
	if err != nil {
		return TransferDraft{}, err
	}
	refs, err := domainMapping(output["references"], "body.output.references")
	if err != nil {
		return TransferDraft{}, err
	}
	sources, err := resourceTransferResources(refs, "fromResource")
	if err != nil {
		return TransferDraft{}, err
	}
	destinations, err := resourceTransferResources(refs, "toResource")
	if err != nil {
		return TransferDraft{}, err
	}
	return NewTransferDraft(pid, "me2meCreate", "transferRequisites", sources, destinations), nil
}
