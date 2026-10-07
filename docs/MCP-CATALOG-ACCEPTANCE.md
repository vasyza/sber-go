# Offline acceptance: MCP catalog and original-argument validation

Status: **independently verified offline / scoped PASS**. This is not MCP wire/runtime, bank-handler/workflow, whole-SDK/foundation, installation or live banking acceptance.

The original immutable 18-file catalog snapshot and its historical evidence are retained. Completion reviewer audited the unchanged production and original 14 tool contracts / 42 arguments, rechecked 11,964 archived requests (10,344 unique), 36 fresh probes, four detected scratch negative controls and fresh native/race/copy semantics. Parent read back every 270-entry completion integrity target and all scoped live hashes, independently recounted complete JSONL and verified no blocking findings.

Evidence, under `research/go-migration/`:

- `mcp-catalog-independent-review1/completion-verdict.json` — final independent PASS and explicit scope exclusions.
- `mcp-catalog-independent-review1/completion-integrity.json` — acyclic artifact/source integrity manifest, 270 entries, not a signature or bank authorization.
- `mcp-catalog-evidence/review-snapshot/manifest.json` — original 18-file frozen catalog/dependency/fixture seal.
- `mcp-completion-parent-readback/parent-verification.json` — actual parent read-back; also preserves the **separate failing** wire conformance probe.
- `mcp-completion-parent-readback/verified-artifact-manifest.json` — 1,885 unique verified batch targets, including 1,485 unchanged wire historical files.

Approved production is exactly the unchanged `internal/mcptools/{catalog.go,integer.go,validation.go}` bytes in the original seal. Later changes to these files require new validation/review; this document does not extend approval to another package or to handlers merely using their descriptors. Disabled write tools remain unavailable by default; explicit descriptor visibility is not bank-enforced privilege, owner consent or mutation execution authorization.

The separate `internal/mcpwire` recovery passed 39 ordinary tests / 72 subtests, vet/build and real native execution, but full protocol acceptance is **BLOCKED**. Parent repeated its sealed native conformance probe with exit 1: required modern discovery/list cache metadata and unsupported-version error data are missing. Dependency fuzzing is not native wire fuzzing. Fresh independent wire/privacy/version-design review is pending; no source repair or guard weakening is authorized by this catalog approval.
