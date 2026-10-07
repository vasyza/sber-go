// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"encoding/json"
	"fmt"
	"slices"
)

// Transfer models preserve the typed workflow contract without implementing
// transport/mutations. Process/resource/document identifiers are hidden by
// default fmt and JSON, unlike intentionally exportable financial snapshots.
type TransferResource struct{ data **transferResourceData }

type transferResourceData struct{ id, kind, name, currency string }

// NewTransferResource retains all raw resource fields without validation or
// normalization. Copies share immutable identity; accessors expose raw reads.
func NewTransferResource(id, kind, name, currency string) TransferResource {
	if id == "" && kind == "" && name == "" && currency == "" {
		return TransferResource{}
	}
	// A terminal pointer pointee prevents nested fmt badVerb from dereferencing
	// the raw record, including method-inaccessible private wrapper fields.
	data := &transferResourceData{id, kind, name, currency}
	return TransferResource{data: &data}
}

// ID deliberately reads the raw resource identifier.
func (v TransferResource) ID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).id
}

// Kind reads the supplied resource kind.
func (v TransferResource) Kind() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).kind
}

// Name deliberately reads the complete name, without display PAN masking.
func (v TransferResource) Name() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).name
}

// Currency reads the supplied currency without inventing a default.
func (v TransferResource) Currency() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).currency
}

// TransferDraft is immutable. Lists are copied at both construction and read.
type TransferDraft struct{ data **transferDraftData }

type transferDraftData struct {
	pid, flow, state      string
	sources, destinations []TransferResource
}

// NewTransferDraft preserves every workflow field, resource and list ordering.
// Nil lists stay nil; explicit-empty lists stay nonnil. No defaults are invented.
func NewTransferDraft(pid, flow, state string, sources, destinations []TransferResource) TransferDraft {
	if pid == "" && flow == "" && state == "" && sources == nil && destinations == nil {
		return TransferDraft{}
	}
	data := &transferDraftData{pid, flow, state, slices.Clone(sources), slices.Clone(destinations)}
	return TransferDraft{data: &data}
}

// PID deliberately reads the raw transfer process identifier.
func (v TransferDraft) PID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).pid
}

// Flow reads the supplied workflow.
func (v TransferDraft) Flow() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).flow
}

// State reads the supplied state.
func (v TransferDraft) State() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).state
}

// Sources returns a defensive list copy of immutable resource snapshots.
func (v TransferDraft) Sources() []TransferResource {
	if v.data == nil {
		return nil
	}
	return slices.Clone((*v.data).sources)
}

// Destinations returns a defensive list copy of immutable resource snapshots.
func (v TransferDraft) Destinations() []TransferResource {
	if v.data == nil {
		return nil
	}
	return slices.Clone((*v.data).destinations)
}

// PreparedTransfer keeps its exact amount and raw wire fields immutable.
type PreparedTransfer struct{ data **preparedTransferData }

type preparedTransferData struct {
	pid, flow, state, sourceID, destinationID string
	amount                                    Money
	paymentPurpose                            string
}

// NewPreparedTransfer retains every supplied field without validation or
// financial mutation. Money is copied by value; its Decimal is immutable.
func NewPreparedTransfer(pid, flow, state, sourceID, destinationID string, amount Money, paymentPurpose string) PreparedTransfer {
	if pid == "" && flow == "" && state == "" && sourceID == "" && destinationID == "" && amount == (Money{}) && paymentPurpose == "" {
		return PreparedTransfer{}
	}
	data := &preparedTransferData{pid, flow, state, sourceID, destinationID, amount, paymentPurpose}
	return PreparedTransfer{data: &data}
}

// PID deliberately reads the raw transfer process identifier.
func (v PreparedTransfer) PID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).pid
}

// Flow reads the supplied workflow.
func (v PreparedTransfer) Flow() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).flow
}

// State reads the supplied state.
func (v PreparedTransfer) State() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).state
}

// SourceID deliberately reads the raw source resource identifier.
func (v PreparedTransfer) SourceID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).sourceID
}

// DestinationID deliberately reads the raw destination resource identifier.
func (v PreparedTransfer) DestinationID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).destinationID
}

// Amount returns a value copy preserving the exact Decimal, sign, scale and
// currency. Mutating that Money wrapper cannot change this prepared snapshot.
func (v PreparedTransfer) Amount() Money {
	if v.data == nil {
		return Money{}
	}
	return (*v.data).amount
}

// PaymentPurpose deliberately reads the full, unmasked raw purpose.
func (v PreparedTransfer) PaymentPurpose() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).paymentPurpose
}

// TransferResult is an immutable result, including its optional document ID.
type TransferResult struct{ data **transferResultData }

type transferResultData struct {
	pid, flow, state string
	documentID       *string
}

// NewTransferResult retains all workflow fields and defensively copies the
// optional raw document ID. Nil and a present empty ID remain distinct.
func NewTransferResult(pid, flow, state string, documentID *string) TransferResult {
	if pid == "" && flow == "" && state == "" && documentID == nil {
		return TransferResult{}
	}
	data := &transferResultData{pid, flow, state, copyTransferDocumentID(documentID)}
	return TransferResult{data: &data}
}

func copyTransferDocumentID(documentID *string) *string {
	if documentID == nil {
		return nil
	}
	value := *documentID
	return &value
}

// PID deliberately reads the raw transfer process identifier.
func (v TransferResult) PID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).pid
}

// Flow reads the supplied workflow.
func (v TransferResult) Flow() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).flow
}

// State reads the supplied state.
func (v TransferResult) State() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).state
}

// DocumentID deliberately reads the optional raw ID into a fresh pointer copy.
// Mutating or comparing that pointer cannot mutate/identify the backing ID.
func (v TransferResult) DocumentID() *string {
	if v.data == nil {
		return nil
	}
	return copyTransferDocumentID((*v.data).documentID)
}

func (v TransferResource) String() string {
	return fmt.Sprintf("TransferResource(id=<redacted>, kind=%q, currency=%q)", RedactPAN(v.Kind()), RedactPAN(v.Currency()))
}

func (v TransferDraft) String() string {
	return fmt.Sprintf("TransferDraft(pid=<redacted>, flow=%q, state=%q, sources=%d, destinations=%d)", RedactPAN(v.Flow()), RedactPAN(v.State()), len(v.Sources()), len(v.Destinations()))
}

func (v PreparedTransfer) String() string {
	return fmt.Sprintf("PreparedTransfer(pid=<redacted>, flow=%q, state=%q, amount=<redacted>, currency=%q)", RedactPAN(v.Flow()), RedactPAN(v.State()), RedactPAN(v.Amount().Currency))
}

func (v TransferResult) String() string {
	present := "None"
	if v.DocumentID() != nil {
		present = "<present>"
	}
	return fmt.Sprintf("TransferResult(pid=<redacted>, flow=%q, state=%q, document_id=%s)", RedactPAN(v.Flow()), RedactPAN(v.State()), present)
}

func (v TransferResource) Format(f fmt.State, verb rune) { domainSafeFormat(f, v.String()) }

func (v TransferDraft) Format(f fmt.State, verb rune) { domainSafeFormat(f, v.String()) }

func (v PreparedTransfer) Format(f fmt.State, verb rune) { domainSafeFormat(f, v.String()) }

func (v TransferResult) Format(f fmt.State, verb rune) { domainSafeFormat(f, v.String()) }

func (v TransferResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"id": "<redacted>", "kind": RedactPAN(v.Kind()), "name": "<redacted>", "currency": RedactPAN(v.Currency())})
}

func (v TransferDraft) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"pid": "<redacted>", "flow": RedactPAN(v.Flow()), "state": RedactPAN(v.State()), "sources": len(v.Sources()), "destinations": len(v.Destinations())})
}

func (v PreparedTransfer) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"pid": "<redacted>", "flow": RedactPAN(v.Flow()), "state": RedactPAN(v.State()), "amount": "<redacted>", "currency": RedactPAN(v.Amount().Currency)})
}

func (v TransferResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"pid": "<redacted>", "flow": RedactPAN(v.Flow()), "state": RedactPAN(v.State()), "document_id_present": v.DocumentID() != nil})
}
