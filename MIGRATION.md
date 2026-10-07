# Modular Go implementation

The 2026-10-07 owner request resumed development from WIP commit `104c8d86ca63f9b5d0c6e6b8dafd3d53d635eb28`. The earlier log is preserved in [docs/history/MIGRATION-WIP.md](docs/history/MIGRATION-WIP.md).

## Compatibility

The module remains `github.com/vasyza/sber-go`. Existing root imports, exported names and function signatures remain available through aliases and forwards. Implementations live under `internal/errs`, `internal/session`, `internal/transport`, `internal/auth` and `internal/bank`. Optional public packages are `browser`, `mcp` and `rental`.

Aliases preserve Go source assignability. Reflection and diagnostics may display the implementation package rather than the former root. Cross-package SDK errors share the `SDKError` marker; concrete errors remain available through `errors.As`.

Client outcomes can now include a diagnostic wrapper. Match them with `errors.Is`/`errors.As` rather than direct type assertions; explicit rejection metadata remains available after unwrapping.

## Repairs and integrations

- Protect decoded client rejection outcomes from unsupported fmt/logger verbs while retaining explicit metadata access. Cover reads, direct mutations, transfer START and mutation sequences.
- Separate Linux session publication from validation; add native macOS atomic saves and metadata inspection.
- Make private test fixtures explicit rather than dependent on process umask; preserve Linux-only inode/terminal tests.
- Correct MCP cache fields, version error data, legacy counteroffers and strict string schema validation.
- Wire bank read commands and six MCP handlers to typed resources. Add Linux primary/OTP/PIN profile creation with atomic no-replace enrollment.
- Integrate session serialization with the enrollment module's pinned directory capability without relaxing ordinary no-follow profile saves.
- Split money, models, transfers, export, date scanning, time filters and parsers into focused files. Preserve synthetic fixtures and public-consumer tests.
- Add examples, Make targets, direct dependency declarations and Linux/macOS CI.

## Acceptance scope

[STATUS.md](docs/STATUS.md) records current checks. Historical manifests and review verdicts are retained as evidence, not new approvals. Passing synthetic checks do not prove authenticated bank behavior or settle the full upstream parity inventory. Financial mutation APIs remain explicit opt-in with one-shot/uncertain-outcome handling.
