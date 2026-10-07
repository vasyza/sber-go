package bank

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func entityFixtureProducts(t *testing.T) Products {
	t.Helper()
	balance := &Money{Amount: resourceAmount(t, "4111111111111111.50"), Currency: "RUB"}
	source := "availableTotalLimit"
	accountID := "1001"
	return Products{Accounts: []Account{{ID: "1001", Name: "Fixture current", Last4: "0001", State: "active", Hidden: true, Arrested: true, Balance: balance, Kind: "ctaccount"}, {ID: "2002", Name: "Fixture savings", Last4: "0002", State: "active", Kind: "account"}}, Cards: []Card{{ID: "4004", Name: "Fixture card", Last4: "0004", Type: "debit", State: "active", Hidden: true, Arrested: true, IsMain: true, Balance: &Money{Amount: resourceAmount(t, "10.50"), Currency: "RUB"}, BalanceSource: &source, AccountID: &accountID, AccountBalance: balance}, {ID: "5005", Name: "Second fixture", AccountID: &accountID}, {ID: "6006", Name: "Unlinked fixture"}}}
}
func TestEntityPortfolioPreservesEverySnapshotFieldAndRelationships(t *testing.T) {
	raw := entityFixtureProducts(t)
	r := &resourceScript{t: t}
	p := NewBankPortfolio(raw, r)
	if !reflect.DeepEqual(p.Raw(), raw) || len(p.Accounts()) != 2 || len(p.Cards()) != 3 {
		t.Fatal("snapshot loss")
	}
	a, c := p.Accounts()[0], p.Cards()[0]
	if !reflect.DeepEqual(a.Snapshot(), raw.Accounts[0]) || !reflect.DeepEqual(c.Snapshot(), raw.Cards[0]) {
		t.Fatal("bound fields lost")
	}
	if c.Account() != a || len(a.Cards()) != 2 || a.Cards()[0] != c || len(p.Accounts()[1].Cards()) != 0 || p.Cards()[2].Account() != nil {
		t.Fatal("wrong portfolio relation")
	}
	if a.ID() != "1001" || a.Name() != "Fixture current" || a.Last4() != "0001" || a.State() != "active" || !a.Hidden() || !a.Arrested() || a.Balance().Amount.String() != "4111111111111111.50" || a.Kind() != "ctaccount" {
		t.Fatal("account getter loss")
	}
	if c.ID() != "4004" || c.Name() != "Fixture card" || c.Last4() != "0004" || c.Type() != "debit" || c.State() != "active" || !c.Hidden() || !c.Arrested() || !c.IsMain() || c.Balance().Amount.String() != "10.50" || *c.BalanceSource() != "availableTotalLimit" || *c.AccountID() != "1001" || c.AccountBalance().Amount.String() != "4111111111111111.50" {
		t.Fatal("card getter loss")
	}
	// Constructors, reads and collection slices must not alias mutable raw models.
	raw.Accounts[0].Name = "changed"
	raw.Accounts[0].Balance.Currency = "USD"
	*raw.Cards[0].AccountID = "different"
	raw.Cards[0].Balance.Amount = resourceAmount(t, "0")
	snap := c.Snapshot()
	snap.Balance.Amount = resourceAmount(t, "0")
	*snap.AccountID = "different"
	copyRaw := p.Raw()
	copyRaw.Accounts[0].Name = "changed"
	p.Cards()[0] = nil
	p.Accounts()[0] = nil
	a.Cards()[0] = nil
	if a.Name() != "Fixture current" || a.Balance().Currency != "RUB" || c.Balance().Amount.String() != "10.50" || c.Account() != a || p.Cards()[0] != c || a.Cards()[0] != c {
		t.Fatal("mutable alias into portfolio")
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var export map[string]any
	if err = json.Unmarshal(data, &export); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"id", "name", "last4", "type", "state", "hidden", "arrested", "is_main", "balance", "balance_source", "account_id", "account_balance"}
	if len(export) != len(wantKeys) {
		t.Fatal("serialization field loss")
	}
	for _, key := range wantKeys {
		if _, ok := export[key]; !ok {
			t.Fatal("missing " + key)
		}
	}
	if !strings.Contains(string(data), "4111111111111111.50") || strings.Contains(string(data), "requester") {
		t.Fatal("financial amount silently masked or binding exported")
	}
	if fmt.Sprint(p) != "BankPortfolio(accounts=2, cards=3)" {
		t.Fatal("portfolio repr")
	}
}
func TestEntityPortfolioLinksOnlyUniqueCurrentAccountIDs(t *testing.T) {
	for _, duplicateCurrent := range []bool{false, true} {
		raw := entityFixtureProducts(t)
		kind := "account"
		if duplicateCurrent {
			kind = "ctaccount"
		}
		raw.Accounts = append(raw.Accounts, Account{ID: "1001", Kind: kind})
		p := NewBankPortfolio(raw, &resourceScript{t: t})
		if duplicateCurrent {
			if p.Cards()[0].Account() != nil || len(p.Accounts()[0].Cards()) != 0 {
				t.Fatal("ambiguous current ID guessed")
			}
		} else if p.Cards()[0].Account() != p.Accounts()[0] {
			t.Fatal("savings collision erased current relation")
		}
	}
}
func TestEntityDirectBoundConstructorsPreserveRawFields(t *testing.T) {
	raw := entityFixtureProducts(t)
	r := &resourceScript{t: t}
	p := NewBankPortfolio(raw, r)
	a := NewBankAccount(raw.Accounts[1], r, p)
	c := NewBankCard(raw.Cards[0], r, p)
	if !reflect.DeepEqual(a.Snapshot(), raw.Accounts[1]) || !reflect.DeepEqual(c.Snapshot(), raw.Cards[0]) || c.Account() != p.Accounts()[0] || len(a.Cards()) != 0 {
		t.Fatal("direct constructor fields/relations")
	}
}
