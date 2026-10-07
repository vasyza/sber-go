# Local MCP server

Build with `make build`, create a profile through owner login or the SDK, then configure a stdio client:

```json
{
  "mcpServers": {
    "sber-go": {
      "command": "/absolute/path/to/bin/sber",
      "args": ["mcp"]
    }
  }
}
```

The verified bank root CA is embedded in the build; no external certificate file or additional arguments are required. An optional `"--ca-bundle", "/absolute/trusted/bank-ca.pem"` replaces default trust for this client. Native chain and hostname verification remain enabled. See [certificate provenance and rotation](../internal/transport/certificates/README.md).

The CLI opens the default user profile after owner login. Add `"--profile", "/absolute/private/path/profile.json"` to select another file. One selected SDK client belongs to the process. Tool arguments cannot select filesystem paths or provide credentials. Nullable `session_id` defaults to that client; `"current"` is the only explicit selector. Nullable setup `profile` refers to the selected profile; `"default"` is the only named selector.

The process uses the proxy saved through `sber config set proxy ADDRESS`.
The MCP process must run as the same operating system user with the same configuration directory.
Add `"--proxy", "socks5://127.0.0.1:1080"` to replace the saved setting for that process.
Add `"--no-proxy"` to select a direct connection.
Both options configure native bank connections; MCP still uses standard input and output.
Tool arguments cannot change the proxy or supply proxy login values.
Proxy failures stop requests without a direct connection.
See the [proxy procedure](CLI.md#proxy-settings).

| Tool | Result |
| --- | --- |
| `sber_setup_status` | Local status; authorization has not been checked |
| `sber_session_info` | Redacted metadata; optional `check_live` warm-up |
| `sber_session_close` | Close session and invalidate further reads |
| `sber_products` | Account/card snapshots and exact quantities |
| `sber_operations` | Paginated history and completeness metadata |
| `sber_operations_page` | One page and next offset |

The advertised catalog contains implemented handlers only. Session close is local lifecycle control with `readOnlyHint: false`; financial handlers remain absent. Authentication is owner-operated before server startup. The larger library schema inventory describes source contracts, not implemented application handlers.

The transport uses the official Go SDK v1.8.0 for `2026-07-28` stateless discovery/tool requests and the `2025-11-25` initialize/initialized handshake. Modern discovery/list include `ttlMs` and `cacheScope`. Unsupported modern versions return `-32022` with `data.supported`/`data.requested`; legacy initialization counteroffers the supported version. Cache TTL is zero, scope private. Requirements were checked against the official [modern specification](https://modelcontextprotocol.io/specification/2026-07-28) and [legacy lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle). See the [adapter profile](#adapter-profile) for strict framing, numeric ID limits, schema compatibility, and cancellation behavior.

Frames/IDs are bounded. Strict JSON and schemas precede handlers. Cancellation and terminal stream failure stop active work. Private callback errors become static tool errors. Stdout is reserved for frames.

Embedding: `mcp.New(mcp.Options{Client: client})`, `Serve(ctx, input, output)`, and deferred `Close()`. Serve owns streams for its lifetime; Close releases the session afterward. A bounded owner-authorized SDK read E2E passed on macOS; real MCP-to-bank calls and complete history coverage were not separately tested. See [STATUS.md](STATUS.md).

## Adapter profile

`internal/mcpwire` preserves `New(Options)` and `Server.Serve(ctx, input, output)`.
The official Go SDK owns negotiation, dispatch, request contexts, and JSON-RPC encoding.
The adapter accepts injected streams and handlers; it does not load profiles or authenticate.

### Protocol behavior



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

### Application boundaries

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

### Stdio and shutdown

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


## Source catalog

`internal/mcptools` retains 14 source tool schemas.
`Catalog(false)` returns nine descriptors; `Catalog(true)` adds five financial descriptors.
Descriptor policy does not grant bank access or authorize a transfer.
The application advertises only the six implemented handlers listed above.

Descriptors are independent copies and reject unknown arguments.
Defaults describe source contracts; the catalog does not apply them or execute handlers.
`ValidateArguments` checks the complete original document within a 1 MiB bound.
It rejects invalid Unicode, decoded duplicate keys, extra fields, invalid types, and missing required fields.
Integer validation retains exact decimal and exponent semantics without `float64` conversion.
Handlers must separately validate selected sessions, dates, balances, workflows, and history completeness.

## Verification

Wire tests use synthetic handlers and in-memory streams.
The official SDK client exercises both supported revisions.
Application tests use an injected synthetic client for all six handlers.
The tests cover argument precision, framing, schemas, cancellation, concurrency, output bounds, and diagnostic privacy.

```sh
go test -race ./internal/mcpwire ./internal/mcptools ./internal/strictjson ./tests/mcp
```

[STATUS.md](STATUS.md) records the independent review scope and live verification limits.
