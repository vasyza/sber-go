# Pure rental ledger — offline APP domain

`github.com/vasyza/sber-go/rental` is a standard-library-only evaluator of **explicit owner-defined ledgers**. It imports no SDK/root package. It does not read configuration, authenticate, call a bank, store state, send messages, schedule work or determine legal debt.

**No actual tenants, phone/Telegram destinations, contractual dates, currency/price choices or payer mappings are configured.** Every fixture in this package is SYNTHETIC. The three-ledger fixture models one parking rental and two motorcycle rentals only as a synthetic shape. Its prices and dates are not owner records. “Future” in a test means after that test's explicit `AsOf`, never a wall-clock assumption.

## API

```go
func Evaluate(in Input) (Evaluation, error)
```

- `Input`: explicit `AsOf`, `[]Tenant`, `[]Period`, `[]Receipt`, `[]CollectionEvidence`.
- `Tenant`: stable ledger `ID`, exact `Currency`, explicit `LedgerStart`.
- `Period`: globally unique `ID`, exact `TenantID`, explicit `Start`, exclusive `End`, independent explicit `DueAt`, positive `Price Money`. `DueAt` may precede `Start` when the explicit contract requires advance payment; it must not precede `LedgerStart`.
- `Receipt`: globally stable `ID`, positive `Amount Money`, explicit `ReceivedAt`, `Cash` or `Transfer`, owner `Confirmed` flag and an exact `TenantID` **or** unresolved `PossibleTenantIDs`.
- `CollectionEvidence`: one optional record per tenant, explicit `CoverageStart`, `CoverageThrough`, `ObservedAt`, `Complete`, `OwnerReconciled` and negative `HasGaps`, `Truncated`, `PageUncertain` flags.
- `Evaluation`: opaque owned storage; use `AsOf()`, `Periods()`, `Allocations()`, `Credits()`, `Totals()`, `Review()`, `Candidates()`. Accessors return deep independent copies, including nested reasons/affected-ID slices. Do not rely on default JSON marshaling of the opaque evaluation; marshal the returned value records explicitly.
- `InputError`: static `Code` (`INVALID_INPUT`, `CONFLICTING_RECEIPT`, `OVERFLOW`) and schema `Field`, with no raw sender/history data. **Every error returns an empty evaluation, never partial candidates or allocations.**

Do not mutate caller-owned input concurrently while evaluation reads it. Concurrent evaluations of shared read-only input and concurrent accessor-copy mutations are supported and race-tested.

## Exact allocation

`Money.Minor` is `int64`, never floating point. All amounts are positive incoming funds/positive obligation prices. Zero or negative amounts are invalid; refunds, reversals, penalties, interest, FX and deposits are not inferred or modeled. Currency identifiers must be exactly three uppercase ASCII letters and match the configured ledger; there is no live ISO registry lookup, exponent lookup, formatting conversion or FX conversion. The caller defines and consistently supplies minor-unit denomination.

The checked sum of all explicit prices **per currency**, and independently all unique receipt amounts **per currency**, must not exceed `math.MaxInt64`. The receipt bound includes unresolved and after-`AsOf` receipts. Deduplication happens before receipt summation. This intentionally conservative aggregate bound may reject otherwise isolated ledgers whose combined values exceed the bound. Every allocation/output aggregate is a nonnegative subset of a checked bound; subtraction cannot cross zero.

1. Validate the entire input before returning decisions.
2. Deduplicate receipt IDs globally using all semantic fields: amount/currency, exact mapping or possible-ID set, confirmation, method and receipt instant. Equivalent timestamp offsets and possible-ID set order do not create new funds. Conflicting duplicates fail closed; stale and confirmed versions must not be mixed as separate receipts.
3. Order explicit periods by tenant ID then contractual `Start`. Reject overlaps within a tenant; adjacent half-open periods are allowed.
4. Order receipts by `ReceivedAt` then stable receipt ID. Receipt-review instants are canonical UTC; original contractual start/end/due representations are preserved.
5. Allocate only confirmed, exactly mapped receipts received on/before `AsOf`, oldest explicit obligation first **within that ledger only**. Keep exact partial remainders. Apply prepayment to explicitly supplied later periods without creating periods or changing anchors. Retain residual credit with its receipt/tenant identity; never spill it into another tenant.

A receipt before a period's start is not a new anchor. Owner-confirmed opening/prepayment receipts may precede ledger start; the owner must reconcile their availability in the ledger's starting position. The engine does not prove that a receipt was not already consumed by a different, omitted ledger snapshot. Inputs must describe one coherent owner-reconciled ledger scope, including all obligations/starting funds relevant to that scope.

The SYNTHETIC cash regression supplies `500000` RUB minor units and two explicitly configured `250000`-minor-unit periods. It demonstrates 5000 cash covering two 2500 periods under that fixture's stated two-decimal denomination, **not an actual owner payment or price**.

## Mapping and owner review

- Nonblank `Receipt.TenantID` is an exact owner-defined identifier, never a sender name or prefix search. Similar IDs remain independent.
- Nonempty `PossibleTenantIDs` with blank exact ID is **ambiguous**, even with a single hint. It is never promoted to exact mapping. Hint IDs must be unique, known and currency-compatible; supplying both exact mapping and hints is invalid.
- Blank exact ID with no hints is fully unmapped. It may affect **every configured ledger**, conservatively even across currencies, until owner resolution.
- Unconfirmed exact receipts affect only their exact ledger. Ambiguous receipts affect only the explicit possible-ID set. A single unresolved minor unit is enough to block confident positive debt, even if it could only partially satisfy it.
- All unresolved receipts are returned by `Review()` with receipt ID, exact amount/currency, instant, affected IDs and compact reason codes. They are neither allocated nor counted as confirmed funds. Review is also globally deduplicated.
- All receipts after `AsOf`, including confirmed exact receipts, remain owner-visible with `RECEIPT_AFTER_AS_OF`. They do not allocate, become credit or block debt in that historical snapshot.
- Unknown alleged exact/hint IDs or invalid source fields are schema errors, not guessed mappings. Represent genuinely unmapped valid receipts with a blank exact ID rather than an invented identifier.

No field stores a sender name, phone, Telegram ID, message destination or message body. A reminder candidate is only a decision record, not contact authorization.

## Paid confidence is weaker than debt/reminder confidence

| Condition | State | Candidate |
|---|---|---|
| Confirmed exact funds fully cover a valid period | `PAID`, even with uncertain extra history/unresolved receipts | No |
| Positive remainder and missing/unsafe evidence or potentially applicable unresolved funds | `UNKNOWN` | No |
| Positive remainder, complete reconciled safe ledger evidence, due instant after `AsOf` | `NOT_DUE` | No |
| Positive remainder, complete reconciled safe ledger evidence, due instant on/before `AsOf` | `DUE` | Metadata only |

Not-due eligibility is governed by **`DueAt`, not `Start`**. An explicit advance-payment obligation with `DueAt <= AsOf < Start` can produce candidate metadata when all debt-proof gates pass. A later period start does not impose an additional embargo, and no candidate authorizes contact or delivery.

Debt proof requires all of the following, independently of whether any operations were observed:

- Explicit tenant ledger start and evidence boundaries.
- `CoverageStart <= LedgerStart` and `CoverageThrough >= AsOf`.
- `ObservedAt >= AsOf` and `ObservedAt >= CoverageThrough`; later owner reconciliation of an explicitly historical snapshot is allowed.
- `Complete == true`, covering all relevant collection channels, cash and reconciled starting position, not just bank observation.
- `OwnerReconciled == true`.
- No gaps, truncation or pagination uncertainty and no unresolved applicable receipt received on/before `AsOf`.

Absent evidence, zero/default flags, missing evidence timestamps, inadequate coverage, stale evidence and missing owner reconciliation remain `UNKNOWN`. Identical/contradictory duplicate evidence records, reversed coverage or invalid nonzero timestamps fail closed as input errors. Missing required contractual/receipt timestamps, IDs, prices, invalid currencies/methods, duplicate tenant/period IDs and contradictory/overlapping periods are errors. There is no `time.Now` or implicit recurrence/start/due-date inference.

## Verification and boundary

The evidence bundle is outside the module under `research/go-migration/rental-evidence/`: retained real RED/GREEN outputs, frozen source snapshots per run, race test events, coverage, vet/build/dependency checks, bounded mutation fuzz logs and a programmatically validated acceptance/test mapping.

Re-run from the module root with the pinned compiler:

```sh
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=sum.golang.org /home/hermes/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin/go test -race ./rental
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=sum.golang.org /home/hermes/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin/go vet ./rental
GOTOOLCHAIN=local GOPROXY=off GOSUMDB=sum.golang.org /home/hermes/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin/go build ./rental
```

**Independent engine review 1: PASS.** All 15 offline APP criteria were exercised on the frozen package; the parent verified all 16 manifest entries and unchanged production hashes. Two nonblocking suggestions are now covered by test-only observation/advance-due regressions and documentation. The strengthened package passes 30 regular tests, 92 regular subtests and 21 fuzz seed cases under race; vet/build pass. Production code was not changed after approval. The separate offline CLI review failed at timestamp decoding precision; its bounded adapter repair is pending and is not included in this engine approval. Its bug does not change the approved native time.Time engine production, but the CLI cannot be accepted until its input boundary is repaired and independently reviewed.

This is **local allocation software**, not proof of bank synchronization, live history completeness, actual tenant collection readiness or legal debt. Actual owner-provided mappings/contracts/reconciled complete history remain external live gates. The package performs no bank or tenant interaction; the separate native `rental-check` command only previews explicit noncredential JSON. No bank/MCP/cron/delivery integration, commits or publication are authorized by this engine review.
