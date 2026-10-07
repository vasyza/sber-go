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

The wire supports `2026-07-28` stateless discovery/tool requests and the `2025-11-25` initialize/initialized handshake. Modern discovery/list include `ttlMs` and `cacheScope`. Unsupported modern versions return `-32022` with `data.supported`/`data.requested`; legacy initialization counteroffers the supported version. Cache TTL is zero, scope private. Requirements were checked against the official [modern specification](https://modelcontextprotocol.io/specification/2026-07-28) and [legacy lifecycle](https://modelcontextprotocol.io/specification/2025-11-25/basic/lifecycle).

Frames/IDs are bounded. Strict JSON and schemas precede handlers. Cancellation and terminal stream failure stop active work. Private callback errors become static tool errors. Stdout is reserved for frames.

Embedding: `mcp.New(mcp.Options{Client: client})`, `Serve(ctx, input, output)`, and deferred `Close()`. Serve owns streams for its lifetime; Close releases the session afterward. Current bank authorization and history coverage need owner smoke checks; synthetic checks are in [STATUS.md](STATUS.md).
