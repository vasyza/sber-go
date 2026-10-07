// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"encoding/json"
	"strconv"
	"strings"
)

func domainInt(value any) int {
	switch v := value.(type) {
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		text := strings.Map(func(r rune) rune {
			if r == '_' {
				return -1
			}
			if n, ok := domainDigitValue(r); ok {
				return rune('0' + n)
			}
			return r
		}, strings.TrimSpace(v))
		n, err := strconv.Atoi(text)
		if err == nil {
			return n
		}
		return 0
	case json.Number:
		if !json.Valid([]byte(v)) {
			return 0
		}
	}
	d, err := ParseDecimal(value)
	if err != nil {
		return 0
	}
	// Truncate before allocating powers of ten. An adversarial JSON exponent
	// cannot make optional analytics counts allocate an enormous big.Int.
	point := len(d.digits) + d.exponent
	if point <= 0 || point > 19 {
		return 0
	}
	digits := d.digits
	if point < len(digits) {
		digits = digits[:point]
	} else {
		digits += strings.Repeat("0", point-len(digits))
	}
	if d.negative {
		digits = "-" + digits
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0
	}
	return n
}

// ParsePFMAmounts parses optional analytics periods and categories offline.
func ParsePFMAmounts(payload map[string]any) (PFMAmounts, error) {
	if err := domainEnvelope(payload); err != nil {
		return PFMAmounts{}, err
	}
	body, err := domainMapping(payload["body"], "body")
	if err != nil {
		return PFMAmounts{}, err
	}
	raw, _ := body["amounts"].([]any)
	periods := []PFMPeriod{}
	for _, item := range raw {
		period, err := domainMapping(item, "amounts[]")
		if err != nil {
			return PFMAmounts{}, err
		}
		categories := []CategoryAmount{}
		xs, _ := period["categoryAmounts"].([]any)
		for _, item := range xs {
			cat, err := domainMapping(item, "categoryAmounts[]")
			if err != nil {
				return PFMAmounts{}, err
			}
			categories = append(categories, CategoryAmount{ID: domainID(cat["id"]), Name: domainText(cat["name"]), ExternalID: domainText(cat["externalId"]), NationalAmount: domainSoftMoney(cat["nationalAmount"]), VisibleAmount: domainSoftMoney(cat["visibleAmount"]), CountOperations: domainInt(cat["countOperations"])})
		}
		periods = append(periods, PFMPeriod{From: domainText(period["from"]), To: domainText(period["to"]), IncomeType: domainText(period["incomeType"]), NationalAmount: domainSoftMoney(period["nationalAmount"]), VisibleAmount: domainSoftMoney(period["visibleAmount"]), Categories: categories})
	}
	return PFMAmounts{Periods: periods}, nil
}
