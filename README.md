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

Cobra v1.10.2 reads CLI arguments and produces help text.
The CLI has 20 operational commands.
They cover authentication, session restoration, products, cards, history, analytics, private export, and MCP.
Use `sber help COMMAND` to show command options.
Use `--help` or `-h` to show help without profile access or bank requests.

Create a profile in your local Linux or macOS terminal.
Enter the login, password, SMS code, and any new online banking PIN at the hidden prompts.

```sh
./bin/sber login --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber status --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber check-session --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber products --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber operations --profile "$HOME/.local/share/sber-go/profile.json" \
  --from 2026-01-01 --to 2026-01-31 --limit 30 --max-pages 100
```

An existing profile requires a private parent directory with mode `0700` and a regular private file with mode `0600`.
The `login` command makes missing private directories and refuses to replace an existing profile.
Authentication prepares public configuration before secret prompts.
Credentials are not accepted through arguments, environment variables, or MCP.
The [operator manual](docs/CLI.md), [command reference](docs/CLI-REFERENCE.md), and [technical terms](docs/CLI-TERMS.md) use the [ASD-STE100 Issue 9 writing policy](docs/CLI-STYLE.md).

The verified Russian Trusted Root CA is part of the SDK, CLI, and MCP.
Native bank requests require no certificate download or external CA file.
Default trust includes the canonical system PEM bundle when available.
TLS validates the certificate chain and the hostname.
Use `--ca-bundle PATH` to replace default trust for one client.
Read the [certificate source and update instructions](internal/transport/certificates/README.md).

Remembered device login uses a hidden PIN.
Use `login --remembered-profile EXISTING_PATH --profile NEW_PATH` to make a new profile from an existing identity.
Optional public browser initialization requires matching Firefox and Playwright paths.
It also requires a dedicated private browser profile with verified certificate trust.
Read the [authentication instructions](docs/AUTH.md).

The `refresh-session` command restores an existing profile through hidden PIN input and optional SMS confirmation.
Interactive reads can restore an expired session once, save it, and repeat the read.
Use `--no-renew` for a read without secret prompts.
Batch commands and MCP require separate terminal restoration.
The bank controls session lifetime.
A session file does not promise access for a year.

To restore an existing session, use this command:

```sh
./bin/sber refresh-session --profile "$HOME/.local/share/sber-go/profile.json"
```

TLS setup supplies HTTP/1.1 through ALPN.
After a network closure, setup can make up to three connection attempts before HTTP transmission.
All attempts use the same request timeout.
Certificate failures stop connection setup.
The client does not repeat transmitted HTTP requests after a connection error.

The `card-rename` and `transfer-own` commands show offline plans by default.
Execution requires `--execute` and hidden local `CONFIRM` input.
A transfer requires a second confirmation after preparation.
Financial requests do not restore sessions or repeat requests automatically.
These commands have synthetic validation only.
Real financial execution remains outside the live verification scope.

History retains explicit coverage metadata.
A final or empty page still has `WindowCompleteness: "unknown"` without independent coverage evidence.
An error or page cap gives a nonzero exit code without a successful partial result.

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

The MCP transport uses the official Go SDK v1.8.0 for MCP 2026-07-28 and legacy 2025-11-25 clients. See [MCP adapter and verification](docs/MCP-GO-SDK.md).

## Architecture and verification boundary

Core error, session, transport, authentication and bank-resource packages have separate responsibilities. Optional browser bootstrap, MCP and rental functionality have independent entry points. See [package responsibilities](docs/ARCHITECTURE.md), [migration notes](MIGRATION.md) and [current verification](docs/STATUS.md).

Local tests, race checks, vet and build cover the implemented contracts on Linux and macOS. On 2026-10-07, owner-authorized primary login, PIN restoration, interactive expiry recovery, and native CLI read E2E passed on macOS. The E2E covered all 15 read and local profile commands, including card details, limits, analytics, operation details, and private export. Authentication used ordinary public browser rendering with observed browser identity. See the [CLI verification record](docs/CLI-VERIFICATION.md) for executable checks and limits. Real financial mutations, full history coverage and independent production review remain outside this evidence. Earlier WIP/review reports remain historical evidence in `docs/`; their paused/failing status describes the original snapshot.

MIT attribution is retained in [LICENSE](LICENSE) and [NOTICE](NOTICE). CPython notices remain in [third_party/cpython/LICENSE](third_party/cpython/LICENSE). There is no Python runtime adapter.
