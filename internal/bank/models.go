// Domain contracts ported from the MIT-licensed sber-mcp reference.
package bank

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// Account is a parsed account snapshot; Kind distinguishes current from savings.
// Go domain wrappers are mutable, unlike the source's frozen Python dataclasses;
// pointers and slices are not made immutable by copying a containing model.
type Account struct {
	ID       string `json:"id" sber:"literal"`
	Name     string `json:"name"`
	Last4    string `json:"last4"`
	State    string `json:"state"`
	Hidden   bool   `json:"hidden"`
	Arrested bool   `json:"arrested"`
	Balance  *Money `json:"balance"`
	Kind     string `json:"kind" sber:"literal"`
}

// WithDefaults is the explicit source-compatible Account construction path.
// It returns a shallow copy, defaulting an empty Kind to "ctaccount" and leaving
// every other field intact. It does not validate or invent IDs or balances.
// Direct struct literals retain Go zero-value semantics; Balance is not cloned.
func (a Account) WithDefaults() Account {
	if a.Kind == "" {
		a.Kind = "ctaccount"
	}
	return a
}

// Card stores only the last four PAN digits, not the card/account number.
type Card struct {
	ID             string  `json:"id" sber:"literal"`
	Name           string  `json:"name"`
	Last4          string  `json:"last4"`
	Type           string  `json:"type"`
	State          string  `json:"state"`
	Hidden         bool    `json:"hidden"`
	Arrested       bool    `json:"arrested"`
	IsMain         bool    `json:"is_main"`
	Balance        *Money  `json:"balance"`
	BalanceSource  *string `json:"balance_source" sber:"literal"`
	AccountID      *string `json:"account_id" sber:"literal"`
	AccountBalance *Money  `json:"account_balance"`
}

// Resource identifies an operation endpoint, not a full card number.
type Resource struct {
	Type string `json:"type" sber:"literal"`
	ID   string `json:"id" sber:"literal"`
}

// Operation keeps transaction amounts and post-operation balances distinct.
type Operation struct {
	ID                       string    `json:"id" sber:"literal"`
	Date                     string    `json:"date" sber:"literal"`
	Form                     string    `json:"form"`
	Type                     string    `json:"type"`
	ClassificationCode       string    `json:"classification_code" sber:"literal"`
	CreationChannel          string    `json:"creation_channel"`
	State                    string    `json:"state"`
	StateName                string    `json:"state_name"`
	StateDescription         string    `json:"state_description"`
	Merchant                 string    `json:"merchant"`
	Description              string    `json:"description"`
	Amount                   *Money    `json:"amount"`
	BillingAmount            *Money    `json:"billing_amount"`
	NationalAmount           *Money    `json:"national_amount"`
	Commission               *Money    `json:"commission"`
	Tips                     *Money    `json:"tips"`
	RefusalReason            string    `json:"refusal_reason"`
	FromResource             *Resource `json:"from_resource"`
	ToResource               *Resource `json:"to_resource"`
	ScopeCardIDs             []string  `json:"scope_card_ids" sber:"literal"`
	IsFinancial              *bool     `json:"is_financial"`
	IsHidden                 bool      `json:"is_hidden"`
	BalanceAfter             *Money    `json:"balance_after"`
	BalanceAfterResourceID   *string   `json:"balance_after_resource_id" sber:"literal"`
	BalanceAfterResourceName *string   `json:"balance_after_resource_name"`
}

// CardLedgerEntry is one scoped/direct card view of a transaction.
type CardLedgerEntry struct {
	OperationID string `json:"operation_id" sber:"literal"`
	CardID      string `json:"card_id" sber:"literal"`
	Direction   string `json:"direction" sber:"literal"`
	Amount      *Money `json:"amount"`
}

// OperationDetailField.Value is a *Money, string, or nil (source union).
type OperationDetailField struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// UnmarshalJSON preserves the native field union on financial export roundtrips.
func (f *OperationDetailField) UnmarshalJSON(data []byte) error {
	// Validate the complete original union document before decoding can replace
	// malformed Unicode or overwrite duplicate names (including unknown fields).
	if strictjson.Validate(data) != nil {
		return NewParseError("detail_field")
	}
	var raw struct {
		Name  string          `json:"name"`
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return NewParseError("detail_field")
	}
	var value any
	if len(raw.Value) != 0 && string(raw.Value) != "null" {
		dec := json.NewDecoder(bytes.NewReader(raw.Value))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return NewParseError("detail_field")
		}
		switch x := v.(type) {
		case string:
			value = x
		case map[string]any:
			m, err := ParseMoney(x)
			if err != nil {
				return err
			}
			value = m
		default:
			return NewParseError("detail_field")
		}
	}
	*f = OperationDetailField{Name: raw.Name, Type: raw.Type, Value: value}
	return nil
}

type OperationDetail struct {
	UOHID              string                 `json:"uoh_id" sber:"literal"`
	Form               string                 `json:"form"`
	Title              string                 `json:"title"`
	Amount             *Money                 `json:"amount"`
	State              string                 `json:"state"`
	StateName          string                 `json:"state_name"`
	StateDescription   string                 `json:"state_description"`
	StatementAvailable bool                   `json:"statement_available"`
	Fields             []OperationDetailField `json:"fields"`
}

type CardLimits struct {
	Purchase       *Money `json:"purchase"`
	Available      *Money `json:"available"`
	AvailableTotal *Money `json:"available_total"`
}

type CreditInfo struct {
	Limit          *Money `json:"limit"`
	OwnSum         *Money `json:"own_sum"`
	Debt           *Money `json:"debt"`
	MinPayment     *Money `json:"min_payment"`
	MinPaymentDate string `json:"min_payment_date"`
}

type CardInfo struct {
	ID            string      `json:"id" sber:"literal"`
	Name          string      `json:"name"`
	Last4         string      `json:"last4"`
	State         string      `json:"state"`
	CardHolder    string      `json:"card_holder"`
	PaySystemType string      `json:"pay_system_type"`
	ExpireDate    string      `json:"expire_date"`
	Limits        CardLimits  `json:"limits"`
	Credit        *CreditInfo `json:"credit"`
}

type CategoryAmount struct {
	ID              string `json:"id" sber:"literal"`
	Name            string `json:"name"`
	ExternalID      string `json:"external_id"`
	NationalAmount  *Money `json:"national_amount"`
	VisibleAmount   *Money `json:"visible_amount"`
	CountOperations int    `json:"count_operations"`
}

type PFMPeriod struct {
	From           string           `json:"from_"`
	To             string           `json:"to"`
	IncomeType     string           `json:"income_type"`
	NationalAmount *Money           `json:"national_amount"`
	VisibleAmount  *Money           `json:"visible_amount"`
	Categories     []CategoryAmount `json:"categories"`
}

type PFMAmounts struct {
	Periods []PFMPeriod `json:"periods"`
}

// TimeFilter has inclusive, Moscow-aware bounds. Date-only upper bounds use
// 23:59:59 to match the source API's second-resolution date contract.
type TimeFilter struct {
	From *time.Time `json:"from"`
	To   *time.Time `json:"to"`
}

type OperationsPage struct {
	Operations []Operation `json:"operations"`
	NextOffset *int        `json:"next_offset"`
}

// Products is a snapshot of all three account sections and wallet cards.
type Products struct {
	Accounts []Account `json:"accounts"`
	Cards    []Card    `json:"cards"`
}
