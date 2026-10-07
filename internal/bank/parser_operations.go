// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"sort"
	"strings"
)

// ParseOperations retains the server's N+1 sentinel. A missing/malformed
// body.operations is an error (UNKNOWN downstream), never successful zero history.
// Scopes supply card context only; they never alter the transaction amount.
func ParseOperations(payload map[string]any, scopes ...string) ([]Operation, error) {
	if err := domainEnvelope(payload); err != nil {
		return nil, err
	}
	body, err := domainMapping(payload["body"], "body")
	if err != nil {
		return nil, err
	}
	raw, ok := body["operations"].([]any)
	if !ok {
		return nil, NewParseError("body.operations")
	}
	cards := map[string]bool{}
	for _, scope := range scopes {
		r := domainResource(map[string]any{"id": scope})
		if r != nil && r.Type == "card" {
			cards[r.ID] = true
		}
	}
	scopeIDs := make([]string, 0, len(cards))
	for id := range cards {
		scopeIDs = append(scopeIDs, id)
	}
	sort.Strings(scopeIDs)
	result := make([]Operation, 0, len(raw))
	for _, item := range raw {
		v, err := domainMapping(item, "operation")
		if err != nil {
			return nil, err
		}
		amount, err := domainCoreMoney(v["operationAmount"], "operationAmount")
		if err != nil {
			return nil, err
		}
		state := domainOptionalMapping(v["state"])
		billing := domainOptionalMapping(v["billingAmount"])
		op := Operation{ID: domainID(v["uohId"]), Date: domainOperationDate(v["date"]), Form: domainText(v["form"]), Type: domainText(v["type"]), ClassificationCode: domainID(v["classificationCode"]), CreationChannel: domainText(v["creationChannel"]), State: domainText(state["category"]), StateName: domainText(state["name"]), StateDescription: domainText(state["description"]), Merchant: strings.TrimSpace(domainText(v["correspondent"])), Description: domainText(v["description"]), Amount: amount, BillingAmount: domainSoftMoney(v["billingAmount"]), NationalAmount: domainSoftMoney(v["nationalAmount"]), Commission: domainSoftMoney(v["commission"]), Tips: domainSoftMoney(v["tips"]), RefusalReason: domainText(v["refusalReason"]), FromResource: domainResource(v["fromResource"]), ToResource: domainResource(v["toResource"]), ScopeCardIDs: append([]string{}, scopeIDs...), IsHidden: domainTruthy(v["isHidden"])}
		if b, ok := v["isFinancial"].(bool); ok {
			op.IsFinancial = &b
		}
		_, hasID := billing["id"]
		_, hasName := billing["name"]
		if hasID || hasName {
			op.BalanceAfter = domainMoneyCopy(op.BillingAmount)
			if id := domainID(billing["id"]); id != "" {
				op.BalanceAfterResourceID = domainStringPointer(id)
			}
			if name := domainText(billing["name"]); name != "" {
				op.BalanceAfterResourceName = domainStringPointer(name)
			}
		}
		result = append(result, op)
	}
	return result, nil
}

func domainMoneyCopy(m *Money) *Money {
	if m == nil {
		return nil
	}
	n := *m
	return &n
}

// ParseOperationDetails parses the pure details response. Optional monetary
// detail fields tolerate malformed values; required body data is fail-closed.
func ParseOperationDetails(payload map[string]any) (OperationDetail, error) {
	if err := domainEnvelope(payload); err != nil {
		return OperationDetail{}, err
	}
	body, err := domainMapping(payload["body"], "body")
	if err != nil {
		return OperationDetail{}, err
	}
	header, state := domainOptionalMapping(body["header"]), domainOptionalMapping(body["state"])
	fields := []OperationDetailField{}
	items, _ := body["fields"].([]any)
	for _, item := range items {
		raw := domainOptionalMapping(item)
		var value any
		if m := domainSoftMoney(raw["value"]); m != nil {
			value = m
		} else if raw["value"] != nil {
			if _, ok := raw["value"].(map[string]any); !ok {
				value = domainText(raw["value"])
			}
		}
		fields = append(fields, OperationDetailField{Name: domainText(raw["name"]), Type: domainText(raw["type"]), Value: value})
	}
	return OperationDetail{UOHID: domainID(body["uohId"]), Form: domainText(body["form"]), Title: domainText(header["title"]), Amount: domainSoftMoney(header["operationAmount"]), State: domainText(state["category"]), StateName: domainText(state["name"]), StateDescription: domainText(state["description"]), StatementAvailable: domainTruthy(body["statementAvailable"]), Fields: fields}, nil
}

// BuildCardLedger deduplicates by (operation, card, direction), in insertion
// order. Direct transfers produce a debit and a credit; additional scopes use
// the transaction's original sign, never billingAmount/account balances.
func BuildCardLedger(operations []Operation) []CardLedgerEntry {
	type key struct{ operation, card, direction string }
	seen := map[key]int{}
	entries := []CardLedgerEntry{}
	add := func(e CardLedgerEntry) {
		k := key{e.OperationID, e.CardID, e.Direction}
		if i, ok := seen[k]; ok {
			entries[i] = e
		} else {
			seen[k] = len(entries)
			entries = append(entries, e)
		}
	}
	for _, op := range operations {
		direct := map[string]bool{}
		for _, edge := range []struct {
			resource  *Resource
			direction string
		}{{op.FromResource, "out"}, {op.ToResource, "in"}} {
			r := edge.resource
			if r == nil || r.Type != "card" {
				continue
			}
			direct[r.ID] = true
			m := domainMoneyCopy(op.Amount)
			if m != nil {
				m.Amount = m.Amount.Abs()
				if edge.direction == "out" && m.Amount.Sign() != 0 {
					m.Amount = m.Amount.Neg()
				}
			}
			add(CardLedgerEntry{OperationID: op.ID, CardID: r.ID, Direction: edge.direction, Amount: m})
		}
		scoped := map[string]bool{}
		for _, id := range op.ScopeCardIDs {
			if !direct[id] {
				scoped[id] = true
			}
		}
		ids := make([]string, 0, len(scoped))
		for id := range scoped {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			direction := "unknown"
			if op.Amount != nil {
				switch op.Amount.Amount.Sign() {
				case -1:
					direction = "out"
				case 1:
					direction = "in"
				default:
					direction = "neutral"
				}
			}
			add(CardLedgerEntry{OperationID: op.ID, CardID: id, Direction: direction, Amount: domainMoneyCopy(op.Amount)})
		}
	}
	return entries
}
