package bank

import (
	"context"
	"errors"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-go/internal/errs"
)

func resourceWorkflowURL(body map[string]any, expected string) error {
	if body["url"] != expected {
		return NewParseError("workflow.url")
	}
	return nil
}
func resourceHasDoneMarker(output any) bool {
	raw, ok := output.(map[string]any)
	if !ok {
		return false
	}
	screens, ok := raw["screens"].([]any)
	if !ok {
		return false
	}
	for _, screen := range screens {
		s, ok := screen.(map[string]any)
		if !ok {
			continue
		}
		header, ok := s["header"].([]any)
		if !ok {
			continue
		}
		for _, item := range header {
			i, ok := item.(map[string]any)
			if !ok {
				continue
			}
			p, ok := i["properties"].(map[string]any)
			if ok && p["level"] == "done" {
				return true
			}
		}
	}
	return false
}

// Confirm holds the requester's serialized sequence for exactly the captured
// EXTERNAL_ENTER / EXTERNAL_RETURN / final info transition. The callback-local
// sender is never retained or exposed. Unknown branches (including OTP), URLs,
// missing document IDs and missing done markers fail closed and cannot repeat.
func (a *TransfersAPI) Confirm(ctx context.Context, transfer PreparedTransfer) (TransferResult, error) {
	if !a.options.AllowMutations {
		return TransferResult{}, ErrResourceMutationsDisabled
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if transfer.Flow() != "me2meCreate" || transfer.State() != "summary" {
		return TransferResult{}, NewParseError("transfer.state")
	}
	pid, err := resourceWorkflowPID(transfer.PID())
	if err != nil {
		return TransferResult{}, err
	}
	if prepared, ok := a.prepared[pid]; !ok || prepared != transfer {
		return TransferResult{}, NewParseError("transfer.origin")
	}
	if a.confirmationAttempts[pid] {
		return TransferResult{}, NewParseError("transfer.already_confirmed")
	}
	if err = ctx.Err(); err != nil {
		return TransferResult{}, err
	}
	pageID, err := resourceTransferPageID(pid)
	if err != nil {
		return TransferResult{}, err
	}
	if a.confirmationAttempts == nil {
		a.confirmationAttempts = map[string]bool{}
	}
	a.confirmationAttempts[pid] = true
	var completed map[string]any
	err = a.requester.MutationSequence(ctx, func(send func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		enter, err := send(ctx, Me2MeWorkflowPath, map[string]any{"document": map[string]any{"flow": transfer.Flow(), "state": transfer.State()}}, map[string]string{"cmd": "EVENT", "name": "summaryNext", "pid": pid}, pageID, true)
		if err != nil {
			return err
		}
		enterBody, err := resourceWorkflowBody(enter, "EXTERNAL_ENTER", pid, "", "")
		if err != nil {
			return err
		}
		if err = resourceWorkflowURL(enterBody, ConfirmationWorkflowPath); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		returned, err := send(ctx, ConfirmationWorkflowPath, map[string]any{}, map[string]string{"cmd": "EVENT", "name": "on-enter", "pid": pid}, pageID, true)
		if err != nil {
			return err
		}
		returnedBody, err := resourceWorkflowBody(returned, "EXTERNAL_RETURN", pid, "", "")
		if err != nil {
			return err
		}
		if err = resourceWorkflowURL(returnedBody, Me2MeWorkflowPath); err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		payload, err := send(ctx, Me2MeWorkflowPath, map[string]any{}, map[string]string{"cmd": "EVENT", "name": "on-return", "pid": pid}, pageID, true)
		if err != nil {
			return err
		}
		completed, err = resourceWorkflowBody(payload, "SUCCESS", pid, "me2meInfo", "showInfo")
		return err
	})
	if err != nil {
		return TransferResult{}, errors.Join(&sdkErrs.MutationUncertain{Message: "transfer confirmation result unknown; do not repeat"}, clientSafeError(err))
	}
	output, ok := completed["output"].(map[string]any)
	if !ok {
		return TransferResult{}, &sdkErrs.MutationUncertain{Message: "transfer confirmation lacks final output"}
	}
	document, ok := output["document"].(map[string]any)
	if !ok {
		return TransferResult{}, &sdkErrs.MutationUncertain{Message: "transfer confirmation lacks document"}
	}
	id, ok := document["srcDocumentId"].(string)
	if !ok || id == "" || utf8.RuneCountInString(id) > 128 || resourceValidateText(id, "document_id", true) != nil {
		return TransferResult{}, &sdkErrs.MutationUncertain{Message: "transfer confirmation lacks valid document ID"}
	}
	if !resourceHasDoneMarker(output) {
		return TransferResult{}, &sdkErrs.MutationUncertain{Message: "transfer confirmation lacks done marker"}
	}
	return NewTransferResult(pid, "me2meInfo", "showInfo", &id), nil
}
