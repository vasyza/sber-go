// Public bank API; implementation lives in internal/bank.
package sber

import (
	"context"
	"io"
	"time"

	sdkBank "github.com/vasyza/sber-sdk/internal/bank"
)

type ClientTransportFactory = sdkBank.ClientTransportFactory
type ClientRenewal = sdkBank.ClientRenewal
type ClientOptions = sdkBank.ClientOptions
type SberClient = sdkBank.SberClient

func NewSberClient(bundle SessionBundle, o ClientOptions) (*SberClient, error) {
	return sdkBank.NewSberClient(bundle, o)
}

type ClientCleanupError = sdkBank.ClientCleanupError

// NewSberClientFromCredentials holds the observed minimum cookie pair in memory.
// Hosts must both be explicit or both absent (runtime discovery). SessionPath,
// when set, persists only rotated credentials, not extra protection cookies.
func NewSberClientFromCredentials(credentials SberCredentials, bundleOptions CredentialsBundleOptions, options ClientOptions) (*SberClient, error) {
	return sdkBank.NewSberClientFromCredentials(credentials, bundleOptions, options)
}

// NewSberClientFromSessionFile loads only the explicitly named profile. Private
// files are mandatory unless SessionLoadOptions explicitly relaxes permissions.
func NewSberClientFromSessionFile(path string, options ClientOptions, load ...SessionLoadOptions) (*SberClient, error) {
	return sdkBank.NewSberClientFromSessionFile(path, options, load...)
}

type ClientFileOptions = sdkBank.ClientFileOptions

// NewSberClientFromFiles imports observed hosts/headers/cookies, not a browser
// process. It performs no login, warm-up or bank request. Cookies may complete a
// sanitized HAR. Unlike profile factories, imported source files are NOT updated.
func NewSberClientFromFiles(har, cookies string, o ClientOptions, imports ...ClientFileOptions) (*SberClient, error) {
	return sdkBank.NewSberClientFromFiles(har, cookies, o, imports...)
}

// NewSberClientFromPINProfile keeps a full remembered browser profile. A ready
// profile is opened without login; an unready profile renews once and is saved
// before its first business transport exists. All paths are caller-explicit.
func NewSberClientFromPINProfile(ctx context.Context, path string, provider PINProvider, o ClientOptions, load ...SessionLoadOptions) (*SberClient, error) {
	return sdkBank.NewSberClientFromPINProfile(ctx, path, provider, o, load...)
}

type ClientTransportOwner = sdkBank.ClientTransportOwner
type BankPortfolio = sdkBank.BankPortfolio

func NewBankPortfolio(raw Products, requester BusinessRequester, options ...ResourceOptions) *BankPortfolio {
	return sdkBank.NewBankPortfolio(raw, requester, options...)
}

type BankAccount = sdkBank.BankAccount

func NewBankAccount(raw Account, requester BusinessRequester, portfolio *BankPortfolio, options ...ResourceOptions) *BankAccount {
	return sdkBank.NewBankAccount(raw, requester, portfolio, options...)
}

type BankCard = sdkBank.BankCard

func NewBankCard(raw Card, requester BusinessRequester, portfolio *BankPortfolio, options ...ResourceOptions) *BankCard {
	return sdkBank.NewBankCard(raw, requester, portfolio, options...)
}

type BankProduct = sdkBank.BankProduct
type ParseError = sdkBank.ParseError

// NewParseError retains the supplied schema location for explicit Field reads.
// Its ordinary error text/JSON are static; it never wraps a private cause.
func NewParseError(field string) *ParseError { return sdkBank.NewParseError(field) }

type Decimal = sdkBank.Decimal

// ParseDecimal accepts decimal strings (including a comma decimal separator),
// validated JSON number lexemes and native numbers. String normalization does
// not apply to json.Number. Decode JSON with DecodeJSON, not float64, when the
// original numeric lexeme must be preserved.
func ParseDecimal(value any) (Decimal, error) { return sdkBank.ParseDecimal(value) }

type Money = sdkBank.Money

// ParseMoney returns nil for nil money objects or an absent amount key. A
// nonnil value that is not a JSON object, or an explicit invalid/null amount,
// is a hard error. Optional parser metadata must use its separate soft path.
// currencyCode:null falls through to currency.code.
func ParseMoney(value any) (*Money, error) { return sdkBank.ParseMoney(value) }

type Account = sdkBank.Account
type Card = sdkBank.Card
type Resource = sdkBank.Resource
type Operation = sdkBank.Operation
type CardLedgerEntry = sdkBank.CardLedgerEntry
type OperationDetailField = sdkBank.OperationDetailField
type OperationDetail = sdkBank.OperationDetail
type CardLimits = sdkBank.CardLimits
type CreditInfo = sdkBank.CreditInfo
type CardInfo = sdkBank.CardInfo
type CategoryAmount = sdkBank.CategoryAmount
type PFMPeriod = sdkBank.PFMPeriod
type PFMAmounts = sdkBank.PFMAmounts
type TimeFilter = sdkBank.TimeFilter
type OperationsPage = sdkBank.OperationsPage
type TransferResource = sdkBank.TransferResource

// NewTransferResource retains all raw resource fields without validation or
// normalization. Copies share immutable identity; accessors expose raw reads.
func NewTransferResource(id, kind, name, currency string) TransferResource {
	return sdkBank.NewTransferResource(id, kind, name, currency)
}

type TransferDraft = sdkBank.TransferDraft

// NewTransferDraft preserves every workflow field, resource and list ordering.
// Nil lists stay nil; explicit-empty lists stay nonnil. No defaults are invented.
func NewTransferDraft(pid, flow, state string, sources, destinations []TransferResource) TransferDraft {
	return sdkBank.NewTransferDraft(pid, flow, state, sources, destinations)
}

type PreparedTransfer = sdkBank.PreparedTransfer

// NewPreparedTransfer retains every supplied field without validation or
// financial mutation. Money is copied by value; its Decimal is immutable.
func NewPreparedTransfer(pid, flow, state, sourceID, destinationID string, amount Money, paymentPurpose string) PreparedTransfer {
	return sdkBank.NewPreparedTransfer(pid, flow, state, sourceID, destinationID, amount, paymentPurpose)
}

type TransferResult = sdkBank.TransferResult

// NewTransferResult retains all workflow fields and defensively copies the
// optional raw document ID. Nil and a present empty ID remain distinct.
func NewTransferResult(pid, flow, state string, documentID *string) TransferResult {
	return sdkBank.NewTransferResult(pid, flow, state, documentID)
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
func JSONable(value any) (any, error) { return sdkBank.JSONable(value) }

// ExportJSON is an explicit financial-data export, not a credential dump.
func ExportJSON(value any) ([]byte, error) { return sdkBank.ExportJSON(value) }

// WritePrivateJSON atomically replaces the exact target with a mode-0600 file.
// It replaces a target symlink itself, never writing through it, and removes
// unpublished temporary files on every failure. The parent directory must exist
// and be trusted (not attacker-replaceable). This financial exporter is not the
// enrollment credential writer and makes no no-replace publication guarantee.
func WritePrivateJSON(path string, value any) error { return sdkBank.WritePrivateJSON(path, value) }

type Products = sdkBank.Products

// DecodeJSON preserves number tokens for exact money and rejects trailing JSON,
// malformed Unicode and decoded duplicate properties before Go can repair or
// overwrite them. Opaque identifiers must retain their original meaning.
// It does not perform transport, read HAR headers or load private profiles.
func DecodeJSON(r io.Reader) (map[string]any, error) { return sdkBank.DecodeJSON(r) }

// RedactPAN masks only Luhn-valid 13–19 digit candidates, including separated
// PANs; non-card long identifiers are not indiscriminately destroyed.
func RedactPAN(text string) string { return sdkBank.RedactPAN(text) }

// ParseOperations retains the server's N+1 sentinel. A missing/malformed
// body.operations is an error (UNKNOWN downstream), never successful zero history.
// Scopes supply card context only; they never alter the transaction amount.
func ParseOperations(payload map[string]any, scopes ...string) ([]Operation, error) {
	return sdkBank.ParseOperations(payload, scopes...)
}

// ParseOperationDetails parses the pure details response. Optional monetary
// detail fields tolerate malformed values; required body data is fail-closed.
func ParseOperationDetails(payload map[string]any) (OperationDetail, error) {
	return sdkBank.ParseOperationDetails(payload)
}

// ParseCardInfo never retains a full PAN; all display text is Luhn-redacted.
func ParseCardInfo(payload map[string]any) ([]CardInfo, error) { return sdkBank.ParseCardInfo(payload) }

// ParsePFMAmounts parses optional analytics periods and categories offline.
func ParsePFMAmounts(payload map[string]any) (PFMAmounts, error) {
	return sdkBank.ParsePFMAmounts(payload)
}

// ParseSourceDate implements the independent canonical Python3.12 DATE
// scanner. Its entry point accepts UTF-8 byte lengths 7, 8 or 10; its basic
// calendar/week scanner does not require consuming residual bytes. In
// particular a valid DATE and DATETIME can denote different days. The result
// is a UTC-labeled civil date, not a UTC instant or a Moscow attachment.
func ParseSourceDate(value string) (time.Time, error) { return sdkBank.ParseSourceDate(value) }

type SourceDateTime = sdkBank.SourceDateTime

// ParseSourceDateTime implements the canonical Python3.12 source ISO grammar
// and default naive-Moscow attachment entirely in Go. Clock/offset fractions
// truncate to microseconds ONCE when read, before computing the true instant.
// Source civil years are 1..9999; a truthful fractional-offset Native UTC value
// can be outside those civil years. Moscow-bound conversion validates separately.
func ParseSourceDateTime(value string) (SourceDateTime, error) {
	return sdkBank.ParseSourceDateTime(value)
}

// OperationSortKey treats invalid dates as the oldest UTC moment, and naive
// ISO dates as Moscow time. It never guesses an unknown operation's date.
func OperationSortKey(date string) time.Time { return sdkBank.OperationSortKey(date) }

// SourceOperationCompare reproduces the canonical operation_sort_key ordering
// relation. Parsed naive values share one Moscow ZoneInfo and compare civil
// fields, even across imaginary clocks; aware fixed-zone and mixed-identity
// pairs compare truthful instants. Invalid input uses the source min-UTC key.
// A zero result means neither < nor >, NOT Python datetime equality. The mixed
// relation can contain cycles: do not pass it to an arbitrary total-order sort.
// SortSourceOperations supplies the pinned canonical application algorithm.
func SourceOperationCompare(a, b string) int { return sdkBank.SourceOperationCompare(a, b) }

// NewTimeFilter accepts ISO dates or datetimes; empty strings leave a bound
// unset. Aware datetimes convert to Europe/Moscow, naive ones start there.
func NewTimeFilter(from, to string) (TimeFilter, error) { return sdkBank.NewTimeFilter(from, to) }

// NewSourceTimeFilter uses the canonical application's Moscow request-WALL
// ordering, before truncating wire output to seconds. It is separate from the
// native NewTimeFilter instant-order convention. Returned bounds remain real
// native instants with source imaginary-clock attachment; Contains still uses
// the native instant contract. No explicit native bound or nanosecond is changed.
func NewSourceTimeFilter(from, to string) (TimeFilter, error) {
	return sdkBank.NewSourceTimeFilter(from, to)
}

// BuildCardLedger deduplicates by (operation, card, direction), in insertion
// order. Direct transfers produce a debit and a credit; additional scopes use
// the transaction's original sign, never billingAmount/account balances.
func BuildCardLedger(operations []Operation) []CardLedgerEntry {
	return sdkBank.BuildCardLedger(operations)
}

// ParseProducts parses pure products JSON. Ambiguous current-account numbers
// never link a card; plain savings accounts do not become current parents.
func ParseProducts(payload map[string]any) (Products, error) { return sdkBank.ParseProducts(payload) }

type BusinessRequester = sdkBank.BusinessRequester

const ProductsPath = sdkBank.ProductsPath

type ProductsAPI = sdkBank.ProductsAPI

func NewProductsAPI(requester BusinessRequester) *ProductsAPI {
	return sdkBank.NewProductsAPI(requester)
}

const PFMAmountsPath = sdkBank.PFMAmountsPath

type AnalyticsOptions = sdkBank.AnalyticsOptions

func DefaultAnalyticsOptions() AnalyticsOptions { return sdkBank.DefaultAnalyticsOptions() }

type AnalyticsAPI = sdkBank.AnalyticsAPI

func NewAnalyticsAPI(requester BusinessRequester) *AnalyticsAPI {
	return sdkBank.NewAnalyticsAPI(requester)
}

type Resources = sdkBank.Resources

func NewResources(requester BusinessRequester, options ...ResourceOptions) *Resources {
	return sdkBank.NewResources(requester, options...)
}

const CardInfoPath = sdkBank.CardInfoPath

type CardsAPI = sdkBank.CardsAPI

func NewCardsAPI(requester BusinessRequester, options ...ResourceOptions) *CardsAPI {
	return sdkBank.NewCardsAPI(requester, options...)
}

type HistoryMetadata = sdkBank.HistoryMetadata
type OperationsCollection = sdkBank.OperationsCollection

const OperationDetailsPath = sdkBank.OperationDetailsPath
const OperationsPath = sdkBank.OperationsPath

type ResourceOptions = sdkBank.ResourceOptions
type OperationsPageOptions = sdkBank.OperationsPageOptions

func DefaultOperationsPageOptions() OperationsPageOptions {
	return sdkBank.DefaultOperationsPageOptions()
}

type OperationsAPI = sdkBank.OperationsAPI

func NewOperationsAPI(requester BusinessRequester, options ...ResourceOptions) *OperationsAPI {
	return sdkBank.NewOperationsAPI(requester, options...)
}

type OperationsQuery = sdkBank.OperationsQuery

func DefaultOperationsQuery() OperationsQuery { return sdkBank.DefaultOperationsQuery() }

// SortSourceOperations returns a shallow value copy in canonical Python3.12.3
// sorted(..., reverse=True) order. SourceOperationCompare is not transitive on
// mixed timezone identities, so the comparison SCHEDULE is part of the source
// contract. This native port retains CPython's reverse/run/binary insertion,
// powersort stack, directional merges and adaptive gallop schedule. It does
// not invoke Python, invent instants, normalize operation dates or mutate input.
// Algorithm adapted from CPython v3.12.3 Objects/listobject.c, PSF License 2;
// copyright (c) 2001-2023 Python Software Foundation, all rights reserved.
// The retained license and modification summary are in NOTICE and third_party/cpython/LICENSE.
func SortSourceOperations(operations []Operation) []Operation {
	return sdkBank.SortSourceOperations(operations)
}

type PaginationLimitError = sdkBank.PaginationLimitError
type AccountsAPI = sdkBank.AccountsAPI

func NewAccountsAPI(requester BusinessRequester, options ...ResourceOptions) *AccountsAPI {
	return sdkBank.NewAccountsAPI(requester, options...)
}

type TransferOptions = sdkBank.TransferOptions

func DefaultTransferOptions() TransferOptions { return sdkBank.DefaultTransferOptions() }

const ChangeProductNamePath = sdkBank.ChangeProductNamePath

var ErrResourceMutationsDisabled = sdkBank.ErrResourceMutationsDisabled

type SessionAPI = sdkBank.SessionAPI

func NewSessionAPI(requester BusinessRequester) *SessionAPI { return sdkBank.NewSessionAPI(requester) }

const Me2MeWorkflowPath = sdkBank.Me2MeWorkflowPath
const ConfirmationWorkflowPath = sdkBank.ConfirmationWorkflowPath

type TransfersAPI = sdkBank.TransfersAPI

func NewTransfersAPI(requester BusinessRequester, options ...ResourceOptions) *TransfersAPI {
	return sdkBank.NewTransfersAPI(requester, options...)
}
