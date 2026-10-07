# sber-go — WIP

Native Go migration of the unofficial ex3lite Sber SDK, local CLI/MCP layers, and a pure rental ledger. **Owner-requested source handoff, not a production-ready release.** Autonomous work is paused; the owner will finish the project.

This snapshot includes the latest combined SDK repair-cycle-4 source, regression tests and synthetic fixtures, plus the existing CLI, MCP and rental packages. See **[handoff and known blockers](docs/HANDOFF.md)** before use. Historical manifests/review reports are evidence, not a claim that every acceptance criterion passes.

## Build

Go **1.27.1** is pinned in `go.mod`; there is no Python runtime adapter.

```sh
go build ./...
go build -o bin/sber ./cmd/sber
go build -o bin/rental-check ./cmd/rental-check
```

Both CLI commands use Cobra v1.10.2.
The `sber` command checks local profile files and shows profile properties with secret values removed.
The `rental-check` command reads a supplied rental ledger from standard input and writes an offline preview.
Both commands provide help with `--help` or `-h`.

```sh
./bin/sber --help
./bin/sber status --profile /explicit/private/path/profile.json
./bin/rental-check --help
./bin/rental-check < explicit-ledger.json
```

The rental preview contains `reminders_enabled: false` and `bank_authorization_checked: false`.
The commands do not collect bank history or send reminders.
Give the rental command all contract data in the ledger.
Read the [sber guide](docs/CLI.md) and [rental-check guide](docs/RENTAL-CLI.md) for input rules and exit codes.
CLI guides, help text, and messages follow the [ASD-STE100 writing rules](docs/CLI-WRITING.md).

## Development checks

```sh
go test -race ./...
go vet ./...
```

**The full acceptance suite is not green/approved.** A known failing client-rejection privacy regression is deliberately retained; the complete parity matrix, foundation/auth safety, protocol conformance, CI, server installation and authorized live-history checks remain unfinished. Do not treat `go build` or previously passing scoped tests as full readiness.

## Contents

- Module root: auth/session/transport, exact financial models/parsers, client/resources and transfer workflows.
- `browser/`, `internal/ownerinput`, `internal/enrollment`: partial native/browser/bootstrap and local owner-input boundaries.
- `internal/mcpwire`, `internal/mcptools`: native stdio engine and catalog; catalog slice accepted offline, wire conformance still blocked.
- `rental/`, `internal/rentalcli`, `cmd/rental-check`: pure ledger and offline CLI. The earlier CLI acceptance applies to its listed source hashes.
- `testdata/`: synthetic fixtures/reference contracts; the Python inventory script is development-only, not a runtime dependency.
- `docs/`: parity inventory, design notes, retained failed reviews, current handoff.

Never commit real bank credentials, profiles, cookies, HARs or account history. TLS/sandbox remain enabled; mutations require explicit local opt-in and must not replay uncertain financial requests. Live login/history and tenant delivery require separate owner operation and reconciliation.

MIT upstream notices are retained in `LICENSE`/`NOTICE`; CPython-derived scanner/sort contract notices are preserved under `third_party/cpython` and `docs/DATETIME-CYCLE4.md`.
