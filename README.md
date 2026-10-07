# sber-go — WIP

Native Go migration of the unofficial ex3lite Sber SDK, local CLI/MCP layers, and a pure rental ledger. **Owner-requested source handoff, not a production-ready release.** The MCP transport now uses the official Go SDK; other unfinished application work remains documented below.

This snapshot includes the latest combined SDK repair-cycle-4 source, regression tests and synthetic fixtures, plus the existing CLI, MCP and rental packages. See **[handoff and known blockers](docs/HANDOFF.md)** before use. Historical manifests/review reports are evidence, not a claim that every acceptance criterion passes.

## Build

Go **1.27.1** is pinned in `go.mod`; there is no Python runtime adapter.

```sh
go build ./...
go build -o bin/sber ./cmd/sber
go build -o bin/rental-check ./cmd/rental-check
```

The `sber` command currently provides partial offline status/session-inspection surfaces, not a verified complete bank login/server installation. `rental-check` accepts an explicit synthetic or owner-supplied noncredential ledger on stdin and prints an offline preview:

```sh
./bin/rental-check < explicit-ledger.json
```

Its output keeps `reminders_enabled: false` and `bank_authorization_checked: false`. It does not collect bank history, invent contracts or send reminders.

## Development checks

```sh
go test -race ./...
go vet ./...
```

**The full acceptance suite is not green/approved.** A known failing client-rejection privacy regression is deliberately retained; the complete parity matrix, foundation/auth safety, protocol conformance, CI, server installation and authorized live-history checks remain unfinished. Do not treat `go build` or previously passing scoped tests as full readiness.

## Contents

- Module root: auth/session/transport, exact financial models/parsers, client/resources and transfer workflows.
- `browser/`, `internal/ownerinput`, `internal/enrollment`: partial native/browser/bootstrap and local owner-input boundaries.
- `internal/mcpwire`, `internal/mcptools`: official MCP Go SDK v1.8.0 adapter for MCP 2026-07-28 and the native read-only catalog; see [MCP adapter](docs/MCP-GO-SDK.md). Bank handlers and server installation remain unfinished.
- `rental/`, `internal/rentalcli`, `cmd/rental-check`: pure ledger and independently accepted offline CLI boundary.
- `testdata/`: synthetic fixtures/reference contracts; the Python inventory script is development-only, not a runtime dependency.
- `docs/`: parity inventory, design notes, retained failed reviews, current handoff.

Never commit real bank credentials, profiles, cookies, HARs or account history. TLS/sandbox remain enabled; mutations require explicit local opt-in and must not replay uncertain financial requests. Live login/history and tenant delivery require separate owner operation and reconciliation.

MIT upstream notices are retained in `LICENSE`/`NOTICE`; CPython-derived scanner/sort contract notices are preserved under `third_party/cpython` and `docs/DATETIME-CYCLE4.md`.
