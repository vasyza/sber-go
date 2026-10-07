package bank

import (
	"context"
)

// Resources binds all APIs to one requester and one workflow lifetime. Concrete
// clients can retain this bundle and forward Portfolio; no concrete client
// type, transport or authentication implementation is required here.
type Resources struct {
	Products   *ProductsAPI
	Accounts   *AccountsAPI
	Operations *OperationsAPI
	Cards      *CardsAPI
	Transfers  *TransfersAPI
	Analytics  *AnalyticsAPI
	Session    *SessionAPI
	requester  BusinessRequester
	binding    *entityBinding
}

func NewResources(requester BusinessRequester, options ...ResourceOptions) *Resources {
	o := resourceOptions(options)
	a := &Resources{Products: NewProductsAPI(requester), Operations: NewOperationsAPI(requester, o), Cards: NewCardsAPI(requester, o), Transfers: NewTransfersAPI(requester, o), Analytics: NewAnalyticsAPI(requester), Session: NewSessionAPI(requester), requester: requester}
	a.binding = &entityBinding{requester, o, a.Operations, a.Cards, a.Transfers}
	a.Cards.binding = a.binding
	a.Accounts = &AccountsAPI{requester, o, a.binding}
	return a
}

// Portfolio performs exactly one product read and resolves relationships in
// that response. Future reads produce new snapshots, sharing workflow guards.
func (a *Resources) Portfolio(ctx context.Context, forceUpdate bool) (*BankPortfolio, error) {
	return resourceFetchPortfolio(ctx, a.requester, a.binding, forceUpdate)
}
