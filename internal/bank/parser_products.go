// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import ()

// ParseProducts parses pure products JSON. Ambiguous current-account numbers
// never link a card; plain savings accounts do not become current parents.
func ParseProducts(payload map[string]any) (Products, error) {
	if err := domainEnvelope(payload); err != nil {
		return Products{}, err
	}
	data := payload
	for _, key := range []string{"body", "sections", "technicalSection", "sectionProductData"} {
		data = domainOptionalMapping(data[key])
	}
	result := Products{Accounts: []Account{}, Cards: []Card{}}
	type parent struct {
		id      string
		balance *Money
	}
	parents := map[string][]parent{}
	for _, section := range []struct{ key, kind string }{{"ctaccounts", "ctaccount"}, {"sharingCtAccounts", "ctaccount"}, {"accounts", "account"}} {
		items, err := domainDataList(data[section.key], section.key)
		if err != nil {
			return Products{}, err
		}
		for _, item := range items {
			raw, err := domainMapping(item, section.key)
			if err != nil {
				return Products{}, err
			}
			balance, err := domainCoreMoney(raw["balance"], "balance")
			if err != nil {
				return Products{}, err
			}
			id, number := domainID(raw["id"]), domainID(raw["number"])
			result.Accounts = append(result.Accounts, Account{ID: id, Name: domainText(raw["name"]), Last4: domainLast4(number), State: domainText(raw["state"]), Hidden: domainTruthy(raw["hidden"]), Arrested: domainTruthy(raw["arrested"]), Balance: balance, Kind: section.kind})
			if section.kind == "ctaccount" && number != "" {
				parents[number] = append(parents[number], parent{id, balance})
			}
		}
	}
	items, err := domainDataList(data["cardsInWallet"], "cardsInWallet")
	if err != nil {
		return Products{}, err
	}
	for _, item := range items {
		raw, err := domainMapping(item, "card")
		if err != nil {
			return Products{}, err
		}
		key := "availableLimit"
		var p *parent
		candidates := parents[domainID(raw["cardAccount"])]
		if domainTruthy(raw["isCTA"]) && len(candidates) == 1 {
			p = &candidates[0]
			key = "availableTotalLimit"
		}
		balance, err := domainCoreMoney(raw[key], key)
		if err != nil {
			return Products{}, err
		}
		c := Card{ID: domainID(raw["id"]), Name: domainText(raw["name"]), Last4: domainLast4(raw["number"]), Type: domainText(raw["type"]), State: domainText(raw["state"]), Hidden: domainTruthy(raw["hidden"]), Arrested: domainTruthy(raw["arrested"]), IsMain: domainTruthy(raw["isMain"]), Balance: balance}
		if balance != nil {
			c.BalanceSource = domainStringPointer(key)
		}
		if p != nil {
			c.AccountID = domainStringPointer(p.id)
			c.AccountBalance = domainMoneyCopy(p.balance)
		}
		result.Cards = append(result.Cards, c)
	}
	return result, nil
}
