package bank

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"github.com/vasyza/sber-go/internal/strictjson"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Synthetic financial-export seam probe; parent owns the generic model walker.
func TestProbeBoundFinancialExport(t *testing.T) {
	p := NewBankPortfolio(entityFixtureProducts(t), &resourceScript{t: t})
	card := p.Cards()[0]
	direct, _ := json.Marshal(card)
	snapshot, _ := ExportJSON(card.Snapshot())
	bound, _ := ExportJSON(card)
	t.Logf("marshal=%s snapshot=%s bound=%s", direct, snapshot, bound)
	if !strings.Contains(string(direct), "4111111111111111.50") || !strings.Contains(string(snapshot), "4111111111111111.50") {
		t.Fatal("raw financial snapshot broken")
	}
	if !strings.Contains(string(bound), "4111111111111111.50") {
		t.Fatal("generic ExportJSON on bound custom marshaler reparses Money as display text")
	}
}

// Independent synthetic seam matrix. No requester, session, network or tenant.
func TestFreshReviewFinancialControls(t *testing.T) {
	d, e := ParseDecimal("4111111111111111.50")
	if e != nil {
		t.Fatal(e)
	}
	m := &Money{Amount: d, Currency: "RUB"}
	aid := "fixture-parent"
	raw := Products{Accounts: []Account{{ID: aid, Name: RedactPAN("PAN 4111-1111-1111-1111"), Balance: m, Kind: "ctaccount"}}, Cards: []Card{{ID: "fixture-card", AccountID: &aid, Balance: m, AccountBalance: m}}}
	p := NewBankPortfolio(raw, nil)
	c := p.Cards()[0]
	a := p.Accounts()[0]
	for _, v := range []any{raw, a.Snapshot(), c.Snapshot(), p.Raw(), map[string]any{"products": p.Raw()}, []any{a.Snapshot(), c.Snapshot()}} {
		b, e := ExportJSON(v)
		if e != nil || !strings.Contains(string(b), d.String()) {
			t.Fatalf("native typed snapshot control broken: %s %v", b, e)
		}
	}
	for _, f := range []func() ([]byte, error){p.ExportJSON, a.ExportJSON, c.ExportJSON} {
		b, e := f()
		if e != nil || !strings.Contains(string(b), d.String()) {
			t.Fatalf("entity-specific financial control broken: %s %v", b, e)
		}
	}
	literal := NewBankAccount(Account{ID: "4111111111111111", Balance: m, Kind: "ctaccount"}, nil, nil)
	b, e := literal.ExportJSON()
	if e != nil || !strings.Contains(string(b), `"id": "4111111111111111"`) {
		t.Fatalf("literal entity ID control broken: %s %v", b, e)
	}
	if b, e := json.Marshal(literal.Snapshot()); e != nil || !strings.Contains(string(b), `"id":"4111111111111111"`) {
		t.Fatal("raw snapshot literal ID control broken")
	}
}
func TestFreshReviewFinancialLiteralIDs(t *testing.T) {
	d, _ := ParseDecimal("4111111111111111.50")
	m := &Money{Amount: d, Currency: "RUB"}
	raw := Account{ID: "4111111111111111", Name: "Fixture", Balance: m, Kind: "ctaccount"}
	standard, e := json.Marshal(raw)
	if e != nil || !strings.Contains(string(standard), `"id":"4111111111111111"`) {
		t.Fatal("raw literal ID control broken")
	}
	generic, e := ExportJSON(raw)
	if e != nil {
		t.Fatal(e)
	}
	t.Logf("native_snapshot_generic=%s", generic)
	if !strings.Contains(string(generic), `"id": "4111111111111111"`) {
		t.Error("generic financial snapshot export rewrote a literal account ID as display PAN")
	}
}
func TestFreshReviewFinancialSeamMatrix(t *testing.T) {
	d, _ := ParseDecimal("4111111111111111.50")
	m := &Money{Amount: d, Currency: "RUB"}
	p := NewBankPortfolio(Products{Accounts: []Account{{ID: "fixture-parent", Balance: m, Kind: "ctaccount"}}, Cards: []Card{{ID: "fixture-card", Balance: m, AccountBalance: m}}}, nil)
	a := p.Accounts()[0]
	c := p.Cards()[0]
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"bound_account_pointer", a}, {"bound_card_pointer", c}, {"bound_portfolio_pointer", p},
		{"bound_account_value", *a}, {"bound_card_value", *c}, {"bound_portfolio_value", *p},
		{"slice", []any{a, c, p}}, {"map", map[string]any{"a": a, "c": c, "p": p}}, {"array", [1]*BankCard{c}},
		{"addressable_field", &struct {
			Card BankCard `json:"card"`
		}{*c}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, e := ExportJSON(tc.value)
			if e != nil {
				t.Fatalf("generic financial export returned an error: %v", e)
			}
			t.Logf("generic=%s", b)
			if !strings.Contains(string(b), d.String()) || strings.Contains(string(b), "•••• 1111.50") {
				t.Error("generic export converted a typed financial amount to PAN-masked display text")
			}
		})
	}
}

type cycle3ForeignAccount Account

func TestModelCycle3FinancialLiteralCodesAndExactNumbers(t *testing.T) {
	id := cycle3LiteralID
	m := cycle3FinancialAmount(t)
	m.Currency = id // Source currency is a literal code, never display text.
	account := Account{ID: id, Balance: m, Kind: "ctaccount"}
	card := Card{ID: id, AccountID: &id, Balance: m, AccountBalance: m}
	raw := Products{Accounts: []Account{account}, Cards: []Card{card}}
	p := NewBankPortfolio(raw, nil)
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"currency_money_value", *m, *m}, {"currency_money_pointer", m, m},
		{"currency_account", account, account}, {"currency_card", card, card}, {"currency_products", raw, raw},
		{"currency_bound_account", p.Accounts()[0], account}, {"currency_bound_card", p.Cards()[0], card}, {"currency_bound_portfolio", p, raw},
		{"currency_stream", cycle3TypedStreamFallback{Payload: raw}, map[string]any{"payload": raw, "display": ""}},
		{"currency_stream_bound", cycle3TypedStreamFallback{Payload: p}, map[string]any{"payload": raw, "display": ""}},
		{"currency_scalar_stream", cycle3TypedStreamFallback{Payload: m}, map[string]any{"payload": m, "display": ""}},
		{"currency_mixed", []any{m, p.Cards()[0], card}, []any{m, card, card}},
		{"account_kind_literal", Account{Kind: id}, Account{Kind: id}},
		{"resource_type_literal", Resource{Type: id, ID: id}, Resource{Type: id, ID: id}},
		{"operation_date_literal", Operation{ID: id, Date: id}, Operation{ID: id, Date: id}},
		{"card_balance_source_literal", Card{ID: id, BalanceSource: &id}, Card{ID: id, BalanceSource: &id}},
		{"ledger_direction_literal", CardLedgerEntry{OperationID: id, CardID: id, Direction: id}, CardLedgerEntry{OperationID: id, CardID: id, Direction: id}},
		{"foreign_named_native_schema", cycle3ForeignAccount(Account{ID: id, Kind: id}), Account{ID: "•••• 1111", Kind: "•••• 1111"}},
		{"raw_number_luhn", json.Number(id), json.Number(id)},
		{"raw_number_exact", json.Number("9007199254740993.0100"), json.Number("9007199254740993.0100")},
		{"decimal_luhn", m.Amount, m.Amount},
		{"decimal_pointer_luhn", &m.Amount, &m.Amount},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.want) })
	}
	// Currency outside native Money is still display text, even at that JSON key.
	cycle3AssertGenericJSON(t, map[string]any{"currency": id}, map[string]any{"currency": "•••• 1111"})
	for _, input := range []any{math.NaN(), math.Inf(1), math.Inf(-1), float32(math.Inf(1)), nil, true, false, "NaN", "Infinity"} {
		if _, err := ParseDecimal(input); err == nil {
			t.Fatalf("nonfinite/nonmonetary amount accepted: %T", input)
		}
	}
	for _, text := range []string{"-0.00", "9007199254740993.0100", "4111111111111111.50", "-4111111111111111.50", "1.2300e-1000"} {
		d, err := ParseDecimal(json.Number(text))
		if err != nil {
			t.Fatal(err)
		}
		cycle3AssertGenericJSON(t, Money{Amount: d, Currency: "�"}, Money{Amount: d, Currency: "�"})
	}
	for _, payload := range []any{(*Money)(nil), (*Decimal)(nil), (*BankAccount)(nil), (*BankCard)(nil), (*BankPortfolio)(nil), (*Account)(nil), (*Card)(nil), (*Resource)(nil), (*Operation)(nil)} {
		cycle3AssertGenericJSON(t, cycle3TypedStreamFallback{Payload: payload}, map[string]any{"payload": nil, "display": ""})
	}
}

// Synthetic exact native identity roles; no requester/session/network.
const cycle3LiteralID = "4111111111111111"

func cycle3FinancialAmount(t *testing.T) *Money {
	t.Helper()
	d, err := ParseDecimal("4111111111111111.50")
	if err != nil {
		t.Fatal(err)
	}
	return &Money{Amount: d, Currency: "RUB"}
}

func cycle3DecodedJSON(t *testing.T, data []byte) any {
	t.Helper()
	if strictjson.Validate(data) != nil {
		t.Fatalf("invalid complete document: %s", data)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func cycle3AssertGenericJSON(t *testing.T, input, expected any) {
	t.Helper()
	want, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ExportJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cycle3DecodedJSON(t, out), cycle3DecodedJSON(t, want)) {
		t.Fatalf("native financial shape/identity/value changed: got=%s want=%s", out, want)
	}
	normalized, err := JSONable(input)
	if err != nil {
		t.Fatal(err)
	}
	normalizedJSON, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cycle3DecodedJSON(t, normalizedJSON), cycle3DecodedJSON(t, want)) {
		t.Fatalf("JSONable lost native provenance: got=%s want=%s", normalizedJSON, want)
	}
	after, err := json.Marshal(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("financial export mutated its input")
	}
}

func TestModelCycle3LiteralIdentityIsTypedNotDisplay(t *testing.T) {
	m := cycle3FinancialAmount(t)
	id := cycle3LiteralID
	empty := ""
	financial := true
	pan := "PAN 4111-1111-1111-1111"
	masked := "PAN •••• 1111"
	account := Account{ID: id, Name: pan, Last4: "1111", State: "OPEN", Hidden: true, Arrested: true, Balance: m, Kind: "ctaccount"}
	card := Card{ID: id, Name: pan, Last4: "1111", Type: "debit", State: "OPEN", Hidden: true, Arrested: true, IsMain: true, Balance: m, BalanceSource: &empty, AccountID: &id, AccountBalance: m}
	op := Operation{ID: id, Date: "2026-10-01", Form: pan, Type: "transfer", ClassificationCode: id, CreationChannel: pan, State: "DONE", StateName: pan, StateDescription: pan, Merchant: pan, Description: pan, Amount: m, BillingAmount: m, NationalAmount: m, Commission: m, Tips: m, RefusalReason: pan, FromResource: &Resource{Type: "card", ID: id}, ToResource: &Resource{Type: "account", ID: id}, ScopeCardIDs: []string{id, "literal-second"}, IsFinancial: &financial, IsHidden: true, BalanceAfter: m, BalanceAfterResourceID: &id, BalanceAfterResourceName: &pan}
	wantAccount := account
	wantAccount.Name = masked
	wantCard := card
	wantCard.Name = masked
	wantOperation := op
	wantOperation.Form = masked
	wantOperation.CreationChannel = masked
	wantOperation.StateName = masked
	wantOperation.StateDescription = masked
	wantOperation.Merchant = masked
	wantOperation.Description = masked
	wantOperation.RefusalReason = masked
	wantOperation.BalanceAfterResourceName = &masked
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"account", account, wantAccount},
		{"account_pointer", &account, &wantAccount},
		{"card", card, wantCard},
		{"card_pointer", &card, &wantCard},
		{"resource", Resource{Type: "card", ID: id}, Resource{Type: "card", ID: id}},
		{"operation", op, wantOperation},
		{"operation_pointer", &op, &wantOperation},
		{"ledger", CardLedgerEntry{OperationID: id, CardID: id, Direction: "debit", Amount: m}, CardLedgerEntry{OperationID: id, CardID: id, Direction: "debit", Amount: m}},
		{"detail", OperationDetail{UOHID: id, Title: pan, Amount: m, Fields: []OperationDetailField{{Name: pan, Type: "money", Value: m}, {Name: "text", Value: pan}}}, OperationDetail{UOHID: id, Title: masked, Amount: m, Fields: []OperationDetailField{{Name: masked, Type: "money", Value: m}, {Name: "text", Value: masked}}}},
		{"card_info", CardInfo{ID: id, Name: pan, CardHolder: pan, Limits: CardLimits{Purchase: m, Available: m, AvailableTotal: m}, Credit: &CreditInfo{Limit: m, OwnSum: m, Debt: m, MinPayment: m, MinPaymentDate: "2026-10-01"}}, CardInfo{ID: id, Name: masked, CardHolder: masked, Limits: CardLimits{Purchase: m, Available: m, AvailableTotal: m}, Credit: &CreditInfo{Limit: m, OwnSum: m, Debt: m, MinPayment: m, MinPaymentDate: "2026-10-01"}}},
		{"category", CategoryAmount{ID: id, Name: pan, ExternalID: pan, NationalAmount: m, VisibleAmount: m, CountOperations: 3}, CategoryAmount{ID: id, Name: masked, ExternalID: masked, NationalAmount: m, VisibleAmount: m, CountOperations: 3}},
		{"products", Products{Accounts: []Account{account}, Cards: []Card{card}}, Products{Accounts: []Account{wantAccount}, Cards: []Card{wantCard}}},
		{"operations_page", OperationsPage{Operations: []Operation{op}}, OperationsPage{Operations: []Operation{wantOperation}}},
		{"nil_literal_pointers", Card{ID: id}, Card{ID: id}},
		{"nil_scope", Operation{ID: id}, Operation{ID: id}},
		{"empty_scope", Operation{ID: id, ScopeCardIDs: []string{}}, Operation{ID: id, ScopeCardIDs: []string{}}},
		{"ordinary_id_key_is_display", map[string]any{"id": id, "amount": id, "display": pan}, map[string]any{"id": "•••• 1111", "amount": "•••• 1111", "display": masked}},
		{"foreign_native_tag_is_not_provenance", struct {
			ID string `json:"id" sber:"literal"`
		}{id}, map[string]any{"id": "•••• 1111"}},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.want) })
	}
}

type cycle3SnapshotRequester struct {
	BusinessRequester // nil: any attempted resource/session activity panics.
	Secret            string
	MarshalCalls      int
}

func (r *cycle3SnapshotRequester) MarshalJSON() ([]byte, error) {
	r.MarshalCalls++
	return []byte(`"<requester-redacted>"`), nil
}

type cycle3ForeignLegacyEntity struct {
	BankAccount
	Password string
}

func (cycle3ForeignLegacyEntity) MarshalJSON() ([]byte, error) {
	return []byte(`{"id":"<foreign-redacted>","amount":"4111111111111111.50"}`), nil
}

type cycle3ForeignStreamEntity struct {
	BankCard
	Password string
}

func (*cycle3ForeignStreamEntity) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteValue([]byte(`{"id":"<foreign-stream-redacted>","amount":"4111111111111111.50"}`))
}

func TestModelCycle3BoundFinancialProvenancePreservesContainerShape(t *testing.T) {
	m := cycle3FinancialAmount(t)
	id := cycle3LiteralID
	empty := ""
	pan := "PAN 4111-1111-1111-1111"
	masked := "PAN •••• 1111"
	raw := Products{Accounts: []Account{{ID: id, Name: pan, Last4: "1111", State: "OPEN", Hidden: true, Arrested: true, Balance: m, Kind: "ctaccount"}}, Cards: []Card{{ID: id, Name: pan, Last4: "1111", Type: "debit", State: "OPEN", Hidden: true, Arrested: true, IsMain: true, Balance: m, BalanceSource: &empty, AccountID: &id, AccountBalance: m}}}
	want := entityCopyProducts(raw)
	want.Accounts[0].Name = masked
	want.Cards[0].Name = masked
	requester := &cycle3SnapshotRequester{Secret: "synthetic-private-\xff"}
	p := NewBankPortfolio(raw, requester)
	a, c := p.Accounts()[0], p.Cards()[0]
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"account_pointer", a, want.Accounts[0]}, {"card_pointer", c, want.Cards[0]}, {"portfolio_pointer", p, want},
		{"account_value", *a, want.Accounts[0]}, {"card_value", *c, want.Cards[0]}, {"portfolio_value", *p, want},
		{"slice", []any{a, c, p}, []any{want.Accounts[0], want.Cards[0], want}},
		{"map", map[string]any{"a": a, "c": c, "p": p}, map[string]any{"a": want.Accounts[0], "c": want.Cards[0], "p": want}},
		{"array", [1]*BankCard{c}, []any{want.Cards[0]}},
		{"addressable_field", &struct {
			Card BankCard `json:"card"`
		}{*c}, map[string]any{"card": want.Cards[0]}},
		{"interface_field", &struct {
			Account any `json:"account"`
		}{a}, map[string]any{"account": want.Accounts[0]}},
		{"addressable_array", &[2]BankAccount{*a, *a}, []any{want.Accounts[0], want.Accounts[0]}},
		{"slice_values", []BankPortfolio{*p, *p}, []any{want, want}},
		{"native_and_bound", []any{a.Snapshot(), *a}, []any{want.Accounts[0], want.Accounts[0]}},
		{"nil_account", (*BankAccount)(nil), nil}, {"nil_card", (*BankCard)(nil), nil}, {"nil_portfolio", (*BankPortfolio)(nil), nil},
		{"zero_account", BankAccount{}, Account{}}, {"zero_card", BankCard{}, Card{}}, {"zero_portfolio", BankPortfolio{}, Products{}},
		{"nil_products_lists", NewBankPortfolio(Products{}, requester), Products{}},
		{"empty_products_lists", NewBankPortfolio(Products{Accounts: []Account{}, Cards: []Card{}}, requester), Products{Accounts: []Account{}, Cards: []Card{}}},
		{"foreign_legacy_embedded_entity", cycle3ForeignLegacyEntity{BankAccount: *a, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-redacted>", "amount": "•••• 1111.50"}},
		{"foreign_stream_embedded_entity", &cycle3ForeignStreamEntity{BankCard: *c, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-stream-redacted>", "amount": "•••• 1111.50"}},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.want) })
	}
	if !reflect.DeepEqual(p.Raw(), raw) || requester.MarshalCalls != 0 || c.Account() != a || a.Cards()[0] != c {
		t.Fatal("financial export traversed requester or changed snapshots/relations")
	}
	copy := a.Snapshot()
	copy.Balance.Currency = "USD"
	if a.Balance().Currency != "RUB" {
		t.Fatal("financial export damaged defensive copy semantics")
	}
}

// The full pinned JSON serializer interface set is exercised using synthetic
// financial payloads that a FOREIGN redactor intentionally does not export.
type cycle3TextOnlySecret struct {
	Financial any
	Password  string
}

func (cycle3TextOnlySecret) MarshalText() ([]byte, error) { return []byte("<text-redacted>"), nil }

type cycle3TextAppendSecret struct {
	Financial any
	Password  string
}

func (*cycle3TextAppendSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("<append-redacted>")...), nil
}

type cycle3TextBothSecret struct {
	Financial any
	Password  string
}

func (*cycle3TextBothSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("<append-over-text-redacted>")...), nil
}
func (cycle3TextBothSecret) MarshalText() ([]byte, error) {
	return []byte("synthetic-lower-priority-private-text"), nil
}

type cycle3JSONLegacyTextSecret struct {
	Financial any
	Password  string
}

func (cycle3JSONLegacyTextSecret) MarshalJSON() ([]byte, error) {
	return []byte(`"<legacy-json-over-text-redacted>"`), nil
}
func (*cycle3JSONLegacyTextSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("synthetic-lower-priority-private-append")...), nil
}
func (cycle3JSONLegacyTextSecret) MarshalText() ([]byte, error) {
	return []byte("synthetic-lower-priority-private-text"), nil
}

type cycle3JSONStreamAllSecret struct {
	Financial any
	Password  string
}

func (*cycle3JSONStreamAllSecret) MarshalJSONTo(e *jsontext.Encoder) error {
	return e.WriteToken(jsontext.String("<stream-over-all-redacted>"))
}
func (cycle3JSONStreamAllSecret) MarshalJSON() ([]byte, error) {
	return []byte(`"synthetic-lower-priority-private-json"`), nil
}
func (*cycle3JSONStreamAllSecret) AppendText(b []byte) ([]byte, error) {
	return append(b, []byte("synthetic-lower-priority-private-append")...), nil
}
func (cycle3JSONStreamAllSecret) MarshalText() ([]byte, error) {
	return []byte("synthetic-lower-priority-private-text"), nil
}

func TestModelCycle3FinancialContextHonorsAllForeignSerializerInterfaces(t *testing.T) {
	m := cycle3FinancialAmount(t)
	p := NewBankPortfolio(Products{Accounts: []Account{{ID: cycle3LiteralID, Balance: m}}, Cards: []Card{{ID: cycle3LiteralID, Balance: m}}}, nil)
	for _, payload := range []any{Account{ID: "synthetic_native_\xff"}, m, p, *p, p.Accounts()[0], *p.Accounts()[0], p.Cards()[0], *p.Cards()[0], []any{m, p.Cards()[0]}} {
		for _, tc := range []struct {
			name            string
			input, expected any
		}{
			{"text", cycle3TextOnlySecret{payload, "synthetic-private-\xff"}, "<text-redacted>"},
			{"append", &cycle3TextAppendSecret{payload, "synthetic-private-\xff"}, "<append-redacted>"},
			{"append_over_text", &cycle3TextBothSecret{payload, "synthetic-private-\xff"}, "<append-over-text-redacted>"},
			{"legacy_over_text", &cycle3JSONLegacyTextSecret{payload, "synthetic-private-\xff"}, "<legacy-json-over-text-redacted>"},
			{"stream_over_all", &cycle3JSONStreamAllSecret{payload, "synthetic-private-\xff"}, "<stream-over-all-redacted>"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				cycle3AssertGenericJSON(t, tc.input, tc.expected)
				cycle3AssertGenericJSON(t, []any{tc.input}, []any{tc.expected})
				cycle3AssertGenericJSON(t, map[string]any{"credentials": tc.input}, map[string]any{"credentials": tc.expected})
				cycle3AssertGenericJSON(t, cycle3TypedStreamFallback{Payload: tc.input}, map[string]any{"payload": tc.expected, "display": ""})
				cycle3AssertGenericJSON(t, OperationDetail{UOHID: cycle3LiteralID, Fields: []OperationDetailField{{Value: tc.input}, {Value: m}}}, OperationDetail{UOHID: cycle3LiteralID, Fields: []OperationDetailField{{Value: tc.expected}, {Value: m}}})
			})
		}
	}
	for _, tc := range []struct {
		name            string
		input, expected any
	}{
		{"addressable_struct", &struct {
			Credential cycle3TextAppendSecret `json:"credential"`
		}{cycle3TextAppendSecret{p, "synthetic-private-\xff"}}, map[string]any{"credential": "<append-redacted>"}},
		{"addressable_slice", []cycle3TextAppendSecret{{p, "synthetic-private-\xff"}}, []any{"<append-redacted>"}},
		{"addressable_array", &[1]cycle3TextAppendSecret{{p, "synthetic-private-\xff"}}, []any{"<append-redacted>"}},
		{"nil_text_pointer", (*cycle3TextAppendSecret)(nil), nil},
	} {
		t.Run(tc.name, func(t *testing.T) { cycle3AssertGenericJSON(t, tc.input, tc.expected) })
	}
}

type cycle3TextFailureFloat float64

func (cycle3TextFailureFloat) MarshalText() ([]byte, error) {
	return nil, errors.New("synthetic-private-text-error")
}

type cycle3AppendFailure struct{ Data []byte }

func (v cycle3AppendFailure) AppendText(b []byte) ([]byte, error) { return append(b, v.Data...), nil }

func TestModelCycle3TextErrorsDoNotGainDeferredStreamingBypass(t *testing.T) {
	v := cycle3TextFailureFloat(1.25)
	for _, input := range []any{v, &v, cycle3AppendFailure{Data: []byte{0xff}}, cycle3AppendFailure{Data: []byte{0xfe}}} {
		out, err := ExportJSON(input)
		var pe *ParseError
		if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
			t.Fatal("text serializer error repaired/leaked")
		}
		normalized, err := JSONable(input)
		if normalized != nil || !errors.As(err, &pe) {
			t.Fatal("text error incorrectly acquired streaming-only deferred compatibility")
		}
	}
}

// Original Python3.12 pure-helper result, not a manufactured native oracle.
// Fixture SHA256: e868968bc8c14c152daa8adba4ed69f13d7382d33524e7e4293eb65d40c216a3

func TestModelCycle3OriginalSourceFinancialSnapshotOracle(t *testing.T) {
	data, err := os.ReadFile("../../testdata/financial/source-financial-oracle.original.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != "e868968bc8c14c152daa8adba4ed69f13d7382d33524e7e4293eb65d40c216a3" {
		t.Fatal("original-source fixture bytes changed")
	}
	var fixture struct {
		CanonicalCommit string `json:"canonical_commit"`
		Records         []struct {
			Kind          string `json:"kind"`
			Raw, Expected json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.CanonicalCommit != "984d45ae6ddbf37224e8146df2788e8821108c9e" || len(fixture.Records) != 22 {
		t.Fatal("canonical source/denominator changed")
	}
	for _, row := range fixture.Records {
		t.Run(row.Kind, func(t *testing.T) {
			var value any
			switch row.Kind {
			case "Money", "MoneyScientific", "MoneySignedZero":
				value = new(Money)
			case "Account", "AccountDefault", "BankAccount":
				value = new(Account)
			case "Card", "BankCard":
				value = new(Card)
			case "Resource":
				value = new(Resource)
			case "Operation":
				value = new(Operation)
			case "CardLedgerEntry":
				value = new(CardLedgerEntry)
			case "Products", "BankPortfolioRaw":
				value = new(Products)
			case "OperationsPage":
				value = new(OperationsPage)
			case "OperationDetailField":
				value = new(OperationDetailField)
			case "OperationDetail":
				value = new(OperationDetail)
			case "CardLimits":
				value = new(CardLimits)
			case "CreditInfo":
				value = new(CreditInfo)
			case "CardInfo":
				value = new(CardInfo)
			case "CategoryAmount":
				value = new(CategoryAmount)
			case "PFMPeriod":
				value = new(PFMPeriod)
			case "PFMAmounts":
				value = new(PFMAmounts)
			default:
				t.Fatal("unaccounted original-source financial case")
			}
			if err := json.Unmarshal(row.Raw, value); err != nil {
				t.Fatal(err)
			}
			switch row.Kind {
			case "BankAccount":
				value = NewBankAccount(*value.(*Account), nil, nil)
			case "BankCard":
				value = NewBankCard(*value.(*Card), nil, nil)
			case "BankPortfolioRaw":
				value = NewBankPortfolio(*value.(*Products), nil)
			}
			want := cycle3DecodedJSON(t, row.Expected)
			cycle3AssertGenericJSON(t, value, want)
		})
	}
}

// The native value is deliberately delegated through the official encoder.
// Custom raw JSON alongside it must not inherit native financial provenance.
type cycle3TypedStreamContainer struct {
	Payload any
	Secret  string
}

func (v cycle3TypedStreamContainer) MarshalJSONTo(e *jsontext.Encoder) error {
	if err := e.WriteToken(jsontext.BeginObject); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("native/~snapshot")); err != nil {
		return err
	}
	if err := jsonv2.MarshalEncode(e, v.Payload); err != nil {
		return err
	}
	if err := e.WriteToken(jsontext.String("raw")); err != nil {
		return err
	}
	if err := e.WriteValue([]byte(`{"id":"4111111111111111","amount":"4111111111111111.50","n":9007199254740993.0100,"tiny":1.2300e-1000}`)); err != nil {
		return err
	}
	return e.WriteToken(jsontext.EndObject)
}
func (cycle3TypedStreamContainer) MarshalJSON() ([]byte, error) {
	return []byte(`"synthetic-legacy-private"`), nil
}

type cycle3TypedStreamFallback struct {
	Payload any    `json:"payload"`
	Display string `json:"display"`
}

func (cycle3TypedStreamFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }

type cycle3TypedLegacyFallback struct{ Payload any }

func (cycle3TypedLegacyFallback) MarshalJSONTo(*jsontext.Encoder) error { return errors.ErrUnsupported }
func (cycle3TypedLegacyFallback) MarshalJSON() ([]byte, error) {
	return []byte(`{"id":"4111111111111111","amount":"4111111111111111.50"}`), nil
}

func TestModelCycle3StreamingDelegationRetainsReachedNativeProvenance(t *testing.T) {
	id := cycle3LiteralID
	m := cycle3FinancialAmount(t)
	pan, masked := "PAN 4111-1111-1111-1111", "PAN •••• 1111"
	account := Account{ID: id, Name: pan, Balance: m, Kind: "ctaccount"}
	card := Card{ID: id, Name: pan, Balance: m, AccountID: &id, AccountBalance: m}
	wantAccount := account
	wantAccount.Name = masked
	wantCard := card
	wantCard.Name = masked
	raw := Products{Accounts: []Account{account}, Cards: []Card{card}}
	wantRaw := Products{Accounts: []Account{wantAccount}, Cards: []Card{wantCard}}
	requester := &cycle3SnapshotRequester{Secret: "synthetic-private-\xff"}
	p := NewBankPortfolio(raw, requester)
	a, c := p.Accounts()[0], p.Cards()[0]
	for _, tc := range []struct {
		name        string
		input, want any
	}{
		{"raw_account", account, wantAccount}, {"raw_card", card, wantCard}, {"raw_products", raw, wantRaw},
		{"bound_account_pointer", a, wantAccount}, {"bound_account_value", *a, wantAccount},
		{"bound_card_pointer", c, wantCard}, {"bound_card_value", *c, wantCard},
		{"bound_portfolio_pointer", p, wantRaw}, {"bound_portfolio_value", *p, wantRaw},
		{"slice", []any{a, c, p}, []any{wantAccount, wantCard, wantRaw}},
		{"map", map[string]any{"a": a, "c": c, "p": p}, map[string]any{"a": wantAccount, "c": wantCard, "p": wantRaw}},
		{"array", [1]BankCard{*c}, []any{wantCard}},
		{"addressable_field", &struct {
			Card BankCard `json:"card"`
		}{*c}, map[string]any{"card": wantCard}},
		{"operation", Operation{ID: id, Amount: m, FromResource: &Resource{Type: "card", ID: id}, ScopeCardIDs: []string{id}}, Operation{ID: id, Amount: m, FromResource: &Resource{Type: "card", ID: id}, ScopeCardIDs: []string{id}}},
		{"native_credential_union", OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: cycle3ForeignLegacyEntity{BankAccount: *a, Password: "synthetic-private-\xff"}}, {Value: m}}}, OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: map[string]any{"id": "<foreign-redacted>", "amount": "•••• 1111.50"}}, {Value: m}}}},
		{"native_stream_credential_union", OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: &cycle3ForeignStreamEntity{BankCard: *c, Password: "synthetic-private-\xff"}}, {Value: m}}}, OperationDetail{UOHID: id, Fields: []OperationDetailField{{Value: map[string]any{"id": "<foreign-stream-redacted>", "amount": "•••• 1111.50"}}, {Value: m}}}},
		{"nil_entity_pointer", (*BankCard)(nil), nil},
		{"foreign_entity_redactor", cycle3ForeignLegacyEntity{BankAccount: *a, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-redacted>", "amount": "•••• 1111.50"}},
		{"foreign_stream_redactor", &cycle3ForeignStreamEntity{BankCard: *c, Password: "synthetic-private-\xff"}, map[string]any{"id": "<foreign-stream-redacted>", "amount": "•••• 1111.50"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, shape := range []string{"delegated", "unsupported_native_fallback"} {
				t.Run(shape, func(t *testing.T) {
					var input, expected any
					if shape == "delegated" {
						input = cycle3TypedStreamContainer{Payload: tc.input, Secret: "synthetic-private-\xff"}
						expected = map[string]any{"native/~snapshot": tc.want, "raw": map[string]any{"id": "•••• 1111", "amount": "•••• 1111.50", "n": json.Number("9007199254740993.0100"), "tiny": json.Number("1.2300e-1000")}}
					} else {
						input = cycle3TypedStreamFallback{Payload: tc.input, Display: pan}
						expected = map[string]any{"payload": tc.want, "display": masked}
					}
					cycle3AssertGenericJSON(t, input, expected)
				})
			}
		})
	}
	// Foreign explicit legacy JSON wins on sentinel fallback, not its native
	// payload. Neither financial field names nor the hidden payload exempt it.
	cycle3AssertGenericJSON(t, cycle3TypedLegacyFallback{Payload: account}, map[string]any{"id": "•••• 1111", "amount": "•••• 1111.50"})
	if requester.MarshalCalls != 0 || !reflect.DeepEqual(p.Raw(), raw) {
		t.Fatal("stream provenance traversed requester/mutated owned financial snapshots")
	}
}

func TestModelCycle3StreamingNativeTextIsValidatedBeforeLoss(t *testing.T) {
	for _, invalid := range []string{"synthetic_native_\xff", "synthetic_native_\xfe"} {
		id := invalid
		for _, payload := range []any{Account{ID: invalid}, Card{AccountID: &id}, Operation{ScopeCardIDs: []string{invalid}}, Resource{ID: invalid}, NewBankAccount(Account{ID: invalid}, nil, nil), NewBankCard(Card{ID: invalid}, nil, nil), NewBankPortfolio(Products{Cards: []Card{{ID: invalid}}}, nil)} {
			for _, input := range []any{cycle3TypedStreamContainer{Payload: payload}, cycle3TypedStreamFallback{Payload: payload}} {
				out, err := ExportJSON(input)
				var pe *ParseError
				if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || errors.Unwrap(err) != nil {
					t.Fatalf("stream repaired native identity or retained cause: %T %s %v", input, out, err)
				}
				normalized, err := JSONable(input)
				if normalized != nil || !errors.As(err, &pe) {
					t.Fatal("streaming struct incorrectly returned deferred validity")
				}
			}
		}
	}
	for _, payload := range []any{Account{ID: "literal-�"}, NewBankCard(Card{ID: "literal-�"}, nil, nil), Products{Accounts: []Account{{ID: "literal-�"}}}} {
		cycle3AssertGenericJSON(t, cycle3TypedStreamFallback{Payload: payload}, map[string]any{"payload": payload, "display": ""})
	}
}

// Synthetic credentials: pointer-only redaction must run on the addressable
// original field/element, not the plain reflected struct or a copied value.
func TestModelReviewAddressablePointerJSONMarshaler(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"struct_field", &struct {
			Credentials pointerOnlySecret `json:"credentials"`
		}{Credentials: pointerOnlySecret{Password: "fixture-private-password"}}},
		{"slice_element", []pointerOnlySecret{{Password: "fixture-private-password"}}},
		{"array_element", &[1]pointerOnlySecret{{Password: "fixture-private-password"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			standard, err := json.Marshal(tc.value)
			if err != nil || strings.Contains(string(standard), "fixture-private-password") {
				t.Fatalf("invalid synthetic redaction control: %s %v", standard, err)
			}
			data, err := ExportJSON(tc.value)
			if err != nil || strings.Contains(string(data), "fixture-private-password") || !strings.Contains(string(data), "<redacted>") {
				t.Fatalf("addressable pointer marshaler bypass/leak: %s %v", data, err)
			}
		})
	}
	// Typed financial values must still bypass generic string PAN masking.
	amount, err := ParseDecimal("4111111111111111.00")
	if err != nil {
		t.Fatal(err)
	}
	financial := &struct {
		Money   Money   `json:"money"`
		Decimal Decimal `json:"decimal"`
	}{Money: Money{Amount: amount, Currency: "RUB"}, Decimal: amount}
	data, err := ExportJSON(financial)
	if err != nil || strings.Count(string(data), "4111111111111111.00") != 2 || strings.Contains(string(data), "••••") {
		t.Fatalf("typed exact money was masked: %s %v", data, err)
	}
}

func TestModelReviewJSONNumberTokenGrammar(t *testing.T) {
	for _, text := range []string{`"fixture-private-number"`, `null`, `true`, `false`, `{}`, `[]`, ``, `+1`, `01`, `-01`, `.5`, `1.`, `1_000`, `１２`, `1,25`, ` 1`, "1\n", `1 2`, `NaN`, `Infinity`} {
		t.Run(text, func(t *testing.T) {
			_, err := JSONable(json.Number(text))
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
				t.Fatalf("nonnumeric JSON token accepted or unsafe error for %q: %v", text, err)
			}
			data, err := ExportJSON(json.Number(text))
			if data != nil || !errors.As(err, &pe) || strings.Contains(err.Error(), "fixture-private-number") {
				t.Fatalf("unsafe numeric export: %s %v", data, err)
			}
		})
	}
	for _, text := range []string{"0", "-0.00", "9007199254740993.0100", "1.2300e+1000", "-1E-1000", "4111111111111111"} {
		t.Run("valid_"+text, func(t *testing.T) {
			value, err := JSONable(json.Number(text))
			if err != nil || value != json.Number(text) {
				t.Fatalf("valid numeric lexeme lost: %v %v", value, err)
			}
			data, err := ExportJSON(json.Number(text))
			if err != nil || string(data) != text {
				t.Fatalf("numeric lexeme changed: %s %v; want %s", data, err, text)
			}
		})
	}
}

// This finite native scalar reaches the real Go 1.27 encoder after JSONable.
// A streaming marshaler's private error must not escape the export boundary.
type modelReviewEncoderError float64

func (modelReviewEncoderError) MarshalJSONTo(*jsontext.Encoder) error {
	return errors.New("fixture-private-encoder-error")
}

func TestModelReviewExportEncoderErrorSanitized(t *testing.T) {
	value := modelReviewEncoderError(1.25)
	if _, err := JSONable(value); err != nil {
		t.Fatalf("synthetic scalar did not reach encoder: %v", err)
	}
	if _, err := json.Marshal(value); err == nil || !strings.Contains(err.Error(), "fixture-private-encoder-error") {
		t.Fatalf("synthetic encoder failure control missing: %v", err)
	}
	data, err := ExportJSON(value)
	var pe *ParseError
	if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
		t.Fatalf("encoder diagnostic leak: %s %v", data, err)
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("private encoder cause retained")
	}
}

type modelReviewCustomJSON struct{ data []byte }

func (m modelReviewCustomJSON) MarshalJSON() ([]byte, error) { return m.data, nil }

func TestModelReviewCustomJSONStrictFullDocument(t *testing.T) {
	invalidUTF8 := append([]byte(`{"id":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`"}`)...)
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"trailing_object", []byte(`{"safe":true}{"discarded":"fixture-private-trailing"}`)},
		{"trailing_scalar", []byte(`{"safe":true}false`)},
		{"decoded_duplicate", []byte(`{"safe":false,"\u0073afe":true}`)},
		{"nested_duplicate", []byte(`{"nested":{"x":1,"x":2}}`)},
		{"invalid_utf8", invalidUTF8},
		{"unpaired_high", []byte(`{"id":"\ud800"}`)},
		{"unpaired_low", []byte(`{"id":"\udfff"}`)},
		{"reversed_pair", []byte(`{"id":"\udfff\ud800"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := modelReviewCustomJSON{data: tc.data}
			_, err := JSONable(value)
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
				t.Fatalf("invalid custom JSON document accepted: %s, error=%v", tc.name, err)
			}
			data, err := ExportJSON(value)
			if data != nil || !errors.As(err, &pe) || errors.Unwrap(err) != nil {
				t.Fatalf("invalid custom export/cause retained: %s %v", data, err)
			}
		})
	}
	for _, text := range []string{` {"id":"\ud83d\ude00","literal":"�","n":9007199254740993.0100} `, `[{"x":1},{"x":2}]`, `true`, `null`, `1.2300e+1000`, `"safe"`} {
		t.Run("valid_"+text, func(t *testing.T) {
			data, err := ExportJSON(modelReviewCustomJSON{data: []byte(text)})
			if err != nil || !json.Valid(data) {
				t.Fatalf("valid complete custom JSON rejected: %s %v", data, err)
			}
			if strings.Contains(text, "9007199254740993.0100") && !strings.Contains(string(data), "9007199254740993.0100") {
				t.Fatalf("custom numeric lexeme lost: %s", data)
			}
		})
	}
}

func TestModelReviewMoneyJSONRejectsDecodedDuplicateProperties(t *testing.T) {
	amount, err := ParseDecimal("7.2500")
	if err != nil {
		t.Fatal(err)
	}
	original := Money{Amount: amount, Currency: "USD"}
	for _, text := range []string{
		`{"amount":true,"amount":"1.00","currency":"RUB"}`,
		`{"amount":"NaN","amount":"1.00","currency":"RUB"}`,
		`{"amount":null,"\u0061mount":"1.00"}`,
		`{"amount":"1.00","amount":"1.00"}`,
		`{"amount":"1.00","currency":"USD","\u0063urrency":"RUB"}`,
		`{"amount":"1.00","currency":{"code":"USD","\u0063ode":"RUB"}}`,
		`{"amount":"1.00","extra":[{"id":1,"id":2}]}`,
	} {
		t.Run(text, func(t *testing.T) {
			for _, unmarshal := range []func([]byte, *Money) error{
				func(data []byte, m *Money) error { return m.UnmarshalJSON(data) },
				func(data []byte, m *Money) error { return json.Unmarshal(data, m) },
			} {
				got := original
				err := unmarshal([]byte(text), &got)
				var pe *ParseError
				if !errors.As(err, &pe) || pe.Field() != "money" || err.Error() != "sber: invalid domain data" {
					t.Fatalf("decoded duplicate Money property accepted: %s, err=%v", text, err)
				}
				if got != original {
					t.Fatalf("failed Money decode mutated receiver: %v", got)
				}
			}
		})
	}
	var got Money
	if err := json.Unmarshal([]byte(`{"amount":9007199254740993.0100,"currency":{"code":"\ud83d\ude00"},"extra":[{"id":1},{"id":2}]}`), &got); err != nil || got.Amount.String() != "9007199254740993.0100" || got.Currency != "😀" {
		t.Fatalf("valid exact Money or Unicode pair lost: %v %v", got, err)
	}
}

func TestModelReviewExplicitMalformedMoneyObject(t *testing.T) {
	for _, text := range []string{`true`, `false`, `"NaN"`, `"not-money"`, `[]`, `[1]`, `0`, `1.25`} {
		t.Run(text, func(t *testing.T) {
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			var value any
			if err := decoder.Decode(&value); err != nil {
				t.Fatalf("invalid synthetic JSON fixture: %v", err)
			}
			money, err := ParseMoney(value)
			var pe *ParseError
			if money != nil || !errors.As(err, &pe) || pe.Field() != "money" {
				t.Errorf("explicit malformed money object accepted: %s -> %v %v", text, money, err)
			}
			core := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"fixture","operationAmount":`+text+`}]}}`)
			if operations, err := ParseOperations(core); operations != nil || !errors.As(err, &pe) {
				t.Errorf("explicit malformed core money accepted: %s -> %v %v", text, operations, err)
			}
			// The soft optional metadata path must still tolerate the same shapes.
			optional := mustDecodeDomain(t, `{"body":{"operations":[{"uohId":"fixture","operationAmount":{"amount":1.2500,"currency":"RUB"},"billingAmount":`+text+`,"nationalAmount":`+text+`,"commission":`+text+`,"tips":`+text+`}]}}`)
			operations, err := ParseOperations(optional)
			if err != nil || len(operations) != 1 {
				t.Fatalf("optional metadata discarded valid operation: %v %v", operations, err)
			}
			op := operations[0]
			if op.Amount == nil || op.Amount.Amount.String() != "1.2500" || op.BillingAmount != nil || op.NationalAmount != nil || op.Commission != nil || op.Tips != nil {
				t.Fatalf("strict/soft money paths conflated: %v", op)
			}
		})
	}
	// Nil objects and absent amount keys stay absent; no balances are fabricated.
	for _, value := range []any{nil, map[string]any(nil), map[string]any{}, map[string]any{"currency": "RUB"}} {
		if money, err := ParseMoney(value); err != nil || money != nil {
			t.Fatalf("absent money was fabricated or rejected: %v %v", money, err)
		}
	}
}

func TestModelReviewAccountDefaultConstruction(t *testing.T) {
	amount, err := ParseDecimal("9007199254740993.0100")
	if err != nil {
		t.Fatal(err)
	}
	balance := &Money{Amount: amount, Currency: "RUB"}
	for _, input := range []Account{
		{},
		{ID: "fixture-account", Name: "fixture", Last4: "1234", State: "OPEN", Hidden: true, Arrested: true, Balance: balance},
		{ID: "fixture-savings", Balance: nil, Kind: "account"},
	} {
		// Interface discovery lets the initial missing construction path fail at
		// runtime, without a placeholder constructor or a compiler-only RED.
		factory, ok := any(input).(interface{ WithDefaults() Account })
		if !ok {
			t.Fatal("source-compatible Account default construction path missing")
		}
		got := factory.WithDefaults()
		want := input
		if want.Kind == "" {
			want.Kind = "ctaccount" // models.py:Account.kind source default.
		}
		if got != want || got.Balance != input.Balance {
			t.Fatalf("Account defaults changed supplied fields/IDs/balance: got=%v want=%v", got, want)
		}
		if input.Kind == "" && got.Kind != "ctaccount" {
			t.Fatalf("wrong source Account default: %q", got.Kind)
		}
	}
}

func TestModelReviewDecimalJSONNumberTokenGrammar(t *testing.T) {
	for _, text := range []string{"+1", "01", "-01", ".5", "1.", "1_000", "１２.５０", "1,25", " 1", "1\n"} {
		t.Run(text, func(t *testing.T) {
			// Source-compatible decimal strings are still allowed; a json.Number
			// explicitly claims to be a JSON number lexeme, not that string syntax.
			if _, err := ParseDecimal(text); err != nil {
				t.Fatalf("synthetic decimal string control rejected: %v", err)
			}
			_, err := ParseDecimal(json.Number(text))
			var pe *ParseError
			if !errors.As(err, &pe) || pe.Field() != "amount" {
				t.Fatalf("invalid numeric token normalized into financial amount: %q %v", text, err)
			}
		})
	}
	for _, text := range []string{"0", "-0.00", "9007199254740993.0100", "1.25e-2"} {
		fromNumber, err := ParseDecimal(json.Number(text))
		fromString, stringErr := ParseDecimal(text)
		if err != nil || stringErr != nil || fromNumber != fromString {
			t.Fatalf("valid numeric token lost: %v %v", fromNumber, err)
		}
	}
}

// Post-GREEN boundary exploration uses independent strict document/decoder
// checks, not the production numeric regex, as the acceptance oracle.
func FuzzModelReviewJSONNumberGate(f *testing.F) {
	for _, text := range []string{"0", "-0.00", "9007199254740993.0100", "1e+1000", `"fixture-private-number"`, "null", " 1", "01", "+1", "１２"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		valid := false
		if strictjson.Validate([]byte(text)) == nil {
			decoder := json.NewDecoder(strings.NewReader(text))
			decoder.UseNumber()
			var decoded any
			if err := decoder.Decode(&decoded); err == nil {
				number, ok := decoded.(json.Number)
				valid = ok && string(number) == text
			}
		}
		data, err := ExportJSON(json.Number(text))
		if valid {
			if err != nil || string(data) != text {
				t.Fatalf("valid token changed: %q %q %v", text, data, err)
			}
			return
		}
		var pe *ParseError
		if data != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
			t.Fatalf("invalid token accepted or unsafe error: %q %v", data, err)
		}
	})
}

func FuzzModelReviewCustomDocumentGate(f *testing.F) {
	for _, data := range [][]byte{
		[]byte(`{"safe":true}`), []byte(`{"safe":true}{}`),
		[]byte(`{"x":1,"\u0078":2}`), []byte(`{"id":"\ud800"}`),
		[]byte(`{"id":"\ud83d\ude00","amount":9007199254740993.0100}`),
		[]byte(`[{"id":1},{"id":2}]`), {'"', 0xff, '"'},
	} {
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		valid := strictjson.Validate(data) == nil
		out, err := ExportJSON(modelReviewCustomJSON{data: data})
		if valid {
			if err != nil || strictjson.Validate(out) != nil {
				t.Fatalf("valid full custom document rejected: %v", err)
			}
			return
		}
		var pe *ParseError
		if out != nil || !errors.As(err, &pe) || pe.Field() != "json_export" || err.Error() != "sber: invalid domain data" {
			t.Fatalf("invalid full custom document accepted or unsafe error: %q %v", out, err)
		}
	})
}

// Synthetic, native-only integrity regression. No bank or credentials.
// Original invalid UTF-8 must be rejected before an owned custom serializer
// hands repaired bytes to the generic walker; distinct IDs must not merge.
func TestIndependentOwnedCustomMarshalRejectsNativeIdentityRepair(t *testing.T) {
	for _, kind := range []string{"account", "card", "portfolio"} {
		t.Run(kind, func(t *testing.T) {
			var native any
			var raw any
			var explicit func() ([]byte, error)
			switch kind {
			case "account":
				v := NewBankAccount(Account{ID: "synthetic_A\xff", Kind: "ctaccount"}, &independentRequester{}, nil)
				native = v
				raw = v.Snapshot()
				explicit = v.ExportJSON
			case "card":
				v := NewBankCard(Card{ID: "synthetic_A\xff"}, &independentRequester{}, nil)
				native = v
				raw = v.Snapshot()
				explicit = v.ExportJSON
			case "portfolio":
				v := NewBankPortfolio(Products{Accounts: []Account{{ID: "synthetic_parent", Kind: "ctaccount"}}, Cards: []Card{{ID: "synthetic_A\xff"}, {ID: "synthetic_A\xfe"}}}, &independentRequester{})
				native = v
				raw = v.Raw()
				explicit = v.ExportJSON
			}
			if _, err := explicit(); err == nil {
				t.Fatal("explicit export control did not reject")
			}
			direct, directErr := native.(json.Marshaler).MarshalJSON()
			data, err := json.Marshal(native)
			t.Logf("owned MarshalJSON kind=%s direct=%s directErr=%v returned=%s error=%v raw=%#v", kind, direct, directErr, data, err, raw)
			if err == nil || directErr == nil {
				t.Fatalf("owned custom serializer silently repaired distinct invalid UTF-8 IDs to U+FFFD: %s", data)
			}
		})
	}
}
func TestIndependentOwnedCustomMarshalValidReplacementCharacterControl(t *testing.T) {
	raw := Card{ID: "synthetic_A�"}
	v := NewBankCard(raw, &independentRequester{}, nil)
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Card
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, raw) || v.ID() != raw.ID {
		t.Fatal("valid literal replacement character changed")
	}
}
