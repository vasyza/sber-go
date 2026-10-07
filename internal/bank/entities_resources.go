package bank

// BankProduct is the closed set of SDK-bound card/account snapshots. The
// abstract source product properties become error-returning native methods.
type BankProduct interface {
	OperationResource() (string, error)
	TransferResource() (string, error)
	bankBinding() *entityBinding
}

func (a *BankAccount) bankBinding() *entityBinding {
	if a == nil || a.data == nil {
		return nil
	}
	return (*a.data).binding
}
func (c *BankCard) bankBinding() *entityBinding {
	if c == nil || c.data == nil {
		return nil
	}
	return (*c.data).binding
}
func (a *BankAccount) OperationResource() (string, error) {
	if !resourceOpaqueID.MatchString(a.ID()) {
		return "", NewParseError("account.id")
	}
	switch a.Kind() {
	case "ctaccount":
		return "ct-account:" + a.ID(), nil
	case "account":
		return "account:" + a.ID(), nil
	default:
		return "", NewParseError("account.kind")
	}
}
func (a *BankAccount) TransferResource() (string, error) {
	if !resourceOpaqueID.MatchString(a.ID()) {
		return "", NewParseError("account.id")
	}
	switch a.Kind() {
	case "ctaccount":
		return "transactionAccount:" + a.ID(), nil
	case "account":
		return "account:" + a.ID(), nil
	default:
		return "", NewParseError("account.kind")
	}
}
func (c *BankCard) OperationResource() (string, error) {
	if !resourceOpaqueID.MatchString(c.ID()) {
		return "", NewParseError("card.id")
	}
	return "card:" + c.ID(), nil
}
func (c *BankCard) TransferResource() (string, error) { return c.OperationResource() }
