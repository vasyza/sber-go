package sber

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// ExportJSON is an explicit, complete financial snapshot export, not display
// redaction. Parsed bank labels are already PAN-safe. Preserve literal IDs and
// exact Money strings, and reject invalid UTF-8 instead of repairing it. The
// generic exporter admits these owned snapshots through a typed boundary;
// foreign custom marshalers still control their own redaction and wire shape.
func (p *BankPortfolio) ExportJSON() ([]byte, error) { return entityExportFinancial(p.Raw()) }
func (a *BankAccount) ExportJSON() ([]byte, error)   { return entityExportFinancial(a.Snapshot()) }
func (c *BankCard) ExportJSON() ([]byte, error)      { return entityExportFinancial(c.Snapshot()) }

// Only these exact library-owned entities may delegate to a native snapshot.
// Do not use an open interface: an embedded entity could promote that method
// and bypass a foreign wrapper's credential redactor. The copy never includes
// requester/binding/portfolio implementation fields or their cyclic graph.
func domainJSONEntitySnapshot(value any) (any, bool) {
	switch v := value.(type) {
	case BankPortfolio:
		return (&v).Raw(), true
	case *BankPortfolio:
		return v.Raw(), true
	case BankAccount:
		return (&v).Snapshot(), true
	case *BankAccount:
		return v.Snapshot(), true
	case BankCard:
		return (&v).Snapshot(), true
	case *BankCard:
		return v.Snapshot(), true
	}
	return nil, false
}

// Validate original snapshot text before either encoder can repair native
// UTF-8. Only the owned public snapshot layouts are inspected, never binding
// or requester implementation fields.
func entityValidateFinancial(value any) error {
	validStrings := func(values ...string) bool {
		for _, v := range values {
			if !utf8.ValidString(v) {
				return false
			}
		}
		return true
	}
	moneyValid := func(m *Money) bool { return m == nil || utf8.ValidString(m.Currency) }
	pointerText := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	validAccount := func(a Account) bool {
		return validStrings(a.ID, a.Name, a.Last4, a.State, a.Kind) && moneyValid(a.Balance)
	}
	validCard := func(c Card) bool {
		return validStrings(c.ID, c.Name, c.Last4, c.Type, c.State, pointerText(c.BalanceSource), pointerText(c.AccountID)) && moneyValid(c.Balance) && moneyValid(c.AccountBalance)
	}
	valid := true
	switch v := value.(type) {
	case Account:
		valid = validAccount(v)
	case Card:
		valid = validCard(v)
	case Products:
		for _, a := range v.Accounts {
			valid = valid && validAccount(a)
		}
		for _, c := range v.Cards {
			valid = valid && validCard(c)
		}
	default:
		valid = false
	}
	if !valid {
		return NewParseError("financial_snapshot")
	}
	return nil
}

// Ordinary compatibility JSON is compact and raw, unlike generic display
// normalization. Keep the ordinary encoder's shape and HTML-escape defaults.
func entityMarshalFinancial(value any) ([]byte, error) {
	if err := entityValidateFinancial(value); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil || strictjson.Validate(data) != nil {
		return nil, NewParseError("financial_snapshot")
	}
	return data, nil
}

func entityExportFinancial(value any) ([]byte, error) {
	if err := entityValidateFinancial(value); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, NewParseError("financial_snapshot")
	}
	if strictjson.Validate(buf.Bytes()) != nil {
		return nil, NewParseError("financial_snapshot")
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
