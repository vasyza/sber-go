# Concrete client → resource binding

`SberClient` owns exactly one `NewResources` bundle for its lifetime. All successful
native constructors converge on `NewSberClient`, which attaches the bundle before
publishing the client. Binding construction only allocates native API state; it
adds no request, transport, authentication, warm-up, export, or persistence.
Existing constructor validation, transport factories, PIN-profile renewal,
immutable option copying, cleanup, and close ownership are unchanged.

## Source convenience API mapping

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

## Policy, workflow lifetime, and ownership

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

## Frozen verification and limits

Evidence lives at the absolute path
`/home/hermes/workspace/rental-monitoring/research/go-migration/business-integration-evidence/`.
It retains prebinding bytes/fixtures, full RED/GREEN JSONL and gate manifests,
the isolated sealed-source-plus-owned-overlay snapshot, stress/fuzz logs,
source-to-test mappings, and compiled native artifacts. `final-report.json`
records exact hashes, counts, gate results, and independent-review status.

Verification is synthetic only: injected in-process transport/resource fixtures
cannot perform I/O, and fixture credential POSTs panic. No bank request, owner
profile, actual HAR, real credential flow, or tenant message was used. The joint
scope is the root business package on the sealed parent baseline; it is **not**
whole-SDK/auth/foundation/rental/CLI approval or live bank compatibility.

External cases are retained, not silently repaired or accepted:

- The sealed generic `ExportJSON(boundCard)` seam loses Money provenance through
  its custom marshaler and masks a legitimate Luhn-looking financial amount.
  Entity-specific `ExportJSON` preserving typed values does **not** establish the
  generic contract. Shared model fixes require separate current readback/review.
- An additional prebinding probe shows unsupported `%w` formatting of a bare
  concrete client with the existing synthetic transport can expose its fixture
  state. The new private resource delegation boundary prevents that traversal
  through convenience APIs. It does not claim to redesign or fix the pre-existing
  raw concrete-client formatting boundary.
- Active shared-layer semantic repairs, the separately blocked foundation work,
  and whole-project approval remain outside this integration's ownership.
