package sber

import "context"

// clientResourceRequester is a private delegation boundary, not a new owner.
// A terminal pointer prevents default/unsupported fmt traversal from following
// the public API -> requester -> client -> Resources cycle into session state.
// Every operation is forwarded unchanged to the same concrete client's core.
// There is no independent policy, retry, authentication, transport or close.
// Both pointers are initialized before publication and never changed.
type clientResourceRequester struct{ client **SberClient }

var _ BusinessRequester = (*clientResourceRequester)(nil)

func (r *clientResourceRequester) PostRead(ctx context.Context, path string, payload map[string]any) (map[string]any, error) {
	return (*r.client).PostRead(ctx, path, payload)
}
func (r *clientResourceRequester) Mutate(ctx context.Context, path string, payload map[string]any, query map[string]string, pageID string, workflow bool) (map[string]any, error) {
	return (*r.client).Mutate(ctx, path, payload, query, pageID, workflow)
}
func (r *clientResourceRequester) MutationSequence(ctx context.Context, callback func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error {
	return (*r.client).MutationSequence(ctx, callback)
}
func (r *clientResourceRequester) ExportSession() (SessionBundle, error) {
	return (*r.client).ExportSession()
}
func (r *clientResourceRequester) ExportCredentials() (SberCredentials, error) {
	return (*r.client).ExportCredentials()
}
func (r *clientResourceRequester) WarmUp(ctx context.Context, force bool) error {
	return (*r.client).WarmUp(ctx, force)
}

// Products returns the client's stable native products API. Resource APIs share
// this client's transport/renewal/close ownership; getters do no I/O.
func (c *SberClient) Products() *ProductsAPI { return (*c.core().resources).Products }

// Operations returns the stable native API bound to this client.
func (c *SberClient) Operations() *OperationsAPI { return (*c.core().resources).Operations }

// Accounts returns the stable native API bound to this client.
func (c *SberClient) Accounts() *AccountsAPI { return (*c.core().resources).Accounts }

// Cards returns the stable native API bound to this client.
func (c *SberClient) Cards() *CardsAPI { return (*c.core().resources).Cards }

// Transfers returns the stable native API bound to this client.
func (c *SberClient) Transfers() *TransfersAPI { return (*c.core().resources).Transfers }

// Analytics returns the stable native API bound to this client.
func (c *SberClient) Analytics() *AnalyticsAPI { return (*c.core().resources).Analytics }

// Session returns the stable native API bound to this client.
func (c *SberClient) Session() *SessionAPI { return (*c.core().resources).Session }

// Portfolio performs one products read and returns a new bound snapshot.
// It shares the client-held resource bundle and one-shot workflow lifetime.
func (c *SberClient) Portfolio(ctx context.Context, forceUpdate bool) (*BankPortfolio, error) {
	return (*c.core().resources).Portfolio(ctx, forceUpdate)
}
