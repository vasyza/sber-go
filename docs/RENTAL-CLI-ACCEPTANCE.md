# rental-check — scoped offline independent acceptance

**PASS for the reviewed native offline CLI boundary**, after CLI repair cycle 1. This completes the separate CLI repair/review stage; it does not consume or reset any SDK repair round. The SDK is still at round 4 of 6 with its own fresh reviews pending.

This acceptance supersedes the historical “independent review pending” status in the retained phase document `docs/RENTAL-CLI.md`. That document and all original 475-file review-seal bytes are deliberately preserved for provenance. Original CLI review-1 FAIL remains archived, not rewritten to PASS.

## Exact accepted scope

- Native command `cmd/rental-check`, strict stdin decoding under `internal/rentalcli`, explicit caller-supplied ledger only.
- Timestamp grammar/precision is validated before native decoding at all nine positions; no silent subnanosecond truncation or malformed clock/offset normalization.
- Every supplied evidence object requires explicit boolean negative gap/truncation/page-uncertainty flags. Missing proof as a whole and missing positive assertions remain UNKNOWN, not inferred completeness.
- Exact int64 minor units, original-byte UTF-8/surrogate/duplicate-key/canonical-name/EOF checks, input bound and static error/output behavior.
- Pure rental engine and strictjson production hashes unchanged. No owner contract, identity, period recurrence, bank scope, history completeness or delivery consent is inferred.

Reviewed CLI production:

- `cmd/rental-check/main.go`: `a732a4e6a850defbebeb42f307161d38920ba99b903e31b916a3cf743e864163`
- `internal/rentalcli/preview.go`: `524b85c05d50656cfd12000532513bf4503b29425c8f5ebf7bd067e45767896a`
- `internal/rentalcli/timestamp.go`: `6220647f8404343c99a84e4f97097b2c81605ed096489eaa098c0cef094b8b32`
- Current reviewed `bin/rental-check`: `09ba6debd00b093eba0bd35600961433fc813be516ccf42d2380fbc31101b38b`

A separately built native reviewer binary has its own verified hash; matching behavior/source provenance is proven, not bit-for-bit cross-directory build reproducibility.

## Verified evidence

Fresh independent verdict: `research/go-migration/rental-cli-independent-review2/completion-verdict.json`, SHA256 `6f66f724dd727dcc03333c0b45b836bd49070484587bd9d3a72ef3a624b72f24`.

Independent acyclic artifact manifest: `rental-cli-independent-review2/artifact-manifest.json`, SHA256 `a4a02b6c7238281ac34e7bd81c74ee452163adaf2d8d5fd12a6e8e48d23e703d`.

Parent verification actually ran `python verify_rental_cli_review2.py`, exit 0. Readback: `research/go-migration/rental-cli-review2-parent-readback/parent-verification.json`.

Parent verified before and after:

- **4,792 distinct declared artifact/source targets**, plus explicit immutable **475-file original seal**, **33 live scoped files**, four previously approved engine/strictjson production hashes and the original failed verdict.
- **389 original-byte fixtures:** 778 old/new executions and 389 reviewer-rebuild executions; 368 identical outcomes, 21 intended new schema rejections, no unexpected change.
- **626 independent native probes**, all recorded assertions pass; includes **80 exact allocation-oracle cases** and three old-binary negative controls. Counts/categories were recomputed from actual records, not copied from prose.
- Fresh scoped race: CLI 27 ordinary/217 subtests/3 fuzz suites/177 seeds; rental 30/92/2/21; strictjson 4/0/2/9. Command package has no own unit-test functions; real command execution is provided by native subprocess tests and probes.
- Independent I/O race tests: 3 ordinary/13 subtests. Fresh vet/build pass. Both bounded temporal/proof fuzz targets completed **20,000 actual reported executions each**; executions are not unique contracts.
- Parent additionally executed **24 exact original-byte probes** across the live reviewed and independently rebuilt native binaries, spanning 12 categories. Exit status/stdout/stderr matched archived outcomes byte-for-byte, and all target/source hashes still matched afterward.

Reviewer also archived five read-only syscall traces, environment controls and `/dev/full` output-failure behavior. Those are synthetic offline execution observations, not bank or owner authorization.

## Limits remain closed

Output always retains `reminders_enabled: false` and `bank_authorization_checked: false`. This acceptance is **not** complete bank-history proof, live collection/reconciliation, actual tenant mappings/contracts, owner-operated enrollment/auth/renewal, tenant messages/cron, full SDK/matrix/CI or server installation.

No production changes were made by the reviewer or parent during this acceptance. The unrelated refused foundation repair, prohibited goal-wait outcome and blocked MCP step were not rerouted. Later production-byte changes need new matching provenance and bounded review; nonbehavioral documentation/status additions do not alter this hash-bound production acceptance.
