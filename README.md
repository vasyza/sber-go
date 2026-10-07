# sber-go

Native Go SDK for the unofficial Sber online-banking protocol: session persistence, primary/PIN/OTP authentication, typed bank resources, exact decimal amounts, a native CLI, a local MCP server, and an independent rental reconciliation engine.

Go **1.27.1**, Linux and macOS, including hidden terminal login and first-time private profile enrollment.

## Build and check

```sh
make check
./bin/sber --help
```

`make check` checks formatting, runs vet and the complete race suite, builds all packages and both commands, and verifies module checksums. Tests use synthetic data and localhost; they need no bank account, Python or installed browser. GitHub Actions contains equivalent Linux/macOS jobs.

## CLI

Create a profile in your local Linux or macOS terminal; login, password, OTP and any newly enrolled online-banking PIN are hidden prompts:

```sh
./bin/sber login --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber status --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber check-session --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber products --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber operations --profile "$HOME/.local/share/sber-go/profile.json" \
  --from 2026-01-01 --to 2026-01-31 --limit 30 --max-pages 100
```

An existing profile needs a private parent directory (0700) and a regular private file (0600). `login` creates missing private directories and refuses to replace an existing profile. Credentials are never accepted through arguments, environment variables or MCP. See [CLI behavior and exit codes](docs/CLI.md).

Use `--ca-bundle PATH` on login and every online command when the bank CA is absent from the system PEM bundle. Remembered-device login uses a hidden PIN: `login --remembered-profile EXISTING_PATH --profile NEW_PATH`. Optional public browser initialization requires explicit matching Firefox/Playwright paths and a dedicated private browser profile with verified certificate trust; see [authentication setup](docs/AUTH.md).

History retains explicit completeness metadata. A final or empty page still has `WindowCompleteness: "unknown"` without independent coverage evidence. An error or page cap produces a nonzero CLI exit without a successful partial result.

## Go API

```go
client, err := sber.NewSberClientFromSessionFile(profilePath, sber.ClientOptions{})
if err != nil {
    return err
}
defer client.Close()

products, err := client.Products().Get(ctx, false)
if err != nil {
    return err
}
encoded, err := sber.ExportJSON(products)
```

Import `github.com/vasyza/sber-go` as `sber`. Root aliases and forwards preserve the previous source API while implementations live in separate internal layers. [The runnable example](examples/read/main.go) includes cancellation, cleanup and deliberate JSON export. [Authentication usage](docs/AUTH.md) explains primary login, OTP and remembered-device renewal.

`Decimal` preserves values such as `9007199254740993.10`. Decode response JSON with `sber.DecodeJSON` to retain number lexemes. `ExportJSON` preserves financial quantities and masks card numbers in display text. Diagnostic formatting of session/auth/client values is redacted; explicit credential and financial exports have separate APIs.

## MCP and rental reconciliation

```sh
./bin/sber mcp --profile /absolute/private/path/profile.json
./bin/rental-check < explicit-ledger.json
```

MCP serves six implemented tools against the selected session: setup status, session info/close, products, operations and one operations page. Authentication remains owner-operated. See [MCP setup](docs/MCP.md). Rental reconciliation consumes an explicit ledger and keeps reminders disabled; see [rental input and output](docs/RENTAL-CLI.md).

## Architecture and verification boundary

Core error, session, transport, authentication and bank-resource packages have separate responsibilities. Optional browser bootstrap, MCP and rental functionality have independent entry points. See [package responsibilities](docs/ARCHITECTURE.md), [migration notes](MIGRATION.md) and [current verification](docs/STATUS.md).

Local tests, race checks, vet and build cover the implemented contracts. On 2026-10-07, owner-authorized native PIN login and an authenticated read E2E passed on macOS: session validation, products and one history page for seven days. The successful session used observed browser identity after ordinary public rendering. Cold primary login without browser initialization, full history coverage and independent production review remain outside this evidence. Earlier WIP/review reports remain historical evidence in `docs/`; their paused/failing status describes the original snapshot.

MIT attribution is retained in [LICENSE](LICENSE) and [NOTICE](NOTICE). CPython notices remain in [third_party/cpython/LICENSE](third_party/cpython/LICENSE). There is no Python runtime adapter.
