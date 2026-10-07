# Prerelease opaque native API

This is a deliberate **private, unpublished prerelease Go API migration**, not a
released compatibility promise. It replaces writable public fields in
`FrontendConfig`, `ParseError`, `TransferResource`, `TransferDraft`,
`PreparedTransfer` and `TransferResult` with constructors and explicit getters.
Every former field remains accessible with its original native value/type. No
bank interaction, transfer transport or mutation permission is introduced.
`Deviceprint` and its existing `Value()` API are unchanged.

## Why methods alone were insufficient

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

## Verification boundary

Synthetic regression RED preceded the implementation of each of the six opaque
types. Frozen pure NEW-layer fixture tests and a bounded scripted auth selection
are exercised offline on Go 1.27.1 with `GOTOOLCHAIN=local`, `GOPROXY=off` and
`GOSUMDB=sum.golang.org`. Auth source bytes/hashes were frozen before edits and
its changes are mechanical field/construction substitutions, not a new auth
implementation or full auth acceptance.

Evidence, commands, complete test events, original/changed hashes and mechanical
proofs are under `research/go-migration/opaque-api-evidence/` in the surrounding
workspace. The final frozen combined cycle2 review exercised the opaque API and
closed its original demonstrated diagnostic/layout findings, but returned FAIL
for remaining financial exporter and source-date semantics. See
`docs/REVIEW-ESCALATION.md` and `research/go-migration/final-review-escalation/`.
This scoped closure is not full source/auth/SDK approval; the two-cycle budget
is exhausted, and publication and broader/live acceptance remain blocked.
No original provider-refused foundation job or foreign foundation test suite
is rerun by this migration.
