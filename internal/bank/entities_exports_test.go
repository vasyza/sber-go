package bank

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestEntityFinancialExportMethodsPreserveMoney(t *testing.T) {
	p := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	exports := []func() ([]byte, error){p.ExportJSON, p.Accounts()[0].ExportJSON, p.Cards()[0].ExportJSON}
	for _, export := range exports {
		data, err := export()
		if err != nil || !strings.Contains(string(data), "4111111111111111.50") {
			t.Fatalf("typed financial export changed amount: %s %v", data, err)
		}
	}
}
func TestEntityFinancialExportPreservesLiteralIdentifiers(t *testing.T) {
	card := NewBankCard(Card{ID: "4111111111111111", Name: "Fixture", Balance: &Money{Amount: resourceAmount(t, "10.50"), Currency: "RUB"}}, &resourceScript{t: t}, nil)
	data, err := card.ExportJSON()
	if err != nil || !strings.Contains(string(data), `"id": "4111111111111111"`) {
		t.Fatal("explicit financial snapshot export altered a literal product ID")
	}
}

func TestEntityFinancialExportRejectsMalformedUTF8RatherThanRepair(t *testing.T) {
	for _, raw := range []Card{{ID: "bad\xff"}, {ID: "valid", Name: "bad\xff"}, {ID: "valid", Balance: &Money{Amount: resourceAmount(t, "1"), Currency: "RU\xff"}}} {
		card := NewBankCard(raw, &resourceScript{t: t}, nil)
		if _, err := card.ExportJSON(); err == nil {
			t.Fatal("explicit snapshot export repaired malformed UTF-8")
		}
	}
}

func TestEntityBoundFormattingNeverTraversesRequester(t *testing.T) {
	b, c := resourceSession(t)
	r := &resourceScript{t: t, bundle: b, credentials: c}
	p := NewBankPortfolio(entityFixtureProducts(t), r)
	values := []any{p, *p, p.Accounts()[0], *p.Accounts()[0], p.Cards()[0], *p.Cards()[0], struct{ private BankCard }{*p.Cards()[0]}, []any{p, p.Accounts()[0], p.Cards()[0]}}
	for _, value := range values {
		for _, verb := range []string{"%s", "%q", "%v", "%+v", "%#v", "%d", "%p", "%w"} {
			data := fmt.Sprintf(verb, value)
			if strings.Contains(data, "fixture-session") || strings.Contains(data, "fixture-token") {
				t.Fatal("fmt traversed requester")
			}
		}
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "fixture-token") || strings.Contains(string(data), "fixture-session") {
			t.Fatal("JSON exported requester")
		}
	}
}
