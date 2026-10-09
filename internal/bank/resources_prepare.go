package bank

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	sdkErrs "github.com/vasyza/sber-sdk/internal/errs"
)

type TransferOptions struct{ Currency, PaymentPurpose string }

func DefaultTransferOptions() TransferOptions { return TransferOptions{Currency: "RUB"} }

var resourceCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
var resourceDecimalParts = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]+))?(?:E([+-]?[0-9]+))?$`)

// Use only public exact Decimal methods, not float conversion or private model
// storage. Precision checks precede fixed-format allocation. Trailing zeros
// are stripped only when the original scale exceeds two, as in the source.
func resourceTransferAmount(amount Decimal) (string, error) {
	if amount.Sign() <= 0 {
		return "", NewParseError("amount")
	}
	m := resourceDecimalParts.FindStringSubmatch(amount.String())
	if m == nil {
		return "", NewParseError("amount")
	}
	exponent := 0
	var err error
	if m[3] != "" {
		exponent, err = strconv.Atoi(m[3])
		if err != nil {
			return "", NewParseError("amount")
		}
	}
	if exponent < -int(^uint(0)>>1)+len(m[2]) {
		return "", NewParseError("amount")
	}
	exponent -= len(m[2])
	digits := strings.TrimLeft(m[1]+m[2], "0")
	if len(digits) < 1 || len(digits) > 18 || exponent > 15-len(digits) {
		return "", NewParseError("amount")
	}
	effective := exponent
	trim := 0
	for i := len(digits) - 1; i >= 0 && effective < 0 && digits[i] == '0'; i-- {
		effective++
		trim++
	}
	if effective < -2 {
		return "", NewParseError("amount")
	}
	if exponent < -2 {
		digits = digits[:len(digits)-trim]
		exponent = effective
	}
	point := len(digits) + exponent
	if point <= 0 {
		return "0." + strings.Repeat("0", -point) + digits, nil
	}
	if point < len(digits) {
		return digits[:point] + "." + digits[point:], nil
	}
	return digits + strings.Repeat("0", point-len(digits)), nil
}
func resourceSelectedResource(id string, resources []TransferResource, field string) (TransferResource, error) {
	if !resourceTransferGrammar.MatchString(id) {
		return TransferResource{}, NewParseError(field)
	}
	count := 0
	var result TransferResource
	for _, r := range resources {
		if r.ID() == id {
			count++
			result = r
		}
	}
	if count != 1 {
		return TransferResource{}, NewParseError(field)
	}
	return result, nil
}
func resourceTransferPageID(pid string) (string, error) {
	if _, err := resourceWorkflowPID(pid); err != nil {
		return "", err
	}
	return "/app/payments/self/workflow?action=CREATE&pid=" + pid, nil
}

// Prepare never moves money by itself. The draft must be issued by this API,
// may be prepared at most once, and preserves the original exact amount.
// Any attempted preparation consumes the draft, including uncertain/canceled
// requests, so an old prepared audit record cannot be overwritten server-side.
func (a *TransfersAPI) Prepare(ctx context.Context, draft TransferDraft, sourceID, destinationID string, amount Decimal, options ...TransferOptions) (PreparedTransfer, error) {
	if !a.options.AllowMutations {
		return PreparedTransfer{}, ErrResourceMutationsDisabled
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if draft.Flow() != "me2meCreate" || draft.State() != "transferRequisites" {
		return PreparedTransfer{}, NewParseError("draft.state")
	}
	pid, err := resourceWorkflowPID(draft.PID())
	if err != nil {
		return PreparedTransfer{}, err
	}
	if issued, ok := a.drafts[pid]; !ok || issued != draft {
		return PreparedTransfer{}, NewParseError("draft.origin")
	}
	if a.prepareAttempts[pid] {
		return PreparedTransfer{}, NewParseError("draft.already_prepared")
	}
	source, err := resourceSelectedResource(sourceID, draft.Sources(), "source_id")
	if err != nil {
		return PreparedTransfer{}, err
	}
	destination, err := resourceSelectedResource(destinationID, draft.Destinations(), "destination_id")
	if err != nil {
		return PreparedTransfer{}, err
	}
	if source.ID() == destination.ID() {
		return PreparedTransfer{}, NewParseError("destination_id")
	}
	amountText, err := resourceTransferAmount(amount)
	if err != nil {
		return PreparedTransfer{}, err
	}
	o := DefaultTransferOptions()
	if len(options) > 1 {
		return PreparedTransfer{}, NewParseError("options")
	}
	if len(options) == 1 {
		o = options[0]
	}
	if !resourceCurrency.MatchString(o.Currency) {
		return PreparedTransfer{}, NewParseError("currency")
	}
	for _, r := range []TransferResource{source, destination} {
		if r.Currency() != "" && r.Currency() != o.Currency {
			return PreparedTransfer{}, NewParseError("currency")
		}
	}
	if err = resourceValidateText(o.PaymentPurpose, "payment_purpose", true); err != nil {
		return PreparedTransfer{}, err
	}
	if utf8.RuneCountInString(o.PaymentPurpose) > 210 {
		return PreparedTransfer{}, NewParseError("payment_purpose")
	}
	pageID, err := resourceTransferPageID(pid)
	if err != nil {
		return PreparedTransfer{}, err
	}
	if err = ctx.Err(); err != nil {
		return PreparedTransfer{}, err
	}
	if a.prepareAttempts == nil {
		a.prepareAttempts = map[string]bool{}
	}
	a.prepareAttempts[pid] = true
	payload, err := a.requester.Mutate(ctx, Me2MeWorkflowPath, map[string]any{"fields": map[string]any{"transfer:me2me:fromResource": source.ID(), "transfer:me2me:toResource": destination.ID(), "transfer:me2me:sum": amountText, "transfer:me2me:sum:currency": o.Currency, "transfer:me2me:paymentPurpose": o.PaymentPurpose}, "document": map[string]any{"flow": draft.Flow(), "state": draft.State()}}, map[string]string{"cmd": "EVENT", "name": "next", "pid": pid}, pageID, true)
	if err != nil {
		var rejected *sdkErrs.APIRejected
		var uncertain *sdkErrs.MutationUncertain
		if errors.As(err, &rejected) && !errors.As(err, &uncertain) {
			return PreparedTransfer{}, err
		}
		return PreparedTransfer{}, errors.Join(&sdkErrs.MutationUncertain{Message: "transfer preparation result unknown; do not repeat"}, clientSafeError(err))
	}
	if _, err = resourceWorkflowBody(payload, "SUCCESS", pid, "me2meCreate", "summary"); err != nil {
		return PreparedTransfer{}, &sdkErrs.MutationUncertain{Message: "transfer preparation response invalid; do not repeat"}
	}
	prepared := NewPreparedTransfer(pid, "me2meCreate", "summary", source.ID(), destination.ID(), Money{Amount: amount, Currency: o.Currency}, o.PaymentPurpose)
	if a.prepared == nil {
		a.prepared = map[string]PreparedTransfer{}
	}
	a.prepared[pid] = prepared
	return prepared, nil
}
