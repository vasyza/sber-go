# Generic MCP stdio wire engine

`internal/mcpwire` is an application-injected, local JSON-RPC transport engine. It has **no default tools, bank handlers, SDK integration, authentication, credential loading, network transport, or mutation-handler integration**. The recovery leaves its existing production and test bytes unchanged.

**Status: restricted dual-era implementation, not full MCP conformance or an independent approval.** Successful package tests do not override the protocol gaps below. Parent/fresh review is still required before acceptance or integration.

## Pinned protocol evidence

The implementation constants are `CurrentVersion = "2026-07-28"` and `LegacyVersion = "2025-11-25"`. On **October 7, 2026**, a fresh official `/specification/latest` retrieval redirected to the July 28, 2026 revision; these are exact revisions, not a promise of future compatibility.[15]

Modern MCP uses per-request version/capabilities metadata rather than the legacy initialization handshake; ordinary modern results include `resultType`.[1][6]

Legacy lifecycle rules are separately pinned to November 25, 2025.[5]

The official TypeScript-derived schema is the protocol oracle, not this package's synthetic tests.[1][10]

## Injected API and configuration

- `New(Options) (*Server, error)` freezes its own catalog. `Options.Info` is display information (`Implementation{Name, Version}`), not an identity or authorization proof. Empty values default to `mcpwire` / `native`.
- `Options.Tools` supplies descriptors and callbacks. A descriptor has `Name`, `Description`, `InputSchema`, `ReadOnly`, a mandatory `Validate`, and **exactly one** of `Handle` or `RawHandle`. `Handle` has type `TypedHandler`; `RawHandle` has type `Handler`.
- `Validator` is `func(context.Context, json.RawMessage) error`. The concrete validator must enforce the tool's actual argument rules before its handler runs. A descriptor is not an executable schema validator.
- `TypedHandler` returns `(ToolResult, error)`. `ToolResult` holds `[]TextContent`, optional object-valued `StructuredContent`, and `IsError`.
- `Handler` returns `(json.RawMessage, error)`. Its bytes must contain a complete tool-result object, **not** a JSON-RPC envelope. Allowed fields are `content`, `structuredContent`, and `isError`; `content` is required and consists only of `{type:"text", text:string}` objects.
- All descriptors are checked, including hidden ones. Valid `ReadOnly=false` descriptors are then omitted from both listing and dispatch. Descriptors are sorted by case-sensitive tool name, and schema bytes are defensively copied. This flag is an application declaration, **not a sandbox or proof that a callback is read-only**.
- `MaxInFlight=0` defaults to 4; explicit values must be 1–32. Negative values are invalid. The option bounds concurrent tool jobs, not the number of control requests processed over the lifetime of a connection.

`New(Options{})` creates an empty catalog. The package must not be wired to bank tools merely because its synthetic handlers pass tests.

### Descriptor subset and bounds

Configuration accepts at most 64 descriptors; implementation name/version are each at most 128 UTF-8 bytes. Tool names are 1–128 ASCII bytes drawn from letters, digits, underscore, hyphen, and dot. Descriptions are UTF-8 and at most 4096 bytes. Encoded listing size is preflighted against the frame cap, including a maximum-size correlation ID and modern response metadata.

Schemas must be strict object documents, at most 65536 bytes, with an object root (`"type":"object"`). Accepted keywords are `$schema`, `type`, `properties`, `items`, boolean `additionalProperties`, `required`, `enum`, `description`, `title`, and `default`. Nested schema depth is bounded at 16. `$schema`, when present, must be the exact 2020-12 dialect URI accepted by `schema.go`. Boolean schemas, `$ref`, composition keywords, constraints such as `minimum`, and other dialects/keywords are rejected. There is **no schema fetching**. This is a deliberately restricted structural descriptor check, **not complete JSON Schema 2020-12 support**; validators still enforce actual tool semantics. Official MCP permits broader schemas.[1][3]

## Framing, parsing, and correlation

`Serve(ctx, input, output)` consumes LF-delimited frames and emits one complete JSON-RPC response per LF-delimited output frame. Official stdio framing uses one JSON-RPC message per line and forbids non-protocol stdout.[2] The engine accepts at most `MaxFrameBytes` (1 MiB) before the LF, including exactly that boundary. Missing terminal LF and over-limit frames are terminal transport errors, not accepted partial input. CR immediately before LF remains whitespace within the JSON document. Raw embedded LF is a frame boundary; escaped newlines inside JSON strings are not.

The existing `internal/strictjson` dependency validates original bytes before permissive decoding: UTF-8, JSON grammar, decoded duplicate keys at every object depth, and paired surrogate escapes. It does not rewrite input bytes or numeric lexemes. `mcpwire` supplies the byte-size policy; strict JSON's existing nesting ceiling is 10000.

Only object-shaped envelopes are accepted; batches are not implemented. Envelope member names are case-sensitive and restricted to `jsonrpc`, `id`, `method`, and `params`. `params`, if present, must be an object. Recognizable method/no-ID notification shapes are silent and cannot dispatch tools or initialize a session; undecodable malformed documents can still produce a parse error. An incoming object containing `result` or `error` terminates with static `ErrClientResponse` without replying, including strictly rejected but permissively recognizable response-shaped objects. This client-response prohibition follows the pinned modern stdio binding; this engine does not issue legacy server-to-client requests either.[2]

IDs are bounded by `MaxIDBytes=256` **encoded bytes** and must be strict strings or integer lexemes. Null, booleans, arrays, objects, fractions, and exponent spellings are rejected; this is narrower than accepting every mathematically integral numeric spelling. Replies retain the original ID token byte spelling, without float conversion or Unicode re-encoding. Active-tool correlation compares decoded strings separately from integers; integer `-0` and `0` collide, whereas string `"0"` and integer `0` do not. A duplicate active tool ID is rejected with `-32600`. Completed IDs can be reused.

## Exact routing behavior

### Modern requests

`server/discover`, or the presence of either modern protocol-version/capability marker, selects modern validation. Case variants of those markers are also detected as modern-shaped but cannot satisfy the required exact keys; they do **not** fall back to legacy state.

Each modern request requires exact `params._meta` keys `io.modelcontextprotocol/protocolVersion` and `io.modelcontextprotocol/clientCapabilities`; the latter must be an object. Optional `io.modelcontextprotocol/clientInfo` must have nonempty string `name` and `version`. These are self-reported metadata, never authenticated identity. The pinned specification requires request-local metadata and forbids relying on prior requests for version/capabilities/identity.[1][8]

The implemented modern methods are:

| Method | Actual behavior |
|---|---|
| `server/discover` | Returns declared versions `[2026-07-28, 2025-11-25]`, tools capability, `resultType:"complete"`, and server-info `_meta`; body permits only `_meta`. |
| `tools/list` | Returns the fixed sorted read-only catalog with modern result metadata; body permits only `_meta` (no pagination implementation). |
| `tools/call` | Validates `_meta`, `name`, object `arguments` (omission defaults to `{}`), invokes the concrete validator, then one injected handler. Unknown tools and invalid arguments produce `-32602`. |
| Other methods | `-32601`; unknown methods cannot be converted into discovery. Modern `initialize` and `ping` are not legacy aliases. |

Missing or invalid required metadata produces `-32602`. An unsupported modern version produces static `-32022`, with the **incomplete error shape noted below**. Modern routing never learns request context from the legacy handshake. The July 28, 2026 revision removes core `ping` and initialization and defines discovery instead.[4][6]

### Legacy requests

Requests without modern markers use the legacy route. `initialize` accepts **only** the exact `2025-11-25` version, an object `capabilities`, and nonempty client-info name/version; repeated initialization is rejected. `notifications/initialized` after initialization enables `tools/list` / `tools/call`. `ping` returns an empty result, including before readiness. Unknown methods return `-32601`, and known tool methods before readiness return `-32602`. Handshake state belongs to one `Serve` invocation, not to the `Server` configuration. The pinned legacy lifecycle uses initialization followed by an initialized notification, and its ping utility expects an empty result.[5][12]

Legacy version negotiation is intentionally narrower than the full official lifecycle: an unsupported legacy version receives `-32602` rather than an alternate supported-version initialization result. General legacy interoperability is therefore **not** established.[5]

## Callbacks, concurrency, ownership, and shutdown

Validator and handler argument buffers are separate defensive copies. Valid calls run concurrently up to the configured bound. Discovery/list/control processing can continue while a tool is waiting. Excess tool jobs are rejected with a static `-32603`; no queue or retry is provided. One event loop serializes output; tool responses may be completion-ordered rather than input-ordered.

`notifications/cancelled` validates its request ID and optional reason type, cancels the matching active callback context, and suppresses that job's eventual response. Unknown/completed IDs and malformed cancellation notifications are ignored. The modern stdio binding requires an explicit cancellation notification and no subsequent messages for a cancelled request.[2][9] The engine cannot undo work already completed or retract a response already emitted.

Blocking I/O must implement `io.Closer`, **and ownership transfers to `Serve`**. Context cancellation closes owned input/output to interrupt blocking operations. Teardown closes input, cancels callback contexts, joins reader/workers, and closes output. EOF cancels active callback contexts and waits for completion; it can drain results from callbacks that complete during teardown. Close errors are not silently reported as success. Supplying nonclosable blocking I/O, a closer that does not unblock I/O, or a callback that ignores cancellation can prevent shutdown. Callers must satisfy this cooperative contract; there is no preemptive callback timeout or goroutine kill.

Reader/writer/close panics and callback panics are translated into static failures. Short writes and writes that return both a full byte count and an error are terminal. Output failure never reruns a handler. A typed/raw handler error returns a static tool result (`isError:true`); validator errors return `-32602`, and invalid callback results/panics return `-32603`. Output is strict-validated and bounded after encoding; an invalid/oversized result becomes a bounded static internal error when possible. Handler allocations themselves are not sandboxed or prebounded.

## Privacy and trust boundary

The package has no logging, environment reads, credential/session persistence, process execution, filesystem access, or network-client imports. It reads/writes only injected streams. Parse/validation/handler/I/O errors never insert source bytes, cancellation reasons, tool names, private version strings, callback error text, or panic payloads into diagnostics. Original correlation IDs are intentionally present in replies.

This is **not a universal secret scrubber**: valid handler result content, descriptors, and configured display identity are application-selected output and can contain sensitive data. Injected callbacks remain responsible for authorization, privacy, cancellation, safe data selection, and concurrency safety. Read-only annotations and client/server display information do not establish trust. Official MCP likewise treats tool annotations and self-reported identity as untrusted for security decisions.[1][3][4]

## Known conformance gaps — do not turn green tests into acceptance

The pinned modern schema requires `ttlMs` and `cacheScope` on **both** `DiscoverResult` and `ListToolsResult`; the recovered `route.go` / `modernResult` omit both fields.[10] It also requires `data.supported` and `data.requested` on `UnsupportedProtocolVersionError`; the recovered response encoder emits only static code/message.[8][10] These are normative modern-conformance gaps, not optional features. The privacy choice not to reflect unsupported request versions must be reconciled with that schema by an owner-approved design, not silently weakened.

Consequently, discovery's advertised versions and the version constants are **not a claim that every message conforms to either complete revision**. Legacy fallback negotiation, the restricted schema/ID/result API, and cooperative shutdown limitations are also explicit. Full JSON Schema semantics, nontext content, general modern structured JSON values, MRTR, prompts/resources/subscriptions, progress/logging, tasks/extensions, HTTP, bank integration, and mutation handlers are not provided. Optional capabilities not implemented are not advertised.

The evidence-only native protocol probe checks these modern required fields against real `Server.Serve` output. Its nonzero conformance result is kept separately from passing ordinary package gates; it is not a production RED→GREEN cycle and no production fix is made in this recovery.

## Recovery verification and provenance

Evidence root:

`/home/hermes/workspace/rental-monitoring/research/go-migration/mcp-wire-evidence/recovery-final/`

- `completion/continuity.json` compares all current owned bytes and dependency context with the last actual `runs/23-client-response-green/source` snapshot. Historical names are not verdicts: `12-concurrency-green` actually exited nonzero.
- The last interrupted run's complete JSONL recount is **39 ordinary top-level passes / 72 ordinary subtest passes, zero fuzz targets or fuzz-seed subtests**. It is historical evidence, not final acceptance. Every original RED/GREEN log, run record, snapshot, and seal is retained.
- `completion/final-source-manifest.json` hashes the actual final owned source and complete inline test fixtures, this document, and unmodified module/strict-JSON dependency context. There are no external `mcpwire` testdata files. The frozen copies and their manifest are sealed read-only; hashes are integrity records, not an independent signature or approval.
- `completion/executed-gates.jsonl` records fresh real commands, working directories, explicit toolchain/environment, exit codes, time bounds, and full-log hashes. Native test execution uses the package directory as cwd; its original binary stdout and `test2json` conversion are both retained.
- `mcpwire` has **no existing native fuzz target**. Bounded fuzzing exercises only the two existing `strictjson` dependency targets (`FuzzValidate`, `FuzzRejectCorruptions`); it must not be represented as wire-engine fuzz coverage. Fixture seeds and evolving fuzz executions are counted separately from ordinary tests.
- `completion/protocol-reference-audit.json` records original reference-seal checks and fresh official retrievals. `completion/current-protocol/` retains actual HTTP bodies/text; `references/` and the separately saved `mcp-docs/` context are unchanged.
- `completion-final-report.json` is the outcome authority for newly executed gates, exact counts/hashes, audit findings, preservation checks, and review status. Whole-repository/CLI/bank integration is outside this scoped verification.

All Go execution is pinned to `/home/hermes/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.27.1.linux-amd64/bin/go`, with `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=sum.golang.org`, `GOWORK=off`, `GOMAXPROCS=2`, and `-mod=readonly` for test/vet/build/compile gates. No module edits, installs, git operations, live bank calls, security-control changes, or worker restarts are part of this recovery.

## Sources

[1] https://modelcontextprotocol.io/specification/2026-07-28/basic
[2] https://modelcontextprotocol.io/specification/2026-07-28/basic/transports/stdio
[3] https://modelcontextprotocol.io/specification/2026-07-28/server/tools
[4] https://modelcontextprotocol.io/specification/2026-07-28/server/discover
[5] https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle
[6] https://modelcontextprotocol.io/specification/2026-07-28/changelog
[8] https://modelcontextprotocol.io/specification/2026-07-28/basic/versioning
[9] https://modelcontextprotocol.io/specification/2026-07-28/basic/utilities/cancellation
[10] https://modelcontextprotocol.io/specification/2026-07-28/schema
[12] https://modelcontextprotocol.io/specification/2025-11-25/basic/utilities/ping
[15] https://modelcontextprotocol.io/specification/latest — Official specification latest redirect verified 2026-10-07
