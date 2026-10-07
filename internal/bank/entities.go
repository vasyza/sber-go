// Client-bound immutable snapshots ported from the MIT-licensed Python SDK.
package bank

import (
	"fmt"
	"reflect"
	"slices"
)

type entityBinding struct {
	requester  BusinessRequester
	options    ResourceOptions
	operations *OperationsAPI
	cards      *CardsAPI
	transfers  *TransfersAPI
}

func newEntityBinding(requester BusinessRequester, options ResourceOptions) *entityBinding {
	return &entityBinding{requester, options, NewOperationsAPI(requester, options), NewCardsAPI(requester, options), NewTransfersAPI(requester, options)}
}
func entitySameRequester(a, b BusinessRequester) bool {
	if a == nil || b == nil {
		return false
	}
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	return av.Type() == bv.Type() && av.Kind() == reflect.Pointer && !av.IsNil() && !bv.IsNil() && av.Pointer() == bv.Pointer()
}
func entityBindingFor(requester BusinessRequester, portfolio *BankPortfolio, options []ResourceOptions) *entityBinding {
	if len(options) == 0 && portfolio != nil && portfolio.data != nil && entitySameRequester(requester, (*portfolio.data).binding.requester) {
		return (*portfolio.data).binding
	}
	return newEntityBinding(requester, resourceOptions(options))
}
func entityStringCopy(v *string) *string {
	if v == nil {
		return nil
	}
	s := *v
	return &s
}
func entityCopyAccount(v Account) Account { v.Balance = domainMoneyCopy(v.Balance); return v }
func entityCopyCard(v Card) Card {
	v.Balance = domainMoneyCopy(v.Balance)
	v.AccountBalance = domainMoneyCopy(v.AccountBalance)
	v.AccountID = entityStringCopy(v.AccountID)
	v.BalanceSource = entityStringCopy(v.BalanceSource)
	return v
}
func entityCopyProducts(p Products) Products {
	out := Products{Accounts: slices.Clone(p.Accounts), Cards: slices.Clone(p.Cards)}
	for i, v := range out.Accounts {
		out.Accounts[i] = entityCopyAccount(v)
	}
	for i, v := range out.Cards {
		out.Cards[i] = entityCopyCard(v)
	}
	return out
}

// BankPortfolio is one products snapshot. Relations express resolution in this
// snapshot, not proof that a missing/ambiguous bank-side relationship is absent.
// Terminal pointer storage keeps requester/session internals out of fmt paths.
type BankPortfolio struct{ data **bankPortfolioData }
type bankPortfolioData struct {
	raw            Products
	binding        *entityBinding
	accounts       []*BankAccount
	cards          []*BankCard
	accountsByID   map[string]*BankAccount
	cardsByAccount map[*BankAccount][]*BankCard
}

func NewBankPortfolio(raw Products, requester BusinessRequester, options ...ResourceOptions) *BankPortfolio {
	return newBankPortfolio(raw, newEntityBinding(requester, resourceOptions(options)))
}
func newBankPortfolio(raw Products, binding *entityBinding) *BankPortfolio {
	d := &bankPortfolioData{raw: entityCopyProducts(raw), binding: binding, accountsByID: map[string]*BankAccount{}, cardsByAccount: map[*BankAccount][]*BankCard{}}
	p := &BankPortfolio{data: &d}
	if raw.Accounts != nil {
		d.accounts = make([]*BankAccount, 0, len(raw.Accounts))
	}
	if raw.Cards != nil {
		d.cards = make([]*BankCard, 0, len(raw.Cards))
	}
	buckets := map[string][]*BankAccount{}
	for _, raw := range d.raw.Accounts {
		record := &bankAccountData{raw: entityCopyAccount(raw), binding: binding, portfolio: p}
		a := &BankAccount{data: &record}
		d.accounts = append(d.accounts, a)
		if raw.ID != "" && raw.Kind == "ctaccount" {
			buckets[raw.ID] = append(buckets[raw.ID], a)
		}
	}
	for id, accounts := range buckets {
		if len(accounts) == 1 {
			d.accountsByID[id] = accounts[0]
		}
	}
	for _, raw := range d.raw.Cards {
		record := &bankCardData{raw: entityCopyCard(raw), binding: binding, portfolio: p}
		c := &BankCard{data: &record}
		d.cards = append(d.cards, c)
		if a := c.Account(); a != nil {
			d.cardsByAccount[a] = append(d.cardsByAccount[a], c)
		}
	}
	return p
}
func (p *BankPortfolio) Raw() Products {
	if p == nil || p.data == nil {
		return Products{}
	}
	return entityCopyProducts((*p.data).raw)
}
func (p *BankPortfolio) Accounts() []*BankAccount {
	if p == nil || p.data == nil {
		return nil
	}
	return slices.Clone((*p.data).accounts)
}
func (p *BankPortfolio) Cards() []*BankCard {
	if p == nil || p.data == nil {
		return nil
	}
	return slices.Clone((*p.data).cards)
}
func (p *BankPortfolio) String() string {
	return fmt.Sprintf("BankPortfolio(accounts=%d, cards=%d)", len(p.Accounts()), len(p.Cards()))
}
func (p *BankPortfolio) Format(f fmt.State, verb rune) { domainSafeFormat(f, p.String()) }
func (p BankPortfolio) MarshalJSON() ([]byte, error)   { return entityMarshalFinancial((&p).Raw()) }

// BankAccount preserves every Account field. Snapshot and financial/pointer
// accessors copy mutable wrappers; identity and relationships are immutable.
type BankAccount struct{ data **bankAccountData }
type bankAccountData struct {
	raw       Account
	binding   *entityBinding
	portfolio *BankPortfolio
}

func NewBankAccount(raw Account, requester BusinessRequester, portfolio *BankPortfolio, options ...ResourceOptions) *BankAccount {
	d := &bankAccountData{entityCopyAccount(raw), entityBindingFor(requester, portfolio, options), portfolio}
	return &BankAccount{data: &d}
}
func (a *BankAccount) Snapshot() Account {
	if a == nil || a.data == nil {
		return Account{}
	}
	return entityCopyAccount((*a.data).raw)
}
func (a *BankAccount) ID() string      { return a.Snapshot().ID }
func (a *BankAccount) Name() string    { return a.Snapshot().Name }
func (a *BankAccount) Last4() string   { return a.Snapshot().Last4 }
func (a *BankAccount) State() string   { return a.Snapshot().State }
func (a *BankAccount) Hidden() bool    { return a.Snapshot().Hidden }
func (a *BankAccount) Arrested() bool  { return a.Snapshot().Arrested }
func (a *BankAccount) Balance() *Money { return a.Snapshot().Balance }
func (a *BankAccount) Kind() string    { return a.Snapshot().Kind }
func (a *BankAccount) Cards() []*BankCard {
	if a == nil || a.data == nil {
		return nil
	}
	p := (*a.data).portfolio
	if p == nil || p.data == nil {
		return nil
	}
	return slices.Clone((*p.data).cardsByAccount[a])
}
func (a *BankAccount) String() string                { return "BankAccount(snapshot)" }
func (a *BankAccount) Format(f fmt.State, verb rune) { domainSafeFormat(f, a.String()) }
func (a BankAccount) MarshalJSON() ([]byte, error)   { return entityMarshalFinancial((&a).Snapshot()) }

// BankCard preserves every Card field and an optional uniquely resolved current
// account. Financial export includes all source fields, never requester data.
type BankCard struct{ data **bankCardData }
type bankCardData struct {
	raw       Card
	binding   *entityBinding
	portfolio *BankPortfolio
}

func NewBankCard(raw Card, requester BusinessRequester, portfolio *BankPortfolio, options ...ResourceOptions) *BankCard {
	d := &bankCardData{entityCopyCard(raw), entityBindingFor(requester, portfolio, options), portfolio}
	return &BankCard{data: &d}
}
func (c *BankCard) Snapshot() Card {
	if c == nil || c.data == nil {
		return Card{}
	}
	return entityCopyCard((*c.data).raw)
}
func (c *BankCard) ID() string             { return c.Snapshot().ID }
func (c *BankCard) Name() string           { return c.Snapshot().Name }
func (c *BankCard) Last4() string          { return c.Snapshot().Last4 }
func (c *BankCard) Type() string           { return c.Snapshot().Type }
func (c *BankCard) State() string          { return c.Snapshot().State }
func (c *BankCard) Hidden() bool           { return c.Snapshot().Hidden }
func (c *BankCard) Arrested() bool         { return c.Snapshot().Arrested }
func (c *BankCard) IsMain() bool           { return c.Snapshot().IsMain }
func (c *BankCard) Balance() *Money        { return c.Snapshot().Balance }
func (c *BankCard) BalanceSource() *string { return c.Snapshot().BalanceSource }
func (c *BankCard) AccountID() *string     { return c.Snapshot().AccountID }
func (c *BankCard) AccountBalance() *Money { return c.Snapshot().AccountBalance }
func (c *BankCard) Account() *BankAccount {
	if c == nil || c.data == nil {
		return nil
	}
	d := *c.data
	if d.raw.AccountID == nil || d.portfolio == nil || d.portfolio.data == nil {
		return nil
	}
	return (*d.portfolio.data).accountsByID[*d.raw.AccountID]
}
func (c *BankCard) String() string                { return "BankCard(snapshot)" }
func (c *BankCard) Format(f fmt.State, verb rune) { domainSafeFormat(f, c.String()) }
func (c BankCard) MarshalJSON() ([]byte, error)   { return entityMarshalFinancial((&c).Snapshot()) }
