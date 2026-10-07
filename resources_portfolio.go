package sber

import "context"

// AccountsAPI resolves cards and accounts from exactly one product response.
type AccountsAPI struct {
	requester BusinessRequester
	options   ResourceOptions
	binding   *entityBinding
}

func NewAccountsAPI(requester BusinessRequester, options ...ResourceOptions) *AccountsAPI {
	o := resourceOptions(options)
	return &AccountsAPI{requester, o, newEntityBinding(requester, o)}
}
func resourceFetchPortfolio(ctx context.Context, requester BusinessRequester, binding *entityBinding, forceUpdate bool) (*BankPortfolio, error) {
	raw, err := NewProductsAPI(requester).Get(ctx, forceUpdate)
	if err != nil {
		return nil, err
	}
	return newBankPortfolio(raw, binding), nil
}
func (a *CardsAPI) List(ctx context.Context, forceUpdate bool) ([]*BankCard, error) {
	a.bindingMu.Lock()
	if a.binding == nil {
		a.binding = &entityBinding{a.requester, a.options, NewOperationsAPI(a.requester, a.options), a, NewTransfersAPI(a.requester, a.options)}
	}
	binding := a.binding
	a.bindingMu.Unlock()
	p, err := resourceFetchPortfolio(ctx, a.requester, binding, forceUpdate)
	if err != nil {
		return nil, err
	}
	return p.Cards(), nil
}
func (a *AccountsAPI) List(ctx context.Context, forceUpdate bool) ([]*BankAccount, error) {
	p, err := resourceFetchPortfolio(ctx, a.requester, a.binding, forceUpdate)
	if err != nil {
		return nil, err
	}
	return p.Accounts(), nil
}
