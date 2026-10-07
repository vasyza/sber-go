# Local MCP server

Build with `make build`, create a profile through owner login or the SDK, then configure a stdio client:

```json
{
  "mcpServers": {
    "sber-go": {
      "command": "/absolute/path/to/bin/sber",
      "args": ["mcp", "--profile", "/absolute/private/path/profile.json"]
    }
  }
}
```

The verified bank root CA is embedded in the build; no external certificate file or additional arguments are required. An optional `"--ca-bundle", "/absolute/trusted/bank-ca.pem"` replaces default trust for this client. Native chain and hostname verification remain enabled. See [certificate provenance and rotation](../internal/transport/certificates/README.md).

One explicitly selected SDK client belongs to the process. Tool arguments cannot select filesystem paths or provide credentials. Nullable `session_id` defaults to that client; `"current"` is the only explicit selector. Nullable setup `profile` refers to the selected profile; `"default"` is the only named selector.

| Tool | Result |
| --- | --- |
| `sber_setup_status` | Local status; authorization has not been checked |
| `sber_session_info` | Redacted metadata; optional `check_live` warm-up |
| `sber_session_close` | Close session and invalidate further reads |
| `sber_products` | Account/card snapshots and exact quantities |
| `sber_operations` | Paginated history and completeness metadata |
| `sber_operations_page` | One page and next offset |

The advertised catalog contains implemented handlers only. Session close is local lifecycle control with `readOnlyHint: false`; financial handlers remain absent. Authentication is owner-operated before server startup. The larger library schema inventory describes source contracts, not implemented application handlers.

The transport uses the official Go SDK v1.8.0 for `2026-07-28` stateless discovery/tool requests and the `2025-11-25` initialize/initialized handshake. Modern discovery/list include `ttlMs` and `cacheScope`. Unsupported modern versions return `-32022` with `data.supported`/`data.requested`; legacy initialization counteroffers the supported version. Cache TTL is zero, scope private. Requirements were checked against the official [modern specification](https://modelcontextprotocol.io/specification/2026-07-28) and [legacy lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle). See the [adapter profile](MCP-GO-SDK.md) for strict framing, numeric ID limits, schema compatibility, and cancellation behavior.

Frames/IDs are bounded. Strict JSON and schemas precede handlers. Cancellation and terminal stream failure stop active work. Private callback errors become static tool errors. Stdout is reserved for frames.

Embedding: `mcp.New(mcp.Options{Client: client})`, `Serve(ctx, input, output)`, and deferred `Close()`. Serve owns streams for its lifetime; Close releases the session afterward. A bounded owner-authorized SDK read E2E passed on macOS; real MCP-to-bank calls and complete history coverage were not separately tested. See [STATUS.md](STATUS.md).
