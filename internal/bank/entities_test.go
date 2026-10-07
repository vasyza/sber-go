package bank

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

func entityTransferProducts() Products {
	return Products{Accounts: []Account{{ID: "source-1", Kind: "ctaccount"}, {ID: "destination-1", Kind: "account"}}, Cards: []Card{{ID: "source-card"}}}
}
func TestEntityTransferToPreparesButNeverConfirms(t *testing.T) {
	for _, cardSource := range []bool{false, true} {
		sourceID := resourceFixtureSource
		if cardSource {
			sourceID = "card:source-card"
		}
		r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: resourceStartResponse()}, {Call: resourcePrepareCall(sourceID, resourceFixtureDestination, "10.50", "RUB", "Fixture purpose"), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)}}}
		p := NewBankPortfolio(entityTransferProducts(), r, ResourceOptions{AllowMutations: true})
		var got PreparedTransfer
		var err error
		if cardSource {
			got, err = p.Cards()[0].TransferTo(context.Background(), p.Accounts()[1], resourceAmount(t, "10.50"), TransferOptions{Currency: "RUB", PaymentPurpose: "Fixture purpose"})
		} else {
			got, err = p.Accounts()[0].TransferTo(context.Background(), p.Accounts()[1], resourceAmount(t, "10.50"), TransferOptions{Currency: "RUB", PaymentPurpose: "Fixture purpose"})
		}
		if err != nil || got.State() != "summary" || got.SourceID() != sourceID || r.sequences != 0 {
			t.Fatal("transfer helper confirmed or lost binding")
		}
		if p.Transfers() == nil {
			t.Fatal("standalone portfolio exposes no confirmation API")
		}
		r.done()
	}
}
func TestEntityAccountTransferToCardAcrossSameRequesterSnapshots(t *testing.T) {
	start := resourceStartResponse()
	refs := start["body"].(map[string]any)["output"].(map[string]any)["references"].(map[string]any)
	refs["toResource"].(map[string]any)["items"] = []any{map[string]any{"value": "card:destination-card", "properties": map[string]any{"type": "card", "name": "Synthetic destination card", "currency": "RUB"}}}
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceStartCall(), Response: start}, {Call: resourcePrepareCall(resourceFixtureSource, "card:destination-card", "1", "RUB", ""), Response: resourceWorkflowResponse("SUCCESS", "me2meCreate", "summary", "", nil)}}}
	source := NewBankPortfolio(entityTransferProducts(), r, ResourceOptions{AllowMutations: true})
	destination := NewBankPortfolio(Products{Cards: []Card{{ID: "destination-card"}}}, r)
	prepared, err := source.Accounts()[0].TransferTo(context.Background(), destination.Cards()[0], resourceAmount(t, "1"))
	if err != nil || prepared.DestinationID() != "card:destination-card" || prepared.SourceID() != resourceFixtureSource || r.sequences != 0 {
		t.Fatal("same-requester snapshot/card destination transfer failed")
	}
	r.done()
}

func TestEntityTransferToRejectsCrossClientAndInvalidValuesBeforeStart(t *testing.T) {
	r := &resourceScript{t: t}
	p := NewBankPortfolio(entityTransferProducts(), r, ResourceOptions{AllowMutations: true})
	other := NewBankPortfolio(entityTransferProducts(), &resourceScript{t: t}, ResourceOptions{AllowMutations: true})
	if _, err := p.Accounts()[0].TransferTo(context.Background(), other.Accounts()[1], resourceAmount(t, "1")); err == nil {
		t.Fatal("cross-client transfer")
	}
	for _, dest := range []BankProduct{nil, (*BankCard)(nil), p.Accounts()[0], NewBankAccount(Account{ID: "bad id", Kind: "account"}, r, nil)} {
		if _, err := p.Accounts()[0].TransferTo(context.Background(), dest, resourceAmount(t, "1")); err == nil {
			t.Fatal("invalid destination")
		}
	}
	if _, err := p.Cards()[0].TransferTo(context.Background(), p.Accounts()[1], resourceAmount(t, "0")); err == nil {
		t.Fatal("invalid amount started workflow")
	}
	disabled := NewBankPortfolio(entityTransferProducts(), r)
	if _, err := disabled.Accounts()[0].TransferTo(context.Background(), disabled.Accounts()[1], resourceAmount(t, "1")); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("helper default gate")
	}
	if len(r.calls) != 0 {
		t.Fatal("invalid transfer helper sent mutation")
	}
}
func TestEntityCardRenameUsesBoundGuardAndKeepsSnapshot(t *testing.T) {
	call := resourceRenameCall("mutation", 4004, "Main")
	call.PageID = "/app/cards/details/4004"
	r := &resourceScript{t: t, steps: []resourceStep{{Call: call, Response: map[string]any{"success": true}}}}
	p := NewBankPortfolio(entityFixtureProducts(t), r, ResourceOptions{AllowMutations: true})
	card := p.Cards()[0]
	if err := card.Rename(context.Background(), "Main"); err != nil {
		t.Fatal(err)
	}
	if card.Name() != "Fixture card" {
		t.Fatal("rename mutated immutable snapshot")
	}
	r.done()
	disabled := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	if err := disabled.Cards()[0].Rename(context.Background(), "Main"); !errors.Is(err, ErrResourceMutationsDisabled) {
		t.Fatal("bound rename default enabled")
	}
}

func TestEntityResourceSyntaxMatchesAccountKindsAndCard(t *testing.T) {
	p := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	for i, want := range [][2]string{{"ct-account:1001", "transactionAccount:1001"}, {"account:2002", "account:2002"}} {
		a := p.Accounts()[i]
		op, err := a.OperationResource()
		if err != nil || op != want[0] {
			t.Fatal("account operation resource")
		}
		tr, err := a.TransferResource()
		if err != nil || tr != want[1] {
			t.Fatal("account transfer resource")
		}
	}
	c := p.Cards()[0]
	op, err := c.OperationResource()
	if err != nil || op != "card:4004" {
		t.Fatal("card op syntax")
	}
	tr, err := c.TransferResource()
	if err != nil || tr != "card:4004" {
		t.Fatal("card transfer syntax")
	}
	for _, id := range []string{"", " bad", "bad ", "a/b", "bad\n", strings.Repeat("a", 129)} {
		a := NewBankAccount(Account{ID: id, Kind: "ctaccount"}, &resourceScript{t: t}, nil)
		c := NewBankCard(Card{ID: id}, &resourceScript{t: t}, nil)
		if _, err := a.OperationResource(); err == nil {
			t.Fatal("bad account op ID repaired")
		}
		if _, err := a.TransferResource(); err == nil {
			t.Fatal("bad transfer ID repaired")
		}
		if _, err := c.OperationResource(); err == nil {
			t.Fatal("bad card ID")
		}
		if _, err := c.TransferResource(); err == nil {
			t.Fatal("bad card transfer")
		}
	}
	a := NewBankAccount(Account{ID: "valid", Kind: "unknown"}, &resourceScript{t: t}, nil)
	if _, err := a.OperationResource(); err == nil {
		t.Fatal("unknown kind")
	}
	if _, err := a.TransferResource(); err == nil {
		t.Fatal("unknown kind transfer")
	}
	// No request is necessary to compute validated resource identifiers.
}
func TestEntityIterOperationsUsesScopedLazyIterator(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 2, "card:4004", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1", "lookahead")}, {Call: resourceHistoryCall(0, 2, "account:2002", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("2")}}}
	p := NewBankPortfolio(entityFixtureProducts(t), r)
	q := OperationsQuery{Limit: 1, MaxPages: 5, From: "2026-07-01"}
	seq := p.Cards()[0].IterOperations(context.Background(), q)
	if len(r.calls) != 0 {
		t.Fatal("iterator eager")
	}
	for op, err := range seq {
		if err != nil || op.ID != "1" {
			t.Fatal("card iterator")
		}
		break
	}
	for op, err := range p.Accounts()[1].IterOperations(context.Background(), q) {
		if err != nil || op.ID != "2" {
			t.Fatal("account iterator")
		}
	}
	r.done()
}
func TestEntityOperationsAlwaysScopedToSnapshotResource(t *testing.T) {
	r := &resourceScript{t: t, steps: []resourceStep{{Call: resourceHistoryCall(0, 11, "card:4004", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("1")}, {Call: resourceHistoryCall(0, 11, "ct-account:1001", "01.07.2026T00:00:00", ""), Response: resourceOperationsResponse("2")}}}
	p := NewBankPortfolio(entityFixtureProducts(t), r)
	q := OperationsQuery{Resource: "card:wrong-caller-scope", Limit: 10, MaxPages: 5, From: "2026-07-01"}
	cards, err := p.Cards()[0].Operations(context.Background(), q)
	if err != nil || cards[0].ScopeCardIDs[0] != "4004" {
		t.Fatal("scoped card operations")
	}
	accounts, err := p.Accounts()[0].Operations(context.Background(), q)
	if err != nil || len(accounts[0].ScopeCardIDs) != 0 {
		t.Fatal("scoped account operations")
	}
	r.done()
}

func cycle3AssertSnapshotError(t *testing.T, output []byte, err error) {
	t.Helper()
	var pe *ParseError
	if output != nil || !errors.As(err, &pe) || pe.Field() != "financial_snapshot" || pe.Error() != "sber: invalid domain data" || errors.Unwrap(pe) != nil {
		t.Fatalf("owned snapshot integrity failure missing/unsafe: data=%s error=%v", output, err)
	}
	if strings.Contains(err.Error(), "synthetic_native_") {
		t.Fatal("owned native error retained original text")
	}
}

func cycle3CheckOwnedNativeReject(t *testing.T, owned json.Marshaler, explicit func() ([]byte, error)) {
	t.Helper()
	data, err := explicit()
	cycle3AssertSnapshotError(t, data, err)
	data, err = owned.MarshalJSON()
	cycle3AssertSnapshotError(t, data, err)
	data, err = json.Marshal(owned)
	cycle3AssertSnapshotError(t, data, err)
	data, err = ExportJSON(owned)
	var pe *ParseError
	if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
		t.Fatal("generic owned export repaired native text")
	}
	normalized, err := JSONable(owned)
	if normalized != nil || !errors.As(err, &pe) || pe.Field() != "json_export" {
		t.Fatal("owned JSONable did not reject original native text eagerly")
	}
}

func TestEntityCycle3OrdinaryMarshalRejectsOriginalNativeText(t *testing.T) {
	for _, invalid := range []string{"synthetic_native_\xff", "synthetic_native_\xfe"} {
		t.Run(string([]byte{invalid[len(invalid)-1]}), func(t *testing.T) {
			for _, field := range []string{"ID", "Name", "Last4", "State", "Kind", "Balance.Currency"} {
				t.Run("account/"+field, func(t *testing.T) {
					raw := Account{ID: "literal-�", Balance: cycle3FinancialAmount(t), Kind: "ctaccount"}
					if field == "Balance.Currency" {
						raw.Balance.Currency = invalid
					} else {
						reflect.ValueOf(&raw).Elem().FieldByName(field).SetString(invalid)
					}
					owned := NewBankAccount(raw, nil, nil)
					cycle3CheckOwnedNativeReject(t, owned, owned.ExportJSON)
					if !reflect.DeepEqual(owned.Snapshot(), raw) {
						t.Fatal("failed account marshaling changed raw native bytes")
					}
				})
			}
			for _, field := range []string{"ID", "Name", "Last4", "Type", "State", "BalanceSource", "AccountID", "Balance.Currency", "AccountBalance.Currency"} {
				t.Run("card/"+field, func(t *testing.T) {
					raw := Card{ID: "literal-�", Balance: cycle3FinancialAmount(t), AccountBalance: cycle3FinancialAmount(t)}
					switch field {
					case "Balance.Currency":
						raw.Balance.Currency = invalid
					case "AccountBalance.Currency":
						raw.AccountBalance.Currency = invalid
					case "BalanceSource", "AccountID":
						reflect.ValueOf(&raw).Elem().FieldByName(field).Set(reflect.ValueOf(&invalid))
					default:
						reflect.ValueOf(&raw).Elem().FieldByName(field).SetString(invalid)
					}
					owned := NewBankCard(raw, nil, nil)
					cycle3CheckOwnedNativeReject(t, owned, owned.ExportJSON)
					if !reflect.DeepEqual(owned.Snapshot(), raw) {
						t.Fatal("failed card marshaling changed raw native bytes")
					}
				})
			}
		})
	}
	for _, tc := range []struct {
		name string
		raw  Products
	}{
		{"same_kind_distinct_invalid_card_ids", Products{Cards: []Card{{ID: "synthetic_native_\xff"}, {ID: "synthetic_native_\xfe"}}}},
		{"same_kind_distinct_invalid_account_ids", Products{Accounts: []Account{{ID: "synthetic_native_\xff"}, {ID: "synthetic_native_\xfe"}}}},
		{"nested_account_currency", Products{Accounts: []Account{{ID: "literal", Balance: &Money{Amount: cycle3FinancialAmount(t).Amount, Currency: "synthetic_native_\xff"}}}}},
		{"nested_card_account_id", Products{Cards: []Card{{ID: "literal", AccountID: func() *string { s := "synthetic_native_\xff"; return &s }()}}}},
	} {
		t.Run("portfolio/"+tc.name, func(t *testing.T) {
			owned := NewBankPortfolio(tc.raw, nil)
			cycle3CheckOwnedNativeReject(t, owned, owned.ExportJSON)
			if !reflect.DeepEqual(owned.Raw(), tc.raw) {
				t.Fatal("failed portfolio marshaling changed raw native bytes")
			}
		})
	}
}

func TestEntityCycle3OrdinaryMarshalKeepsCompactCompleteRawShape(t *testing.T) {
	m := cycle3FinancialAmount(t)
	m.Currency = "�"
	id := cycle3LiteralID
	empty := ""
	raw := Products{Accounts: []Account{{ID: id, Name: "literal � 😀 <>& PAN 4111-1111-1111-1111", Last4: "1111", State: "OPEN", Hidden: true, Arrested: true, Balance: m, Kind: "ctaccount"}, {}}, Cards: []Card{{ID: id, Name: "literal �", Last4: "1111", Type: "debit", State: "OPEN", Hidden: true, Arrested: true, IsMain: true, Balance: m, BalanceSource: &empty, AccountID: &id, AccountBalance: m}, {ID: "nil-amounts"}}}
	requester := &cycle3SnapshotRequester{Secret: "synthetic-private-\xff"}
	p := NewBankPortfolio(raw, requester)
	a, c := p.Accounts()[0], p.Cards()[0]
	for _, tc := range []struct {
		name     string
		value    json.Marshaler
		raw      any
		explicit func() ([]byte, error)
	}{
		{"account_pointer", a, raw.Accounts[0], a.ExportJSON}, {"account_value", *a, raw.Accounts[0], a.ExportJSON},
		{"card_pointer", c, raw.Cards[0], c.ExportJSON}, {"card_value", *c, raw.Cards[0], c.ExportJSON},
		{"portfolio_pointer", p, raw, p.ExportJSON}, {"portfolio_value", *p, raw, p.ExportJSON},
		{"zero_account", BankAccount{}, Account{}, (&BankAccount{}).ExportJSON},
		{"zero_card", BankCard{}, Card{}, (&BankCard{}).ExportJSON},
		{"zero_portfolio", BankPortfolio{}, Products{}, (&BankPortfolio{}).ExportJSON},
		{"empty_portfolio_lists", *NewBankPortfolio(Products{Accounts: []Account{}, Cards: []Card{}}, requester), Products{Accounts: []Account{}, Cards: []Card{}}, NewBankPortfolio(Products{Accounts: []Account{}, Cards: []Card{}}, requester).ExportJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := json.Marshal(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			direct, err := tc.value.MarshalJSON()
			if err != nil || !bytes.Equal(direct, want) || bytes.Contains(direct, []byte("\n")) {
				t.Fatalf("ordinary raw shape/compact bytes changed: got=%s want=%s error=%v", direct, want, err)
			}
			data, err := json.Marshal(tc.value)
			if err != nil || !bytes.Equal(data, want) {
				t.Fatalf("ordinary encoder raw bytes changed: %s %v", data, err)
			}
			data, err = tc.explicit()
			if err != nil || !reflect.DeepEqual(cycle3DecodedJSON(t, data), cycle3DecodedJSON(t, want)) {
				t.Fatalf("explicit raw financial export changed: %s %v", data, err)
			}
		})
	}
	if requester.MarshalCalls != 0 || !reflect.DeepEqual(p.Raw(), raw) {
		t.Fatal("ordinary serializer traversed requester or mutated snapshot")
	}
	for _, value := range []any{(*BankPortfolio)(nil), (*BankAccount)(nil), (*BankCard)(nil)} {
		data, err := json.Marshal(value)
		if err != nil || string(data) != "null" {
			t.Fatal("nil entity JSON behavior changed")
		}
	}
}

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
