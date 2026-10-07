package cli

import (
	"context"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	sber "github.com/vasyza/sber-go"
	"github.com/vasyza/sber-go/internal/ownerinput"
)

var mutationCardName = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё0-9 ,.\-]{1,56}$`)
var mutationResource = regexp.MustCompile(`^(transactionAccount|card|account):[A-Za-z0-9_-]{1,128}$`)
var mutationAmount = regexp.MustCompile(`^[0-9]{1,15}(\.[0-9]{1,2})?$`)
var mutationCurrency = regexp.MustCompile(`^[A-Z]{3}$`)

func isMutationCommand(name string) bool { return name == "card-rename" || name == "transfer-own" }

func validateMutation(name string, a *commandArguments) bool {
	if name == "card-rename" {
		return len(a.cards) == 1 && validateCommand("card-limits", a) && strings.TrimSpace(a.name) != "" && mutationCardName.MatchString(a.name)
	}
	if !mutationResource.MatchString(a.source) || !mutationResource.MatchString(a.destination) || a.source == a.destination || !mutationAmount.MatchString(a.amount) || !mutationCurrency.MatchString(a.currency) || !utf8.ValidString(a.purpose) || utf8.RuneCountInString(a.purpose) > 210 {
		return false
	}
	for _, r := range a.purpose {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	amount, err := sber.ParseDecimal(a.amount)
	return err == nil && amount.Sign() > 0
}

func mutationPlan(command string, a *commandArguments) map[string]any {
	plan := map[string]any{"command": command, "executed": false, "bank_request_sent": false, "bank_validation_checked": false}
	if command == "card-rename" {
		plan["card_id"], plan["name"] = a.cards[0], sber.RedactPAN(a.name)
	} else {
		plan["source"], plan["destination"], plan["amount"], plan["currency"], plan["purpose"] = a.source, a.destination, a.amount, a.currency, sber.RedactPAN(a.purpose)
	}
	return plan
}

func confirmAction(ctx context.Context, diagnostics io.Writer, dependencies *Authentication, plan map[string]any) int {
	if diagnostics == nil {
		return 3
	}
	if code := writeResult(diagnostics, diagnostics, plan); code != 0 {
		return code
	}
	read := ownerSecret
	if dependencies != nil && dependencies.ReadSecret != nil {
		read = dependencies.ReadSecret
	}
	response, err := read(ctx, ownerinput.ConfirmAction)
	confirmed := err == nil && response == "CONFIRM"
	response = ""
	if !confirmed {
		if ctx.Err() != nil {
			return 130
		}
		return fail(diagnostics, 3, "You did not confirm the operation.\nThe command sent no further request.")
	}
	return 0
}
