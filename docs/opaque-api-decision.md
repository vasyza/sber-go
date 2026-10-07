# Coordinated prerelease opaque API decision

## Why this change is necessary

Repair cycle1 retained a concrete P1: `FrontendConfig` exported string fields and raw transfer/`ParseError` fields remain reachable through Go's unsupported formatting diagnostics. `fmt` handles some verbs before `Formatter`, and its `badVerb` fallback disables methods before reflective printing. Adding another `String`/`Format` method cannot close this boundary. Synthetic standalone probes still failed after other functional fixes passed.

## Authorized implementation boundary

This Go repository is private and unpublished: no released consumers or owner installation are switched. The second bounded NEW-layer tranche will replace sensitive value storage with private immutable pointer-backed records plus explicit constructors/getters. This is an intentional prerelease **native API change**, not a removal of SDK protocol/functionality.

Requirements:
- Retain every original frontend field/flag, ParseError schema field and transfer field through explicit accessors. Raw wire values must not be erased, defaulted or re-generated for redaction.
- Readable zero values; no global secret intern table. Document identity/content comparison and copy semantics.
- Clone mutable resource slices, money wrappers and optional identifier pointers at appropriate constructor/getter boundaries.
- Preserve ordinary JSON redaction and valid/unsupported fmt/log/error diagnostics on values, pointers and nested containers.
- Mechanically migrate auth frontend getters/construction and parser error construction without changing bank auth state-machine semantics, credential/POST logic or original assertions.
- Strict test-first tracer evidence and a subsequent fresh independent review. New-layer cycle2 is not a foundation repair retry.

## Evidence and gates

Before the change parent confirmed 15 cycle1 owned source/test SHA256s. The combined frozen NEW-layer snapshot passes 84 top-level and 1436 subtest events with race, vet/build/mod verify. These numbers include fuzz seed events and are not closed independent parity criteria.

Frozen pre-layout files: `research/go-migration/config-models-fix-1-integrated-snapshot` outside git. Saved review finding: `research/go-migration/config-models-review-1.json`. Known diagnostic privacy acceptance still false.

Coordinator implementation: `deleg_11dc5c9b` (opaque API worker); independent cycle1 semantic reviewer uses the immutable frozen snapshot. Foundation, TLS/browser/SRP, owner input/enrollment, CLI, module files, rental and the prior refused job are outside edit scope. No bank, credentials, owner profiles, tenant messages, commits or publication.

The opaque implementation is now exercised and its exact constructor/getter mapping is in `docs/OPAQUE-API.md`. Parent verified 16 current owned hashes and frozen pure gates (91 ordinary tests/4859 subtests, six fuzz suites/50 seeds; race/vet/build pass), plus six narrow mechanical-auth tests/nine subtests. This is **not independent opaque/full-SDK acceptance**. The fresh cycle1 reviewer found additional caller/semantic defects, which the parent reproduced on a copy of current opaque sources. The same bounded cycle2 remains open for the disjoint repairs in `docs/new-layer-cycle2-boundary.md`, followed by combined fresh review; no cycle budget reset or third automatic repair cycle.
