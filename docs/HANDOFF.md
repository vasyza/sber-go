# Owner handoff — unfinished private WIP

The owner requested source publication and will finish the project. This request supersedes autonomous continuation and the earlier prohibition on applying an unaccepted candidate **only for this explicit private WIP handoff**. It does not mark the completion contract achieved or authorize bank access. The standing goal is paused; no live child agents remain. Do not resume work without a new request.

## What was handed over

The full working tree contains the latest sealed **SDK4 combined 206-file candidate** applied over the existing project, not the older financial/date implementation that had remained in the working directory. Original candidate SHA256: `e9df701bf7e1770b3bb0f18a16351edb23cb8da27098d7236a30c2c57bfc1d72`.

Production bytes were copied unchanged from that candidate. Handoff-only changes are documentation/licensing, moving a test fixture lookup from `../independent-oracle.json` to the already-included `testdata/models_review_cycle4/independent-oracle.json`, and retaining the independent client's known-failing synthetic regression as `client_review_cycle4_rejection_privacy_wip_test.go`. No domain repair or blocked review was retried. The old local working tree was backed up outside this repository before copying.

The repository includes Go source/tests, actual synthetic fixtures, the full pending parity inventory, design notes and failed/scoped review reports. Large native binaries, execution logs, browser data and private local owner state are not source publication artifacts. Absolute research paths in historical documents point to local evidence, not runtime dependencies. The development-only Python inventory verifier requires the separate canonical reference checkout; native Go runtime does not.

## Current results, not full acceptance

- Before handoff, the combined private root-package selection passed 350 ordinary tests, 49,595 subtests, 30 fuzz seed suites/268 seeds, an actual compiled race artifact, scoped vet/build/module checks and 9 bounded fuzz targets/90,000 reported executions. **Seven public default-PIN/default-HTTP tests were explicitly excluded**. These are not an unfiltered full-project acceptance result.
- Date/application reviewer saved a scoped PASS for DATES-C3-001..004, including the parent DATE/DATETIME active-test maintenance. The report is retained as `docs/review-cycle4-dates-verdict.json`; full parent artifact acceptance was not completed before the owner handoff.
- Client reviewer saved **FAIL** for the new inherited definite-rejection diagnostic privacy blocker. The four prior CLIENT3 families were reported repaired, but this newer failure prevents acceptance. Report: `docs/review-cycle4-client-verdict.json`.
- Financial independent review terminated with provider `content_policy_blocked`; partial green logs are **not** a completed independent approval. No retry, rephrase, reroute or unfinished probe execution was performed.
- Pure strictjson, rental allocation engine, MCP catalog and offline rental CLI have separately accepted offline scopes. Their PASS does not override SDK/wire/auth or real-history blockers.

## Known code/workflow blockers

### Concrete client rejection diagnostics

`CLIENT4-DECODED-REJECTION-DIAGNOSTIC-PRIVACY`: actual `PostRead`, raw mutation and cached transfer START can return a decoded `*APIRejected` whose remote metadata is exposed by unsupported `fmt`/logger formatting (`%w` used in non-wrapping formatting, including variants). Ordinary valid verbs/JSON and valid `fmt.Errorf("%w", err)` were redacted in the review, while explicit `errors.As` metadata access remained available. This is an inherited concrete-client outcome boundary issue, not proof of live compromise.

The exact synthetic failing witness is retained. Reproduce on your own development machine:

```sh
go test -race -count=1 -run '^TestFreshClient4DecodedRejectionDiagnosticPrivacy$' .
```

This test uses injected synthetic responses and one business call per route; it does not perform real bank requests or auth. No genuine secret values are included.

### Foundation safety not accepted

The earlier foundation review reported: session temporary-file inode/symlink publication, Set-Cookie valueless deletion, Max-Age/Expires precedence, Firefox allowlist traversal, sensitive SRP formatting, GET method-case override and escaped-surrogate session decoding. Its refused repair was not retried. Existing implementation/self-tests do not close these findings. Native crypto/auth/bootstrap/browser/session full parity and live readiness remain unapproved.

### MCP wire conformance

Missing current discovery/list `ttlMs`/`cacheScope`; missing `-32022` error `data.supported`/`data.requested`; legacy unsupported-version counteroffer behavior; schema `description:null`/`title:null`/`required:[null]` coercion. The catalog's offline PASS does not close wire conformance. A separate MCP parent readback/replay/baseline step was blocked and never run; no MCP repair was dispatched.

### Remaining acceptance and live scope

Full source-parity matrix closure, root race/vet/fuzz regression, GitHub CI, complete CLI/MCP bank handlers and authenticated server installation are unfinished. Owner-operated login, actual incoming operations, repeated reads/renewal and complete history are unverified. Tenant IDs/mappings/contracts/opening position/cash reconciliation/contact consent remain owner inputs, not inferred data. Reminders remain disabled.

## Preservation

Repair consumption is retained as **4 of 6 total**; no fifth repair was started. Historical FAILs, source-oracle qualifications and test-only maintenance are not rewritten. Publication is deliberately labeled WIP, not `[verified]`, a release, or successful completion. Use the source as a starting point; do not trust it with real bank secrets or financial mutations until the unresolved boundaries are addressed and independently checked.
