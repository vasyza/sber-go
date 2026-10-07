# Native MCP source-tool catalog and argument integrity

`internal/mcptools` is a pure native contract layer, not a bank service. It performs no filesystem/profile discovery, authentication, network requests, secret-inbox access, financial POSTs or tenant messaging. Bank tool handlers and complete workflow parity are separate unfinished application work.

## Preserved source shape

The catalog retains all 14 names, argument names/types/nullability/requiredness/default annotations and registration order from the pinned licensed MCP source. `Catalog(false)` returns nine non-financial-mutation descriptors; `Catalog(true)` adds the five mutation descriptors. The second call is explicit descriptor policy, not bank-enforced privilege, human approval or authorization to perform a transfer.

Descriptors are independent copies. JSON Schema uses `additionalProperties=false` as intentional native hardening: unknown, secret-bearing and case-aliased arguments are rejected rather than ignored. Defaults are descriptive; this package does not execute handlers or apply workflow/session defaults. It is not a general arbitrary-JSON-Schema validator.

## Validation boundary

`ValidateArguments(name, originalBytes, allowWrites)` rejects unknown/default-disabled tools first, then enforces the 1 MiB `MaximumArgumentBytes` bound before strict validation of the complete original document. Invalid UTF-8, unpaired surrogate escapes, escaped/decoded duplicate keys, multiple documents, nonobject values, extra/case-alias keys, wrong primitive types and missing required fields fail with static errors. Input bytes are not modified. No private decoder cause or source value enters an error.

Required JSON Schema integer semantics are checked exactly without float64, machine integer conversion of the coefficient or allocating decimal powers. Integral decimal/exponent forms and arbitrarily large valid integer tokens remain classified correctly; nonintegral values do not round into integers. The independent big.Rat fuzz oracle is bounded to finite token/exponent sizes, while separate lexical tests exercise huge exponents without allocating powers of ten.

This package does **not** validate profile/session handle existence, contract dates, actual balances, transfer positivity/currency/workflow membership, OTP delivery, owner approval, SDK integer representability or history completeness. Concrete native handlers must implement those source contracts, safe ownership and one-shot semantics before runtime acceptance.

## Executed evidence, not approval

Seven actual vertical RED→GREEN feature cycles were retained: missing catalog, full source schemas, original-document integrity, primitive/requiredness, exact integer semantics, forged write registration bypass, and size bounds. Later defensive-copy/concurrency and fuzz additions are test-only strengthening, not invented production REDs.

The first rational-oracle fuzz run found an oracle harness error on valid JSON number framing whitespace (`1 `). Its original failed log and minimized corpus are retained. The oracle now trims only JSON framing whitespace; production validation was not changed for that oracle correction. The same corpus passes and remains in testdata.

Frozen manifests, exact command/cwd/environment/log outputs, compiled race artifact, source AST/fixture hash proof and bounded fuzz execution are under `research/go-migration/mcp-catalog-evidence/`. Final independent review is required. Passing this package never closes the 14 actual bank workflows, SDK/root/foundation, installation or authorized bank history verification.
