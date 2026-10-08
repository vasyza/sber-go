// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

func domainCreditInfo(value any) *CreditInfo {
	raw := domainOptionalMapping(value)
	if len(raw) == 0 {
		return nil
	}
	return &CreditInfo{Limit: domainSoftMoney(raw["creditLimit"]), OwnSum: domainSoftMoney(raw["creditOwnSum"]), Debt: domainSoftMoney(raw["creditDebt"]), MinPayment: domainSoftMoney(raw["creditMinPayment"]), MinPaymentDate: domainText(raw["creditMinPaymentDate"])}
}

// ParseCardInfo never retains a full PAN; all display text is Luhn-redacted.
func ParseCardInfo(payload map[string]any) ([]CardInfo, error) {
	if err := domainEnvelope(payload); err != nil {
		return nil, err
	}
	body, err := domainMapping(payload["body"], "body")
	if err != nil {
		return nil, err
	}
	raw, _ := domainOptionalMapping(body["cardDetails"])["cards"].([]any)
	cards := []CardInfo{}
	for _, item := range raw {
		card, err := domainMapping(item, "card")
		if err != nil {
			return nil, err
		}
		limits := domainOptionalMapping(card["limits"])
		cards = append(cards, CardInfo{ID: domainID(card["id"]), Name: domainText(card["name"]), Last4: domainLast4(card["number"]), State: domainText(card["state"]), CardHolder: domainText(card["cardHolder"]), PaySystemType: domainText(card["paySystemType"]), ExpireDate: domainText(card["expireDate"]), Limits: CardLimits{Purchase: domainSoftMoney(limits["purchaseLimit"]), Available: domainSoftMoney(limits["availableLimit"]), AvailableTotal: domainSoftMoney(limits["availableTotalLimit"])}, Credit: domainCreditInfo(card["creditType"])})
	}
	return cards, nil
}
