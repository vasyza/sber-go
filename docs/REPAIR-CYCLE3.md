# Coordinated SDK repair/review cycle 3 of 6

Owner authority and retained counter: `REPAIR-BUDGET.md` / `repair-budget.json`. Previous cycles 1–2 remain FAIL/consumed; no foundation/provider/security refusal is retried. This round is native offline work only, not publication or bank readiness.

## Sealed starting point

Parent verified the existing eight failing client/serializer/financial contracts and current production hashes, then froze the complete SDK-only shared/business source/fixture slice under:
`research/go-migration/repair-cycle3/sdk-baseline/manifest.json`.

All **136** sealed files were checked before/after actual baseline execution. Its selected source-contract suite passes **245 ordinary tests / 7,072 ordinary subtests / 14 fuzz suites / 154 seeds** under the pinned Go1.27.1 race detector. This baseline intentionally excludes foundation/auth/cookie/HTTP/browser/crypto test suites, owner helpers and CLI/MCP/rental; copied dependencies compile only. Passing original tests do not close the independent failures.

## Disjoint repair ownership

1. **Financial export and owned serialization:** `models.go`, `entities.go`, `entities_exports.go`, narrowly named cycle3 financial/entity tests and an owned design note. Preserve typed Money/Decimal and literal identifiers before legacy serializers erase provenance. Distinguish trusted native financial snapshots from display/credential redaction; do not disable generic export or bypass credential hooks. Validate original native text before ordinary entity encoding, retaining valid literal U+FFFD and every snapshot field.
2. **Native source-date parity:** `parsers.go`, cycle3 datetime tests and an owned design note. Match actual canonical Python3.12 helper behavior for every source-valid spelling, comparison/filter result and source formatting. Retain exact microsecond truncation, Moscow fold/gap/native bounds and inclusive date semantics. Address all 28 recorded rejected forms and historical/early-year format differences, with the full denominator retained. Any native representation constraint must be explicitly addressed/documented, not hidden in an eligible-subset filter or global identity registry.
3. **Client state/ownership/outcome integrity:** `client*.go`, existing binding callsites/tests as required, and an owned design note. Preserve all constructors, seven resource getters, cached single Resources workflow issuer, Portfolio and close/renewal persistence behavior. Make value/indirect diagnostics safe without copying mutexes; keep established send uncertainty/cancellation when callbacks fail; reject unverifiable/reused closing ownership before persistence/auth adoption. Out-of-range explicit HAR expiry is not missing session expiry.

Workers must explain their scoped architecture before implementing; every production change must have a real current RED followed by minimal GREEN and regression. Preserve public/source functionality, exact original probes and acceptance rules; record intentional prerelease Go-only hardening explicitly. If unavoidable public API changes cross ownership, report the dependency instead of editing another worker's files.

## Review and evidence

Use `research/go-migration/repair-cycle3/{financial,dates,client}/` for full command/cwd/environment/log/hash/differential evidence and sealed final owned sources with actual testdata. Use the parent baseline for immutable foreign dependencies during concurrent RED work, not an unrecorded mutable live tree. No global stashing/staging, commits, network bank activity or module/provider changes. No child may close goals or parent acceptance criteria.

After returned implementations, the parent reads back all owned hashes, repeats scoped current probes/regression/race/vet/build/fuzz and arranges a fresh combined semantic/privacy review. A source/test map or passing worker command alone is not acceptance. Current round consumes one coordinated project attempt; its parallel leaves and individual TDD tracers do not reset or multiply the counter.

## Interrupted delivery and recovered candidate

Original implementation terminal summaries were interrupted/unknown. The parent verified persisted seals and full logs rather than assuming the notification tail meant success. The recovered combined candidate is **171 files**, `research/go-migration/repair-cycle3/parent-recovery/review-manifest.json`, with actual **300 ordinary tests / 27,695 ordinary subtests / 20 fuzz suites / 202 seeds** under race, vet/build/module verification and compiled native artifact execution. Its additional resource page/payload and collection/request-evidence wire adoption each has a real RED→GREEN; the contradictory active cycle2 limitation assertion is separately identified as test-only maintenance. Original failures, baseline seals and legacy native-format differences remain.

Financial/date/parent resource changes are sealed for review, **not yet applied to the repository**. Fresh independent combined semantic/privacy review `deleg_2b93d773` reviews the exact sealed bytes. The project counter remains **3 of 6**; no foundation refusal, forbidden control change, owner bank action, publication/install or tenant activity has been retried or authorized.

## Final review result — historical round 3 FAIL

All three fresh verdicts are **FAIL**. Parent read-back verified **2,155 unique targets** and the unchanged 171-file candidate, then repeated real native financial/client artifacts and exact date application probes. There are **12 blocker families**, four per scope. Archive and consolidated evidence: `research/go-migration/repair-cycle3/cycle3-final-fail-readback/consolidated-verification.json`. The first parent financial binary launch lacked its independent relative oracle fixture; its extra harness failure remains, and the corrected package-cwd launch reproduces the five actual failing ordinary tests/four subtests with the source oracle passing. That correction changes no production or historical seal.

Round 3 is complete as **failed/consumed**, not accepted. The owner-authorized next coordinated round is defined by `REPAIR-CYCLE4.md`; this does not erase earlier failures or broaden original refused foundation/auth permissions.
