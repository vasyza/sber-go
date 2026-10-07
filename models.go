// Domain models ported from the MIT-licensed sber-mcp reference.
package sber

import (
	"bytes"
	"encoding"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/vasyza/sber-go/internal/strictjson"
)

// ParseError denotes an unusable response, never evidence of an empty history.
// Field() explicitly reads the schema location; no input values or error causes
// are retained. The private immutable string indirection also protects fmt paths
// that bypass formatting methods. Native zero and nil pointers read empty.
type ParseError struct{ field *string }

// NewParseError retains the supplied schema location for explicit Field reads.
// Its ordinary error text/JSON are static; it never wraps a private cause.
func NewParseError(field string) *ParseError {
	if field == "" {
		return &ParseError{}
	}
	return &ParseError{field: &field}
}

// Field is a deliberate raw schema-location read, not a logging representation.
func (e *ParseError) Field() string {
	if e == nil || e.field == nil {
		return ""
	}
	return *e.field
}

func (e *ParseError) Error() string                 { return "sber: invalid domain data" }
func (e *ParseError) sberError()                    {}
func (e *ParseError) String() string                { return e.Error() }
func (e *ParseError) GoString() string              { return e.Error() }
func (e *ParseError) Format(f fmt.State, verb rune) { domainSafeFormat(f, e.Error()) }
func (e *ParseError) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Error string `json:"error"`
	}{"parse_error"})
}

// Decimal is an immutable base-ten value. Its zero value is zero. It preserves
// fractional scale (10.50), without binary-floating-point arithmetic.
type Decimal struct {
	digits   string
	exponent int
	negative bool
}

var decimalPattern = regexp.MustCompile(`^([+-]?)([0-9]*)(?:\.([0-9]*))?(?:[eE]([+-]?[0-9]+))?$`)

// JSON number lexemes are stricter than source-compatible decimal strings:
// no surrounding whitespace, leading plus/zeros, Unicode digits or separators.
var domainJSONNumberPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// ParseDecimal accepts decimal strings (including a comma decimal separator),
// validated JSON number lexemes and native numbers. String normalization does
// not apply to json.Number. Decode JSON with DecodeJSON, not float64, when the
// original numeric lexeme must be preserved.
func ParseDecimal(value any) (Decimal, error) {
	var text string
	switch v := value.(type) {
	case Decimal:
		return v, nil
	case *Decimal:
		if v == nil {
			return Decimal{}, NewParseError("amount")
		}
		return *v, nil
	case string:
		text = v
	case json.Number:
		text = string(v)
		if !domainJSONNumberPattern.MatchString(text) {
			return Decimal{}, NewParseError("amount")
		}
	case int:
		text = strconv.FormatInt(int64(v), 10)
	case int8:
		text = strconv.FormatInt(int64(v), 10)
	case int16:
		text = strconv.FormatInt(int64(v), 10)
	case int32:
		text = strconv.FormatInt(int64(v), 10)
	case int64:
		text = strconv.FormatInt(v, 10)
	case uint:
		text = strconv.FormatUint(uint64(v), 10)
	case uint8:
		text = strconv.FormatUint(uint64(v), 10)
	case uint16:
		text = strconv.FormatUint(uint64(v), 10)
	case uint32:
		text = strconv.FormatUint(uint64(v), 10)
	case uint64:
		text = strconv.FormatUint(v, 10)
	case float32:
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return Decimal{}, NewParseError("amount")
		}
		text = strconv.FormatFloat(float64(v), 'g', -1, 32)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return Decimal{}, NewParseError("amount")
		}
		text = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		return Decimal{}, NewParseError("amount")
	}
	text = strings.Map(func(r rune) rune {
		if r == '_' {
			return -1
		}
		if r == ',' {
			return '.'
		}
		if n, ok := domainDigitValue(r); ok {
			return rune('0' + n)
		}
		return r
	}, strings.TrimSpace(text))
	m := decimalPattern.FindStringSubmatch(text)
	if m == nil || m[2]+m[3] == "" {
		return Decimal{}, NewParseError("amount")
	}
	exp := 0
	if m[4] != "" {
		n, err := strconv.Atoi(m[4])
		if err != nil {
			return Decimal{}, NewParseError("amount")
		}
		exp = n
	}
	// An overflowing exponent must not wrap into a different monetary value.
	if exp < -int(^uint(0)>>1)+len(m[3]) {
		return Decimal{}, NewParseError("amount")
	}
	exp -= len(m[3])
	digits := strings.TrimLeft(m[2]+m[3], "0")
	if digits == "" {
		digits = "0"
	}
	if exp > int(^uint(0)>>1)-len(digits) {
		return Decimal{}, NewParseError("amount")
	}
	return Decimal{digits: digits, exponent: exp, negative: m[1] == "-"}, nil
}

func (d Decimal) String() string {
	digits := d.digits
	if digits == "" {
		digits = "0"
	}
	sign := ""
	if d.negative {
		sign = "-"
	}
	adjusted := d.exponent + len(digits) - 1
	if d.exponent > 0 || adjusted < -6 {
		mantissa := digits[:1]
		if len(digits) > 1 {
			mantissa += "." + digits[1:]
		}
		e := strconv.Itoa(adjusted)
		if adjusted >= 0 {
			e = "+" + e
		}
		return sign + mantissa + "E" + e
	}
	point := len(digits) + d.exponent
	if point <= 0 {
		return sign + "0." + strings.Repeat("0", -point) + digits
	}
	if point < len(digits) {
		return sign + digits[:point] + "." + digits[point:]
	}
	return sign + digits
}
func (d Decimal) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }
func (d *Decimal) UnmarshalJSON(data []byte) error {
	if !json.Valid(data) {
		return NewParseError("amount")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return NewParseError("amount")
	}
	parsed, err := ParseDecimal(v)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
func (d Decimal) GoString() string { return d.String() }

// Sign returns -1, 0 or +1 without converting to floating point.
func (d Decimal) Sign() int {
	if d.digits == "" || d.digits == "0" {
		return 0
	}
	if d.negative {
		return -1
	}
	return 1
}
func (d Decimal) Abs() Decimal { d.negative = false; return d }
func (d Decimal) Neg() Decimal { d.negative = !d.negative; return d }

// Money deliberately exports financial data as exact decimal strings. It does
// not contain credentials and is not replaced by an unusable redacted blob.
// The wrapper is mutable Go data, unlike Python's frozen Money dataclass;
// Decimal itself has immutable, unexported base-ten components.
type Money struct {
	Amount   Decimal `json:"amount"`
	Currency string  `json:"currency"`
}

func (m *Money) UnmarshalJSON(data []byte) error {
	// Validate before decoding: maps otherwise overwrite duplicate evidence,
	// and encoding/json can silently replace malformed Unicode identifiers.
	if strictjson.Validate(data) != nil {
		return NewParseError("money")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return NewParseError("money")
	}
	if raw == nil {
		return NewParseError("money")
	}
	value, err := ParseMoney(raw)
	if err != nil {
		return err
	}
	if value == nil {
		return NewParseError("amount")
	}
	*m = *value
	return nil
}

// ParseMoney returns nil for nil money objects or an absent amount key. A
// nonnil value that is not a JSON object, or an explicit invalid/null amount,
// is a hard error. Optional parser metadata must use its separate soft path.
// currencyCode:null falls through to currency.code.
func ParseMoney(value any) (*Money, error) {
	if value == nil {
		return nil, nil
	}
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, NewParseError("money")
	}
	if raw == nil {
		return nil, nil
	}
	amount, present := raw["amount"]
	if !present {
		return nil, nil
	}
	d, err := ParseDecimal(amount)
	if err != nil {
		return nil, err
	}
	currency := raw["currencyCode"]
	if !domainTruthy(currency) {
		currency = raw["currency"]
	}
	if obj, ok := currency.(map[string]any); ok {
		currency = obj["code"]
	}
	c, _ := currency.(string)
	return &Money{Amount: d, Currency: c}, nil
}
func domainTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case json.Number:
		d, err := ParseDecimal(x)
		return err != nil || d.Sign() != 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() != 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return rv.Uint() != 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() != 0
	case reflect.Array, reflect.Map, reflect.Slice:
		return rv.Len() != 0
	case reflect.Pointer, reflect.Interface:
		return !rv.IsNil()
	default:
		return true
	}
}

// Decimal's reference parser accepts Unicode decimal digits, not just ASCII.
func domainDigitValue(r rune) (int, bool) {
	if r >= '0' && r <= '9' {
		return int(r - '0'), true
	}
	for _, span := range unicode.Nd.R16 {
		n := uint32(r)
		if n >= uint32(span.Lo) && n <= uint32(span.Hi) && (n-uint32(span.Lo))%uint32(span.Stride) == 0 {
			return int((n - uint32(span.Lo)) / uint32(span.Stride) % 10), true
		}
	}
	for _, span := range unicode.Nd.R32 {
		n := uint32(r)
		if n >= span.Lo && n <= span.Hi && (n-span.Lo)%span.Stride == 0 {
			return int((n - span.Lo) / span.Stride % 10), true
		}
	}
	return 0, false
}

// Keep fmt's ordinary Money formatting useful, like the reference dataclass.
var _ fmt.Stringer = Decimal{}

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

// Transfer models preserve the typed workflow contract without implementing
// transport/mutations. Process/resource/document identifiers are hidden by
// default fmt and JSON, unlike intentionally exportable financial snapshots.
type TransferResource struct{ data **transferResourceData }

type transferResourceData struct{ id, kind, name, currency string }

// NewTransferResource retains all raw resource fields without validation or
// normalization. Copies share immutable identity; accessors expose raw reads.
func NewTransferResource(id, kind, name, currency string) TransferResource {
	if id == "" && kind == "" && name == "" && currency == "" {
		return TransferResource{}
	}
	// A terminal pointer pointee prevents nested fmt badVerb from dereferencing
	// the raw record, including method-inaccessible private wrapper fields.
	data := &transferResourceData{id, kind, name, currency}
	return TransferResource{data: &data}
}

// ID deliberately reads the raw resource identifier.
func (v TransferResource) ID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).id
}

// Kind reads the supplied resource kind.
func (v TransferResource) Kind() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).kind
}

// Name deliberately reads the complete name, without display PAN masking.
func (v TransferResource) Name() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).name
}

// Currency reads the supplied currency without inventing a default.
func (v TransferResource) Currency() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).currency
}

// TransferDraft is immutable. Lists are copied at both construction and read.
type TransferDraft struct{ data **transferDraftData }

type transferDraftData struct {
	pid, flow, state      string
	sources, destinations []TransferResource
}

// NewTransferDraft preserves every workflow field, resource and list ordering.
// Nil lists stay nil; explicit-empty lists stay nonnil. No defaults are invented.
func NewTransferDraft(pid, flow, state string, sources, destinations []TransferResource) TransferDraft {
	if pid == "" && flow == "" && state == "" && sources == nil && destinations == nil {
		return TransferDraft{}
	}
	data := &transferDraftData{pid, flow, state, slices.Clone(sources), slices.Clone(destinations)}
	return TransferDraft{data: &data}
}

// PID deliberately reads the raw transfer process identifier.
func (v TransferDraft) PID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).pid
}

// Flow reads the supplied workflow.
func (v TransferDraft) Flow() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).flow
}

// State reads the supplied state.
func (v TransferDraft) State() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).state
}

// Sources returns a defensive list copy of immutable resource snapshots.
func (v TransferDraft) Sources() []TransferResource {
	if v.data == nil {
		return nil
	}
	return slices.Clone((*v.data).sources)
}

// Destinations returns a defensive list copy of immutable resource snapshots.
func (v TransferDraft) Destinations() []TransferResource {
	if v.data == nil {
		return nil
	}
	return slices.Clone((*v.data).destinations)
}

// PreparedTransfer keeps its exact amount and raw wire fields immutable.
type PreparedTransfer struct{ data **preparedTransferData }

type preparedTransferData struct {
	pid, flow, state, sourceID, destinationID string
	amount                                    Money
	paymentPurpose                            string
}

// NewPreparedTransfer retains every supplied field without validation or
// financial mutation. Money is copied by value; its Decimal is immutable.
func NewPreparedTransfer(pid, flow, state, sourceID, destinationID string, amount Money, paymentPurpose string) PreparedTransfer {
	if pid == "" && flow == "" && state == "" && sourceID == "" && destinationID == "" && amount == (Money{}) && paymentPurpose == "" {
		return PreparedTransfer{}
	}
	data := &preparedTransferData{pid, flow, state, sourceID, destinationID, amount, paymentPurpose}
	return PreparedTransfer{data: &data}
}

// PID deliberately reads the raw transfer process identifier.
func (v PreparedTransfer) PID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).pid
}

// Flow reads the supplied workflow.
func (v PreparedTransfer) Flow() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).flow
}

// State reads the supplied state.
func (v PreparedTransfer) State() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).state
}

// SourceID deliberately reads the raw source resource identifier.
func (v PreparedTransfer) SourceID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).sourceID
}

// DestinationID deliberately reads the raw destination resource identifier.
func (v PreparedTransfer) DestinationID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).destinationID
}

// Amount returns a value copy preserving the exact Decimal, sign, scale and
// currency. Mutating that Money wrapper cannot change this prepared snapshot.
func (v PreparedTransfer) Amount() Money {
	if v.data == nil {
		return Money{}
	}
	return (*v.data).amount
}

// PaymentPurpose deliberately reads the full, unmasked raw purpose.
func (v PreparedTransfer) PaymentPurpose() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).paymentPurpose
}

// TransferResult is an immutable result, including its optional document ID.
type TransferResult struct{ data **transferResultData }

type transferResultData struct {
	pid, flow, state string
	documentID       *string
}

// NewTransferResult retains all workflow fields and defensively copies the
// optional raw document ID. Nil and a present empty ID remain distinct.
func NewTransferResult(pid, flow, state string, documentID *string) TransferResult {
	if pid == "" && flow == "" && state == "" && documentID == nil {
		return TransferResult{}
	}
	data := &transferResultData{pid, flow, state, copyTransferDocumentID(documentID)}
	return TransferResult{data: &data}
}

func copyTransferDocumentID(documentID *string) *string {
	if documentID == nil {
		return nil
	}
	value := *documentID
	return &value
}

// PID deliberately reads the raw transfer process identifier.
func (v TransferResult) PID() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).pid
}

// Flow reads the supplied workflow.
func (v TransferResult) Flow() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).flow
}

// State reads the supplied state.
func (v TransferResult) State() string {
	if v.data == nil {
		return ""
	}
	return (*v.data).state
}

// DocumentID deliberately reads the optional raw ID into a fresh pointer copy.
// Mutating or comparing that pointer cannot mutate/identify the backing ID.
func (v TransferResult) DocumentID() *string {
	if v.data == nil {
		return nil
	}
	return copyTransferDocumentID((*v.data).documentID)
}

func (v TransferResource) String() string {
	return fmt.Sprintf("TransferResource(id=<redacted>, kind=%q, currency=%q)", RedactPAN(v.Kind()), RedactPAN(v.Currency()))
}
func (v TransferDraft) String() string {
	return fmt.Sprintf("TransferDraft(pid=<redacted>, flow=%q, state=%q, sources=%d, destinations=%d)", RedactPAN(v.Flow()), RedactPAN(v.State()), len(v.Sources()), len(v.Destinations()))
}
func (v PreparedTransfer) String() string {
	return fmt.Sprintf("PreparedTransfer(pid=<redacted>, flow=%q, state=%q, amount=<redacted>, currency=%q)", RedactPAN(v.Flow()), RedactPAN(v.State()), RedactPAN(v.Amount().Currency))
}
func (v TransferResult) String() string {
	present := "None"
	if v.DocumentID() != nil {
		present = "<present>"
	}
	return fmt.Sprintf("TransferResult(pid=<redacted>, flow=%q, state=%q, document_id=%s)", RedactPAN(v.Flow()), RedactPAN(v.State()), present)
}
func domainSafeFormat(f fmt.State, text string)          { _, _ = f.Write([]byte(text)) }
func (v TransferResource) Format(f fmt.State, verb rune) { domainSafeFormat(f, v.String()) }
func (v TransferDraft) Format(f fmt.State, verb rune)    { domainSafeFormat(f, v.String()) }
func (v PreparedTransfer) Format(f fmt.State, verb rune) { domainSafeFormat(f, v.String()) }
func (v TransferResult) Format(f fmt.State, verb rune)   { domainSafeFormat(f, v.String()) }
func (v TransferResource) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"id": "<redacted>", "kind": RedactPAN(v.Kind()), "name": "<redacted>", "currency": RedactPAN(v.Currency())})
}
func (v TransferDraft) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"pid": "<redacted>", "flow": RedactPAN(v.Flow()), "state": RedactPAN(v.State()), "sources": len(v.Sources()), "destinations": len(v.Destinations())})
}
func (v PreparedTransfer) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"pid": "<redacted>", "flow": RedactPAN(v.Flow()), "state": RedactPAN(v.State()), "amount": "<redacted>", "currency": RedactPAN(v.Amount().Currency)})
}
func (v TransferResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"pid": "<redacted>", "flow": RedactPAN(v.Flow()), "state": RedactPAN(v.State()), "document_id_present": v.DocumentID() != nil})
}

// JSONable converts native models/containers to the source JSON shape. Decimal
// and Money amounts remain exact strings; typed financial identifiers and codes
// retain literal text, while display strings are PAN-redacted.
// Custom marshalers (including credential-bearing SDK values) retain their
// own default redaction contract rather than being bypassed via reflection.
// Their output must be one full JSON document with valid Unicode and unique
// decoded object keys; numeric lexemes are preserved, not parsed as float64.
// A failing finite streaming float retains the established deferred encoder-
// error contract as a zero-state static failure marker, never its raw receiver.
func JSONable(value any) (any, error) {
	return domainJSONValue(reflect.ValueOf(value), map[domainJSONVisit]bool{})
}

type domainJSONVisit struct {
	typ    reflect.Type
	ptr    uintptr
	length int // Only slices: a shared start address is not a shared view.
}

func domainJSONAddress(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v.Addr()
	}
	return reflect.Value{}
}

// A failed finite streaming scalar used to reach ExportJSON's final encoder.
// Retain that narrow JSONable contract with no source value or private cause.
// Successful normalization never returns the original named primitive.
type domainJSONDeferredError struct{}

func (domainJSONDeferredError) MarshalJSON() ([]byte, error) {
	return nil, NewParseError("json_export")
}

// Per-call streaming provenance contains only already-normalized plain data,
// not a raw receiver, credential implementation or arbitrary source pointer.
// It is consumed by the walker; JSONable never returns this private carrier.
type domainJSONNormalized struct{ value any }

// Use the package entry point, not a direct MarshalJSONTo call: Go handles
// ErrUnsupported fallback, rejects mutation before fallback and enforces a
// singular value. Strict syntax options prevent replacement/duplicate loss
// before our complete-document check sees the output.
func domainJSONStreaming(value any, visits map[domainJSONVisit]bool) (any, error) {
	// Record native financial values only when the package actually marshals
	// those types/schemas (including a streamer's documented native fallback). Raw
	// custom JSON strings do not gain this exemption from display PAN masking.
	financial := map[jsontext.Pointer]any{}
	marshalers := jsonv2.JoinMarshalers(
		jsonv2.MarshalToFunc(func(e *jsontext.Encoder, m Money) error {
			wire := struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			}{m.Amount.String(), m.Currency}
			if err := jsonv2.MarshalEncode(e, wire); err != nil {
				return err
			}
			financial[e.StackPointer()] = m
			return nil
		}),
		jsonv2.MarshalToFunc(func(e *jsontext.Encoder, d Decimal) error {
			if err := e.WriteToken(jsontext.String(d.String())); err != nil {
				return err
			}
			financial[e.StackPointer()] = d
			return nil
		}),
		jsonv2.MarshalToFunc(func(e *jsontext.Encoder, value any) error {
			// Interface hooks receive a nonnil pointer to the concrete value.
			// Inspect only its exact type, without walking foreign fields. The
			// official encoder retains custom-method/fallback precedence for
			// every unadmitted type; return Unsupported without writing a token.
			value = reflect.ValueOf(value).Elem().Interface()
			v := reflect.ValueOf(value)
			if v.Kind() == reflect.Pointer && v.IsNil() {
				return errors.ErrUnsupported
			}
			if snapshot, owned := domainJSONEntitySnapshot(value); owned {
				value = snapshot
			} else if !domainJSONIsNativeFinancial(value) {
				return errors.ErrUnsupported
			}
			out, err := domainJSONValue(reflect.ValueOf(value), visits)
			if err != nil {
				return err
			}
			if err := jsonv2.MarshalEncode(e, out); err != nil {
				return err
			}
			financial[e.StackPointer()] = domainJSONNormalized{out}
			return nil
		}),
	)
	data, err := jsonv2.Marshal(value, json.DefaultOptionsV1(),
		jsontext.AllowInvalidUTF8(false), jsontext.AllowDuplicateNames(false),
		jsonv2.WithMarshalers(marshalers))
	if err != nil || strictjson.Validate(data) != nil {
		return nil, NewParseError("json_export")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, NewParseError("json_export")
	}
	out = domainJSONRestoreFinancial(out, "", financial)
	return domainJSONValue(reflect.ValueOf(out), visits)
}

func domainJSONRestoreFinancial(value any, path jsontext.Pointer, financial map[jsontext.Pointer]any) any {
	if original, ok := financial[path]; ok {
		return original
	}
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = domainJSONRestoreFinancial(item, path.AppendToken(key), financial)
		}
	case []any:
		for i, item := range v {
			v[i] = domainJSONRestoreFinancial(item, path.AppendToken(strconv.Itoa(i)), financial)
		}
	}
	return value
}

func domainJSONMoney(m Money) (any, error) {
	if !utf8.ValidString(m.Currency) {
		return nil, NewParseError("json_export")
	}
	// Currency is source literal financial metadata, not display prose.
	return map[string]any{"amount": m.Amount.String(), "currency": m.Currency}, nil
}

// Resolve authoritative text methods in an actual object-name context, not a
// value context (where JSON methods have different priority). Only an inert map
// value is encoded; custom key implementation fields are never pre-walked.
// Keep source-compatible conversion for comparable keys without text methods.
func domainJSONMapKey(v reflect.Value) (string, error) {
	for v.Kind() == reflect.Interface {
		if v.IsNil() {
			// A nil interface has no key-facing method to call. Retain the
			// source's nil-to-empty name, even for a text-interface map type.
			return "", nil
		}
		v = v.Elem()
	}
	if v.Type().Implements(reflect.TypeFor[encoding.TextAppender]()) || v.Type().Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
		keyOnly := reflect.MakeMapWithSize(reflect.MapOf(v.Type(), reflect.TypeFor[bool]()), 1)
		keyOnly.SetMapIndex(v, reflect.ValueOf(true))
		data, err := jsonv2.Marshal(keyOnly.Interface(), json.DefaultOptionsV1(),
			jsontext.AllowInvalidUTF8(false), jsontext.AllowDuplicateNames(false))
		if err != nil || strictjson.Validate(data) != nil {
			return "", NewParseError("json_export")
		}
		var names map[string]json.RawMessage
		if json.Unmarshal(data, &names) != nil || len(names) != 1 {
			return "", NewParseError("json_export")
		}
		for name := range names {
			return RedactPAN(name), nil
		}
	}
	if !domainJSONNativeKeyValid(v, map[domainJSONVisit]bool{}) {
		return "", NewParseError("json_export")
	}
	key := domainID(v.Interface())
	if !utf8.ValidString(key) {
		return "", NewParseError("json_export")
	}
	return RedactPAN(key), nil
}

// Check native string identity before fmt-based source-compatible map-key
// conversion can repair malformed text. Exported composite key fields are part
// of that identity; private implementation fields stay behind custom boundaries.
func domainJSONNativeKeyValid(v reflect.Value, visits map[domainJSONVisit]bool) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.String:
		return utf8.ValidString(v.String())
	case reflect.Interface:
		return v.IsNil() || domainJSONNativeKeyValid(v.Elem(), visits)
	case reflect.Pointer:
		if v.IsNil() {
			return true
		}
		visit := domainJSONVisit{typ: v.Type(), ptr: v.Pointer()}
		if visits[visit] {
			return false
		}
		visits[visit] = true
		defer delete(visits, visit)
		return domainJSONNativeKeyValid(v.Elem(), visits)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !domainJSONNativeKeyValid(v.Index(i), visits) {
				return false
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" && !domainJSONNativeKeyValid(v.Field(i), visits) {
				return false
			}
		}
	}
	return true
}

// Literal roles are trusted only on these exact native financial schemas.
// Foreign layouts (even embedded snapshots or identical tags) do not acquire
// this provenance, and untyped custom JSON never passes through this boundary.
func domainJSONIsNativeFinancial(value any) bool {
	switch value.(type) {
	case Account, Card, Resource, Operation, CardLedgerEntry, OperationDetail, CardInfo, CategoryAmount:
		return true
	}
	return false
}

// The annotated native schema uses only string, optional string and string-list
// identity roles. Normalize into plain values, checking original text before
// the encoder can replace bytes. Preserve null/empty distinction and ordering.
func domainJSONLiteral(value any) (any, error) {
	switch v := value.(type) {
	case string:
		if utf8.ValidString(v) {
			return v, nil
		}
	case *string:
		if v == nil {
			return nil, nil
		}
		return domainJSONLiteral(*v)
	case []string:
		if v == nil {
			return nil, nil
		}
		out := make([]any, len(v))
		for i, item := range v {
			x, err := domainJSONLiteral(item)
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	}
	return nil, NewParseError("json_export")
}

// Match the selected V1 encoder's omitempty rules, which are based on native
// kind/length rather than IsZero methods or a container's nilness alone.
func domainJSONEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Interface, reflect.Pointer:
		return v.IsZero()
	}
	return false
}

func domainJSONValue(v reflect.Value, visits map[domainJSONVisit]bool) (any, error) {
	if !v.IsValid() {
		return nil, nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		return domainJSONValue(v.Elem(), visits)
	}
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice) && v.IsNil() {
		return nil, nil
	}
	if v.CanInterface() {
		switch x := v.Interface().(type) {
		case domainJSONNormalized:
			return x.value, nil
		case Decimal:
			return x.String(), nil
		case *Decimal:
			return x.String(), nil
		case *Money:
			return domainJSONMoney(*x)
		case Money:
			return domainJSONMoney(x)
		case json.Number:
			if !domainJSONNumberPattern.MatchString(string(x)) {
				return nil, NewParseError("json_export")
			}
			return x, nil
		}
		if snapshot, owned := domainJSONEntitySnapshot(v.Interface()); owned {
			return domainJSONValue(reflect.ValueOf(snapshot), visits)
		}
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Map || v.Kind() == reflect.Slice {
		visit := domainJSONVisit{typ: v.Type(), ptr: v.Pointer()}
		if v.Kind() == reflect.Slice {
			visit.length = v.Len()
		}
		if visits[visit] {
			return nil, NewParseError("json_export")
		}
		visits[visit] = true
		defer delete(visits, visit)
	}
	// Money pointers must not take the general custom-serializer path: financial
	// amounts that happen to pass Luhn are not card numbers.
	// Check both method sets on the original addressable value. Streaming JSON
	// has precedence over legacy JSON even when the legacy method is on T and
	// the streaming redactor is only on *T. Both outrank text appending/marshaling.
	var custom any
	for _, candidate := range []reflect.Value{domainJSONAddress(v), v} {
		if !candidate.IsValid() || !candidate.CanInterface() {
			continue
		}
		x := candidate.Interface()
		if _, ok := x.(jsonv2.MarshalerTo); ok {
			custom = x
			break
		}
		if _, ok := x.(json.Marshaler); ok {
			custom = x
			continue
		}
		if _, legacy := custom.(json.Marshaler); legacy {
			continue
		}
		if _, ok := x.(encoding.TextAppender); ok {
			custom = x
			continue
		}
		if _, appender := custom.(encoding.TextAppender); appender {
			continue
		}
		if _, ok := x.(encoding.TextMarshaler); ok {
			custom = x
		}
	}
	if custom != nil {
		if _, streaming := custom.(jsonv2.MarshalerTo); streaming {
			out, err := domainJSONStreaming(custom, visits)
			if err != nil {
				// Preserve JSONable's established deferred encoder-error behavior
				// for finite floating scalars, without retaining the receiver or its
				// private error. The only later method is a static failure marker.
				if (v.Kind() == reflect.Float32 || v.Kind() == reflect.Float64) && !math.IsNaN(v.Float()) && !math.IsInf(v.Float(), 0) {
					return domainJSONDeferredError{}, nil
				}
				return nil, NewParseError("json_export")
			}
			return out, nil
		}
		if _, legacy := custom.(json.Marshaler); !legacy {
			// Use the official text-interface precedence/UTF-8 checks too.
			// Text failures do not acquire the streaming-float-only deferred
			// compatibility marker or expose private implementation fields.
			return domainJSONStreaming(custom, visits)
		}
		data, err := custom.(json.Marshaler).MarshalJSON()
		if err != nil || strictjson.Validate(data) != nil {
			return nil, NewParseError("json_export")
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		var out any
		if err = dec.Decode(&out); err != nil {
			return nil, NewParseError("json_export")
		}
		return domainJSONValue(reflect.ValueOf(out), visits)
	}
	if v.Kind() == reflect.Pointer {
		return domainJSONValue(v.Elem(), visits)
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return nil, NewParseError("json_export")
		}
		return RedactPAN(v.String()), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint(), nil
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(v.Float()) || math.IsInf(v.Float(), 0) {
			return nil, NewParseError("json_export")
		}
		if v.Kind() == reflect.Float32 {
			return float32(v.Float()), nil
		}
		return v.Float(), nil
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := 0; i < v.Len(); i++ {
			x, err := domainJSONValue(v.Index(i), visits)
			if err != nil {
				return nil, err
			}
			out[i] = x
		}
		return out, nil
	case reflect.Map:
		out := map[string]any{}
		iter := v.MapRange()
		for iter.Next() {
			key, err := domainJSONMapKey(iter.Key())
			if err != nil {
				return nil, err
			}
			x, err := domainJSONValue(iter.Value(), visits)
			if err != nil {
				return nil, err
			}
			if _, exists := out[key]; exists {
				return nil, NewParseError("json_export")
			}
			out[key] = x
		}
		return out, nil
	case reflect.Struct:
		out := map[string]any{}
		typ := v.Type()
		nativeFinancial := v.CanInterface() && domainJSONIsNativeFinancial(v.Interface())
		for i := 0; i < v.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				continue
			}
			// StructTag.Get unquotes with strconv and can replace invalid raw
			// UTF-8 before the decoded field name is available for validation.
			if !utf8.ValidString(string(field.Tag)) {
				return nil, NewParseError("json_export")
			}
			tag := field.Tag.Get("json")
			name, opts, _ := strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if slices.Contains(strings.Split(opts, ","), "omitempty") && domainJSONEmptyValue(v.Field(i)) {
				continue
			}
			if !utf8.ValidString(name) {
				return nil, NewParseError("json_export")
			}
			// Property names are display text regardless of the field's native
			// financial value role. Check collisions after the same masking
			// applied to map names and authoritative custom JSON output.
			name = RedactPAN(name)
			if _, exists := out[name]; exists {
				return nil, NewParseError("json_export")
			}
			var x any
			var err error
			if nativeFinancial && field.Tag.Get("sber") == "literal" {
				x, err = domainJSONLiteral(v.Field(i).Interface())
			} else {
				x, err = domainJSONValue(v.Field(i), visits)
			}
			if err != nil {
				return nil, err
			}
			out[name] = x
		}
		return out, nil
	default:
		return nil, NewParseError("json_export")
	}
}

// ExportJSON is an explicit financial-data export, not a credential dump.
func ExportJSON(value any) ([]byte, error) {
	out, err := JSONable(value)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		// Encoder diagnostics can include raw tokens or custom-marshaler errors.
		return nil, NewParseError("json_export")
	}
	if strictjson.Validate(buf.Bytes()) != nil {
		return nil, NewParseError("json_export")
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// WritePrivateJSON atomically replaces the exact target with a mode-0600 file.
// It replaces a target symlink itself, never writing through it, and removes
// unpublished temporary files on every failure. The parent directory must exist
// and be trusted (not attacker-replaceable). This financial exporter is not the
// enrollment credential writer and makes no no-replace publication guarantee.
func WritePrivateJSON(path string, value any) error {
	data, err := ExportJSON(value)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	defer f.Close()
	if err = f.Chmod(0600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// Products is a snapshot of all three account sections and wallet cards.
type Products struct {
	Accounts []Account `json:"accounts"`
	Cards    []Card    `json:"cards"`
}
