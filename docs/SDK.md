# Go SDK contracts

Import `github.com/vasyza/sber-go` as `sber`.
The root package exposes the public API through aliases and function forwards.
[AUTH.md](AUTH.md) describes authentication and profile handling.
[ARCHITECTURE.md](ARCHITECTURE.md) describes package boundaries.
[STATUS.md](STATUS.md) records verification limits.

## Immutable values

`FrontendConfig`, `ParseError`, `TransferResource`, `TransferDraft`,
`PreparedTransfer` and `TransferResult` use constructors and explicit getters.
Getters retain the original native values and types.
`Deviceprint` retains its `Value()` API.

## Diagnostic representation



On the pinned Go 1.27.1 toolchain, `%p` is handled before `fmt.Formatter`, invalid
`%w` invokes `badVerb`, and `badVerb` disables formatting methods while reflecting
the original value. Unexported string fields do **not** solve this problem.

The record-backed types hold a private `**record`, not merely a `*record`.
A nested unsupported pointer verb can reset formatting depth to zero and
otherwise dereference a `*record` even inside a private wrapper. The terminal
pointee here is itself a pointer: ordinary diagnostic traversal stops at an
address, not a raw record. `ParseError` uses a private immutable `*string`, whose
terminal pointee is not a composite that fmt dereferences. No backing pointers
escape through getters, and there is no global interning table.

Ordinary formatting, `fmt.Errorf`, standard logger formatting, nested
slices/maps/interfaces/private wrappers and `reflect.Value` wrappers are tested.
This is not encryption, memory erasure, or a secure enclave. Deliberately calling
a raw getter and logging its result, explicitly traversing private pointers via
reflection/unsafe, and memory dumps are outside this diagnostic boundary. Future
Go toolchain changes require re-exercising the fallbacks.

## Constructors

All arguments below are **deliberate raw inputs**, not a redacted Fields/Params
DTO. Constructors do not parse, validate, normalize, mask, contact a bank or
invent cryptographic/workflow/financial defaults.

```go
func NewFrontendConfig(
    baseURL, processID string, pinLength int, nHex, gHex string,
    seamlessWeb, redirectPost bool,
) FrontendConfig

func NewParseError(field string) *ParseError

func NewTransferResource(id, kind, name, currency string) TransferResource

func NewTransferDraft(
    pid, flow, state string, sources, destinations []TransferResource,
) TransferDraft

func NewPreparedTransfer(
    pid, flow, state, sourceID, destinationID string,
    amount Money, paymentPurpose string,
) PreparedTransfer

func NewTransferResult(
    pid, flow, state string, documentID *string,
) TransferResult
```

`ParsePINConfig` and `ParsePrimaryConfig` still perform their existing strict
literal, Unicode, origin, group and mode validation and return a zero runtime
on failure. They now construct the immutable runtime through
`NewFrontendConfig`; optional flags still default to false. Use those parsers
for untrusted frontend HTML, not the raw constructor. A partial synthetic config
such as `NewFrontendConfig("", "", 0, "", "", false, true)` is supported without
synthesizing SRP parameters or a base URL.

## Exact field-to-getter mapping

| Type | Former public field | Explicit getter | Zero read |
|---|---|---|---|
| `FrontendConfig` | `BaseURL string` | `BaseURL() string` | `""` |
| | `ProcessID string` | `ProcessID() string` | `""` |
| | `PINLength int` | `PINLength() int` | `0` |
| | `NHex string` | `NHex() string` | `""` |
| | `GHex string` | `GHex() string` | `""` |
| | `SeamlessWeb bool` | `SeamlessWeb() bool` | `false` |
| | `RedirectPost bool` | `RedirectPost() bool` | `false` |
| `ParseError` | `Field string` | `(*ParseError).Field() string` | `""` (also nil receiver) |
| `TransferResource` | `ID string` | `ID() string` | `""` |
| | `Kind string` | `Kind() string` | `""` |
| | `Name string` | `Name() string` | `""` |
| | `Currency string` | `Currency() string` | `""` |
| `TransferDraft` | `PID string` | `PID() string` | `""` |
| | `Flow string` | `Flow() string` | `""` |
| | `State string` | `State() string` | `""` |
| | `Sources []TransferResource` | `Sources() []TransferResource` | `nil` |
| | `Destinations []TransferResource` | `Destinations() []TransferResource` | `nil` |
| `PreparedTransfer` | `PID string` | `PID() string` | `""` |
| | `Flow string` | `Flow() string` | `""` |
| | `State string` | `State() string` | `""` |
| | `SourceID string` | `SourceID() string` | `""` |
| | `DestinationID string` | `DestinationID() string` | `""` |
| | `Amount Money` | `Amount() Money` | `Money{}` |
| | `PaymentPurpose string` | `PaymentPurpose() string` | `""` |
| `TransferResult` | `PID string` | `PID() string` | `""` |
| | `Flow string` | `Flow() string` | `""` |
| | `State string` | `State() string` | `""` |
| | `DocumentID *string` | `DocumentID() *string` | `nil` |

Getters do not PAN-mask, truncate or erase raw identifiers, names, payment
purposes, amounts or currencies. They are the deliberate boundary for legitimate
wire/owner operations. Existing auth reads become calls such as
`config.ProcessID()` and `config.NHex()`; a partial struct literal becomes an
explicit constructor with the same zero fields and flags. Parser/domain errors
become `NewParseError(existingFieldExpression)` and `err.Field()`.

## Zero, copies, equality and immutability

- Native `T{}` remains usable and readable for each opaque value type. A
  constructor with all native-zero arguments returns exactly `T{}`; an empty
  `NewParseError("")` returns a nonnil pointer to `ParseError{}`.
- All opaque value structs are now comparable. `==` compares **immutable
  identity**, not content. Copies share identity. Independent nonzero
  constructions, even with identical values, have different identities. This
  changes prior content equality for the string-only structs; `TransferDraft`,
  previously slice-bearing and noncomparable, becomes identity-comparable.
- For content equality, compare all scalar getters. Compare Money values with
  `==` (Decimal components remain exact); compare optional IDs by nil/presence
  then dereferenced strings, never pointer addresses. For draft contents, compare
  list lengths/order and each resource's `ID`, `Kind`, `Name` and `Currency`
  getters; `slices.Equal` on independently constructed resources compares
  identity, not resource content. No `Equal` method or raw-fields DTO is added.
- `TransferDraft` clones both input lists and every returned list. Nil stays nil,
  explicit-empty stays nonnil, ordering and duplicate entries are retained.
  Immutable resource snapshots may share their backing identities safely.
- `PreparedTransfer` copies Money at construction and returns Money by value.
  Changing a caller/getter Money wrapper cannot alter a prepared copy. Decimal
  sign, scale and full precision remain intact, including financial values that
  happen to pass Luhn. No amount becomes zero to make diagnostics pass.
- `TransferResult` copies an input document pointer and returns a fresh pointer
  on each nonnil read. Nil and present-empty IDs remain distinct. Mutating a
  returned pointer cannot change the snapshot.
- Strings, the schema field, and backing records cannot be mutated through this
  API. Overwriting a caller's opaque wrapper does not mutate its copies.
  Concurrent reads/getter-copy mutation are exercised under the race detector;
  callers still must synchronize writes to the same wrapper variable itself.

## Error and serializer policy

`*ParseError` remains an `error` and SDK error. `Error`, `String` and `GoString`
remain static (`sber: invalid domain data`), `errors.As` still finds
`*ParseError`, normal `%w` wrapping retains error identity, and no private cause
or `Unwrap` method is added. The original pointer JSON receiver and static
`{"error":"parse_error"}` body are retained, including a direct
`(*ParseError)(nil).MarshalJSON()` call. Under the standard encoder, nil pointers
still marshal as JSON null. Nonaddressable native/copied non-error values have
no exported fields and encode as an opaque `{}` instead of exposing a schema
field; addressable values may use the original pointer marshaler. Both paths
are secret-free, including nested values and explicit `ExportJSON`.

`FrontendConfig` continues to marshal as `"<redacted>"` (standard JSON HTML
escaping is unchanged). Transfer `String`/`Format` and JSON shapes preserve their
prior display policy: resource kind/currency and workflow flow/state remain
PAN-redacted display metadata; IDs, names, PID, prepared amount and purpose are
not dumped. Draft JSON reports list counts, prepared JSON reports redacted amount
and currency, and result JSON reports document-ID presence only.

These redacted JSON methods are **not raw wire/persistence codecs or roundtrip
constructors**. Raw field struct literals/assignment and implicit public-field
JSON population are no longer this API; construct explicitly after decoding the
appropriate wire input. No raw JSON decoder or credential DTO is added.
Financial Account/Card/Operation/Money/Decimal exports, parser policies and
`JSONable`/`ExportJSON`/`WritePrivateJSON` algorithms are otherwise unchanged;
only owned ParseError construction/getter callsites are migrated.


## Client resources

`SberClient` owns exactly one `NewResources` bundle for its lifetime. All successful
native constructors converge on `NewSberClient`, which attaches the bundle before
publishing the client. Binding construction only allocates native API state; it
adds no request, transport, authentication, warm-up, export, or persistence.
Existing constructor validation, transport factories, PIN-profile renewal,
immutable option copying, cleanup, and close ownership are unchanged.

### Resource methods

The reference is `sber-sdk/fork/sber_unofficial/client.py`,
`AsyncSberClient.__init__` and `portfolio`. The seven source attributes become
getters returning the actual, stable native APIs—not placeholder wrappers:

| Source attribute | Native getter | Functional operation exercised synthetically |
| --- | --- | --- |
| `products` | `Products() *ProductsAPI` | `Get(ctx, forceUpdate)` |
| `operations` | `Operations() *OperationsAPI` | `Page(ctx, options)` |
| `accounts` | `Accounts() *AccountsAPI` | `List(ctx, forceUpdate)` |
| `cards` | `Cards() *CardsAPI` | `Info`, `List`, explicitly enabled `Rename` |
| `transfers` | `Transfers() *TransfersAPI` | explicitly enabled `Start`, `Prepare`, `Confirm` |
| `analytics` | `Analytics() *AnalyticsAPI` | `Amounts(ctx, from, to)` |
| `session` | `Session() *SessionAPI` | `Export`, `Credentials`, explicit `WarmUp` |

The exact portfolio signature is:

```go
func (c *SberClient) Portfolio(ctx context.Context, forceUpdate bool) (*BankPortfolio, error)
```

It forwards to the retained bundle's `Portfolio`, not to a newly constructed
`NewBankPortfolio` or a separately built resource API. Every successful portfolio,
accounts-list, or cards-list call reads products exactly once, with the source
body `{"withData":true,"forceUpdate":<supplied bool>}`. Account/card relationships
resolve within that response. A later read creates a distinct immutable financial
snapshot, while retaining the same operations/cards/transfers APIs and issuer.
Returned raw values, money wrappers, identifier pointers, and slice copies remain
independent of both prior snapshots and caller mutation.

Example read-only use after any successful constructor:

```go
products, err := client.Products().Get(ctx, false)
portfolio, err := client.Portfolio(ctx, true)
accounts, err := client.Accounts().List(ctx, false)
cards, err := client.Cards().List(ctx, false)
page, err := client.Operations().Page(ctx)
amounts, err := client.Analytics().Amounts(ctx, "2026-07-01", "2026-07-31")
credentials, err := client.Session().Credentials() // explicit sensitive read
_ = products; _ = portfolio; _ = accounts; _ = cards
_ = page; _ = amounts; _ = credentials; _ = err
```

Keep and use the constructor-returned `*SberClient`; as with its existing mutex
and transport ownership, do not copy a client value and operate the copy. A zero
value or manually assembled client is not an initialized SDK client.

### Policy and ownership

`ClientOptions.AllowMutations` defaults to false. Its copied explicit value is
passed to the retained `ResourceOptions.AllowMutations`. Both gates remain
independent: an enabled separately constructed resource API cannot enable a
read-only concrete client's core. Default-bound entity rename/transfer and direct
Start/Prepare/Confirm refuse before any mutation request or discovery.

Opt-in does not confirm automatically. `TransferTo` performs Start and Prepare
only; the owner must deliberately call `Confirm` on the client's retained issuer
(or the same issuer exposed by a portfolio). Foreign, reconstructed, reused, and
already-attempted draft/prepared snapshots remain fail-closed. Cross-read entity
actions share guards, so reading fresh products cannot reset uncertainty or a
consumed confirmation. Confirmation delegates the whole three-POST sequence to
core `MutationSequence`; queued reads cannot interleave. Failed mutation sends,
cancellation after send, expired authentication, malformed workflow responses,
and uncertain persistence are never automatically retried or PIN-renewed.
Definite-rejection semantics of the existing resource APIs are unchanged.

Resources retain one private terminal-pointer-backed requester adapter forwarding
all six `BusinessRequester` methods unchanged to the original concrete client.
It owns no transport, PIN provider, policy, retry loop, persistence, or close.
The native APIs remain native APIs. Both the bundle and the adapter's client
pointer are initialized before publication and never reassigned. Terminal backing
prevents default and unsupported `fmt` traversal of the newly exposed cyclic
API → requester → client → resource graph from reaching private session state.

Close remains solely the concrete client's responsibility. Bound typed reads,
exports, warm-up, and direct requester calls propagate core `ErrClosed`; a closed
mutation-sequence callback is never invoked. Existing resource mutation methods
may classify a blocked attempted write as `MutationUncertain` rather than expose
`ErrClosed`; their existing error contracts are deliberately retained. Holding a
snapshot or calling a getter cannot resurrect transport, authentication, or a
workflow lifetime. Valid read renewal still publishes the private session before
the typed retry; failed renewal persistence cannot return a typed snapshot.
Definite business rejection cannot become a successful empty portfolio or publish
rotated cookies.

The resource history calendar uses its existing `ResourceOptions.Now` default
(`time.Now`) for the source's default 120-day Moscow window. The binding does **not**
pass `ClientOptions.Monotonic` as `Now`: monotonic time is only warm-up/debounce
clock policy, not a calendar-date injection point.


## Custom requesters

`NewResources` accepts a caller-owned `BusinessRequester` with these methods:

```go
PostRead(context.Context, string, map[string]any) (map[string]any, error)
Mutate(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
MutationSequence(context.Context, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error
ExportSession() (SessionBundle, error)
ExportCredentials() (SberCredentials, error)
WarmUp(context.Context, bool) error
```

The concrete client implements this interface.
The sequence sender belongs to its callback and must not escape it.
Do not call the outer locking `Mutate` from a sequence callback.
Resources borrow the requester and do not close its transport.
Read and mutation policies remain separate; neither changes bank-issued privileges.
