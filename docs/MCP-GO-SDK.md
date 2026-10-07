# MCP Go SDK adapter

`internal/mcpwire` uses the official `github.com/modelcontextprotocol/go-sdk`
**v1.8.0**, which supports MCP **2026-07-28**. The SDK owns discovery,
legacy initialization and version negotiation, method dispatch, request
contexts, and JSON-RPC encoding/decoding. The former custom protocol router
and parser have been removed.

The adapter preserves `New(Options)` and `Server.Serve(ctx, input, output)`.
It remains an injected transport layer: it does not load credentials or owner
profiles or implement bank handlers. The application `mcp` package registers
six implemented handlers against one selected SDK client, and `sber mcp`
provides the server command. Application and authentication verification is
documented separately in [STATUS.md](STATUS.md).

## Protocol behavior

- Advertised revisions are `2026-07-28` and `2025-11-25`.
- Modern calls carry request-local `_meta` with protocol version and client
  capabilities. Discovery and listing include `resultType: "complete"`,
  server identity metadata, `ttlMs: 0`, and `cacheScope: "private"`.
- Unsupported modern versions return `-32022` with the required
  `data.supported` and `data.requested`. The requested protocol token is
  echoed as negotiation data; callers must never put a credential there.
  Ordinary diagnostic messages and callback failures remain static.
- Legacy clients use `initialize` and `notifications/initialized`. An
  unsupported legacy version receives the SDK's `2025-11-25` counteroffer.
  The SDK accepts tool calls after initialization, including before the
  initialized notification. Only a successful legacy initialization opens
  this path; modern discovery cannot authorize metadata-free tool requests.
  Modern and legacy client interoperability are tested on separate connections.
- Only tools are advertised. Logging, roots, and sampling are not enabled.
  Unimplemented application methods return method-not-found after protocol
  validation. Metadata-free tool requests before successful legacy
  initialization return invalid-params errors.
- Listing uses the SDK's pagination implementation with a page size of 64,
  matching the maximum configured catalog size. Invalid cursors fail.

The SDK's support matrix is available in its
[README](https://github.com/modelcontextprotocol/go-sdk/tree/v1.8.0).

## Application boundaries

All descriptors are checked before registration, including hidden ones.
Only `ReadOnly: true` or trusted `LocalControl: true` descriptors are registered.
Local control preserves the application's session-close operation without
enabling financial handlers. Schemas are copied, tools are sorted by name,
and annotations reflect each tool's read-only flag and declare non-destructive
behavior. These declarations do not sandbox a callback or grant bank access.
The unchanged `internal/mcptools` catalog contains nine default descriptors;
its five financial mutation descriptors remain opt-in contract definitions.

Each tool needs a concrete `Validate` callback and exactly one of `Handle`
or `RawHandle`. The SDK's low-level `Server.AddTool` API preserves original
argument bytes instead of applying generic schema defaults or decoding
numbers through `float64`. Validators and handlers receive independent
copies. Argument JSON must be a strict object; duplicates, invalid Unicode,
unknown argument names, and invalid types are rejected before execution.
The source catalog's exact integer validator remains in use.

The descriptor checker intentionally accepts a local subset of JSON Schema
2020-12. Null `title`/`description` and null entries in `required` fail at
construction. Registration is preflighted against the SDK; schema metadata
it cannot represent, such as an overflowing numeric default, returns a static
configuration error. Handlers return text plus optional object-valued structured
JSON, with strict Unicode/duplicate validation and defensive copies.
Application errors produce `isError: true` with `Tool execution failed`;
invalid results or panics produce static internal errors. Tool arguments and
raw structured JSON retain exact numeric tokens. The SDK client decodes
structured numeric values into `float64`; use string amounts when exact
financial values must survive that client representation.

## Stdio and shutdown

The SDK `IOTransport` runs over injected streams, with small local guards for
the existing application boundaries:

- One strict JSON object per LF-delimited frame, at most 1 MiB before LF.
  CRLF and fragmented reads are supported. Framing whitespace is trimmed
  before the SDK decoder; bytes inside arguments remain unchanged.
- Malformed framing/JSON/envelopes, batches, and client response messages
  terminate the transport. Invalid method parameters produce correlated
  protocol errors. This replaces the old engine's malformed-frame recovery.
- String IDs are bounded to 256 encoded bytes. Numeric IDs must be integer
  tokens in `[-9007199254740991, 9007199254740991]`: the pinned SDK's numeric
  ID decoder otherwise rounds through `float64`. Larger IDs fail before
  decoding; clients can use strings. Correlation preserves semantic IDs,
  not the original Unicode escape spelling. IDs remain tracked through response
  emission. Duplicate IDs during active callbacks terminate the connection;
  reuse behind an outstanding write waits for that response to finish, allowing
  immediate reuse once the peer has received the response.
- Methods requiring responses cannot invoke handlers as notifications.
  Explicit cancellation reaches the SDK's request context and suppresses
  the eventual response, as required by the modern stdio binding.
- `MaxInFlight` defaults to four and accepts 1–32. Capacity belongs to each
  `Serve` invocation and covers tool requests through response emission.
  Excess calls receive a static internal error; discovery/listing remain
  available while callbacks run and output is writable. Pending requests,
  including control responses, are bounded to `MaxInFlight + 1`. Blocked
  output backpressures input rather than accumulating completed callbacks.
  No handler retries occur.
- Encoded output is strict and bounded to 1 MiB. Oversized results become
  a static internal error. Short writes, write/close errors, and I/O panics
  fail without retries or exposing their private causes.
- Closable stream ownership transfers to `Serve`. Caller cancellation
  closes streams before SDK shutdown drains handlers, interrupting blocked
  reads/writes. EOF cancels unfinished callbacks. Callbacks must cooperate
  with cancellation; blocking streams must be closable. A callback that
  ignores cancellation can prevent shutdown.

## Verification

The tests use synthetic handlers, in-memory pipes, and localhost fixtures.
The official SDK client exercises both supported revisions against all nine
default source catalog descriptors, exact-number arguments, invalid types,
and default-disabled mutations. Wire regressions cover required cache
fields, unsupported-version data, legacy counteroffers, schema nulls,
strict JSON, callback privacy, concurrency, cancellation, and I/O failures.
Independent review findings have regression coverage for mixed envelopes,
initialization gates, incompatible schema metadata, and blocked-output
admission and correlation.

Application tests also exercise all six implemented handlers with an injected
synthetic SDK client, including exact financial output, local session closure,
and rejection of credential or unselected-profile arguments.

```sh
go test -race ./internal/mcpwire ./internal/mcptools ./internal/strictjson
go test -race ./...
go vet ./...
go build ./...
```

On October 8, 2026, after integration with the current `main`, `make check`
passed on Linux: formatting, full vet/race suite, all packages, both binaries,
and module verification. The scoped MCP/application/CLI race suite also passed.
The first full run encountered an unrelated terminal-input test's transcript
poll error; ten focused repetitions and the subsequent full check passed.

Independent review covers the adapter and its integration with the six
application handlers. These synthetic/localhost checks do not establish live
bank verification or independent production acceptance of the whole SDK.
Historical pre-SDK findings and blocked review receipts are preserved in
[MCP-WIRE.md](MCP-WIRE.md) and [MCP-WIRE-BLOCKED.md](MCP-WIRE-BLOCKED.md).
