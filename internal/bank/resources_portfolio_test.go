package bank

import (
	"context"
	"testing"
)

func resourcePortfolioResponse() map[string]any {
	return resourceMap(`{"success":true,"body":{"sections":{"technicalSection":{"sectionProductData":{"ctaccounts":{"data":[{"id":1001,"name":"Fixture current","number":"unique","balance":{"amount":"10.50","currencyCode":"RUB"}}]},"accounts":{"data":[{"id":2002,"name":"Fixture savings"}]},"cardsInWallet":{"data":[{"id":4004,"name":"Fixture card","number":"0004","isCTA":true,"cardAccount":"unique","availableTotalLimit":{"amount":"2.50","currencyCode":"RUB"}}]}}}}}}`)
}
func resourceProductsCall(force bool) resourceCall {
	return resourceCall{Kind: "read", Path: "/main-screen/rest/v2/m1/web/section/meta", Payload: map[string]any{"withData": true, "forceUpdate": force}}
}
func TestResourceAggregatePortfolioSharesBoundAPIs(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}, {Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}, {Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}}}
	APIs := NewResources(r)
	p, err := APIs.Portfolio(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if p.Cards()[0].bankBinding().operations != APIs.Operations || p.Cards()[0].bankBinding().cards != APIs.Cards || p.Cards()[0].bankBinding().transfers != APIs.Transfers {
		t.Fatal("snapshot actions not bound to shared APIs")
	}
	cards, err := APIs.Cards.List(context.Background(), true)
	if err != nil || cards[0].bankBinding().transfers != APIs.Transfers {
		t.Fatal("cards shortcut binding")
	}
	accounts, err := APIs.Accounts.List(context.Background(), false)
	if err != nil || accounts[0].bankBinding().transfers != APIs.Transfers {
		t.Fatal("account shortcut binding")
	}
	r.done()
}
func TestResourceCardsListReturnsOneBoundPortfolioSnapshot(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceProductsCall(true), Response: resourcePortfolioResponse()}}}
	got, err := NewCardsAPI(r).List(context.Background(), true)
	if err != nil || len(got) != 1 || got[0].Account() == nil || got[0].Account().ID() != "1001" || got[0].Account().Cards()[0] != got[0] {
		t.Fatal("cards list inconsistent snapshot")
	}
	r.done()
}
func TestResourceAccountsListReturnsOneBoundPortfolioSnapshot(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceProductsCall(false), Response: resourcePortfolioResponse()}}}
	got, err := NewAccountsAPI(r).List(context.Background(), false)
	if err != nil || len(got) != 2 || len(got[0].Cards()) != 1 || got[0].Cards()[0].Account() != got[0] || got[1].Kind() != "account" {
		t.Fatal("account list not from one snapshot")
	}
	r.done()
}
