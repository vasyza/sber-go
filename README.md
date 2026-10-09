# sber

Unofficial Go SDK and CLI for SberBank Online: native HTTP authentication, session persistence, typed bank resources, exact decimal amounts, proxy support, and a local MCP server.

The CLI command and public Go package are named `sber`.
The Go module is `github.com/vasyza/sber-sdk`.
This independent community project is not affiliated with or endorsed by Sberbank.

Go **1.27.1 or later**, Linux and macOS, including hidden terminal login and first-time private profile enrollment.

## Install the CLI

Requirements: Linux or macOS and [Go 1.27.1 or later](https://go.dev/doc/install).
You do not need a GitHub account or a source checkout.

1. Install the `sber` command:

   ```sh
   mkdir -p "$HOME/.local/bin"
   GOBIN="$HOME/.local/bin" go install github.com/vasyza/sber-sdk/cmd/sber@latest
   ```

2. Add its directory to the command search path:

   ```sh
   export PATH="$HOME/.local/bin:$PATH"
   ```

   Add the same export line to your shell startup file.

   Use `~/.bashrc` for Bash or `~/.zshrc` for Zsh.
   For a Bash login shell, use `~/.bash_profile`.

3. Check the installed command:

   ```sh
   command -v sber
   sber --help
   ```

`GOBIN` selects the directory where Go installs the executable.
`PATH` lists the directories that the shell searches for commands.
The command is available from any working directory.
Use `sber login` for the first authentication, as described below.
`@latest` selects the latest release, or the default branch when no release tag exists.

## Update the CLI

1. Replace the installed executable:

   ```sh
   GOBIN="$HOME/.local/bin" go install github.com/vasyza/sber-sdk/cmd/sber@latest
   ```

2. Check the command location and help:

   ```sh
   command -v sber
   sber --help
   ```

`command -v sber` must select `$HOME/.local/bin/sber` for this installation.
If it selects another executable, put `$HOME/.local/bin` first in `PATH`.
Run `hash -r` to clear command locations cached by Bash or Zsh.
To read build information, use `go version -m "$(command -v sber)"`.

The update replaces the executable and keeps the saved profile.
If the bank session has expired, use `sber refresh-session`.
If an MCP client keeps `sber` running, restart that client after the update.

### Install the current development version

Use `@main` to select the current development branch:

```sh
GOBIN="$HOME/.local/bin" go install github.com/vasyza/sber-sdk/cmd/sber@main
```

### Install from a source checkout

```sh
git clone https://github.com/vasyza/sber-sdk.git
cd sber-sdk
GOBIN="$HOME/.local/bin" go install ./cmd/sber
```

This procedure also needs Git.
The installed executable is still named `sber`.

### Update a local build

If you run `./bin/sber` from a source checkout, update that checkout:

```sh
git switch main
git pull --ff-only
make build
./bin/sber --help
```

The updated executable is `bin/sber` in that checkout.

## CLI

Cobra v1.10.2 reads CLI arguments and produces help text.
The CLI has 20 operational commands.
The `config` command manages saved CLI settings.
They cover authentication, session restoration, products, cards, history, analytics, private export, and MCP.
Use `sber help COMMAND` to show command options.
Use `--help` or `-h` to show help without profile access or bank requests.

Run `sber login` once in your local Linux or macOS terminal.
The CLI saves a default profile for the current user.
Later commands select that profile automatically.
Enter missing authentication values and the SMS code at the hidden prompts.

```sh
sber login
sber status
sber check-session
sber products
sber operations \
  --from 2026-01-01 --to 2026-01-31 --limit 30 --max-pages 100
```

Use `--profile PATH` to select a different profile for one command.
[Profile locations](docs/CLI.md#default-profile) follow the operating system configuration directory.

An existing profile requires a private parent directory with mode `0700` and a regular private file with mode `0600`.
The `login` command makes missing private directories and refuses to replace an existing profile.
Authentication prepares public configuration before secret prompts.
Authentication values can come from the process environment or an explicit private env file.
Bank secret values are not accepted through command arguments or MCP.
The [operator manual](docs/CLI.md), [command reference](docs/CLI-REFERENCE.md), and [technical terms](docs/CLI-REFERENCE.md#technical-terms) use the [ASD-STE100 Issue 9 writing policy](docs/DEVELOPMENT.md#cli-writing-policy).

### Authentication values

The same input rules apply to primary login, phone login, card login, remembered-device login, and session restoration.

| Environment variable | Value |
| --- | --- |
| `SBER_LOGIN` | Account login. |
| `SBER_PASSWORD` | Account password. |
| `SBER_PINCODE` | Online banking PIN for restoration or new PIN enrollment. |
| `SBER_PHONE` | Phone number for `login --method phone`. |
| `SBER_CARD_NUMBER` | Card number for `login --method card`. |

Export the required variables before running the command, or select a private env file:

```sh
sber login --env-file "$HOME/.config/sber-sdk/credentials.env"
sber login --method card --env-file "$HOME/.config/sber-sdk/credentials.env"
sber refresh-session --env-file "$HOME/.config/sber-sdk/credentials.env"
sber products --env-file "$HOME/.config/sber-sdk/credentials.env"
```

Environment values take priority over file values.
Missing or empty values use hidden terminal input.
An explicit empty environment value overrides the corresponding file value.
SMS codes always use hidden terminal input.
With a configured PIN, restoration and expired-session read renewal can run without a terminal.
If the bank requires SMS confirmation, a command without a terminal stops and keeps the old profile.

The env file must be an owner-controlled regular file with mode `0600` in a directory with mode `0700`.
Paths must have literal, symlink-free components; files with multiple hard links are rejected.
The CLI reads only the selected file, supports literal dotenv assignments, and does not execute or expand its contents.
It does not search for `.env` automatically.
See [authentication values](docs/CLI.md#authentication-values) for file syntax and limits.

### Proxy settings

Save a proxy once for all network commands, including login and MCP:

```sh
sber config set proxy 127.0.0.1:3128:user:password
sber config set proxy http://127.0.0.1:3128:user:password
sber config set proxy https://127.0.0.1:3128:user:password
sber config set proxy socks5://127.0.0.1:1080:user:password
```

These examples show alternative settings.
Replace the address and login values with your proxy values.
Without a scheme, the CLI uses HTTP.
Without `:user:password`, it connects without proxy authentication.
Each new setting replaces the previous address and login values.

```sh
sber config get proxy
sber config list
sber config unset proxy
sber products --proxy socks5://127.0.0.1:1080:user:password
sber products --no-proxy
```

`get` and `list` show the address without login values.
`--proxy` replaces the saved setting for one command.
`--no-proxy` selects a direct connection for one command.
These two options cannot be used together.

The CLI saves settings in `config.json` beside the default profile.
The directory has mode `0700`; the file has mode `0600`.
Proxy passwords in commands can remain in shell history and process arguments.
The CLI removes them from its output and errors.
Environment proxy variables do not select a connection.
If a proxy fails, the request stops without a direct connection.
TLS verification remains enabled.
Read the [proxy procedure](docs/CLI.md#proxy-settings) and [SDK options](docs/SDK.md#proxy-options).

The verified Russian Trusted Root CA is part of the SDK, CLI, and MCP.
Native bank requests require no certificate download or external CA file.
Default trust includes the canonical system PEM bundle when available.
TLS validates the certificate chain and the hostname.
Use `--ca-bundle PATH` to replace default trust for one client.
Read the [certificate source and update instructions](internal/transport/certificates/README.md).

Remembered device login uses a configured PIN or hidden PIN input.
Use `login --remembered-profile EXISTING_PATH --profile NEW_PATH` to make a new profile from an existing identity.
Authentication uses native Go HTTP requests.
No browser or driver installation is required.
Use `login --method phone`, `login --method card`, or `login --method qr` for another login method.
Read the [authentication instructions](docs/AUTH.md).

The `refresh-session` command restores an existing profile through configured or hidden PIN input and optional terminal SMS confirmation.
Reads with a terminal or configured PIN can restore an expired session once, save it, and repeat the read.
Use `--no-renew` to disable restoration during a read.
MCP requires separate session restoration.
The bank controls session lifetime.
A session file does not promise access for a year.

To restore an existing session, use this command:

```sh
sber refresh-session
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

Add the SDK to your Go module:

```sh
go get github.com/vasyza/sber-sdk@latest
```

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

Import `github.com/vasyza/sber-sdk`; its package name is `sber`. Implementations live in separate internal layers. [The runnable example](examples/read/main.go) includes cancellation, cleanup and deliberate JSON export. [Authentication usage](docs/AUTH.md) explains primary login, OTP and remembered-device renewal.

`Decimal` preserves values such as `9007199254740993.10`. Decode response JSON with `sber.DecodeJSON` to retain number lexemes. `ExportJSON` preserves financial quantities and masks card numbers in display text. Diagnostic formatting of session/auth/client values is redacted; explicit credential and financial exports have separate APIs.

## MCP integration

```sh
sber mcp
```

MCP serves six implemented tools against the selected session: setup status, session info/close, products, operations and one operations page. Authentication remains owner-operated. See [MCP setup](docs/MCP.md).

The MCP transport uses the official Go SDK v1.8.0 for MCP 2026-07-28 and legacy 2025-11-25 clients. See [MCP adapter and verification](docs/MCP.md#adapter-profile).

## Build and check

For development, run these commands from the source checkout:

```sh
make check
./bin/sber --help
```

`make check` checks formatting, runs golangci-lint, gopls, vet and the complete race suite, builds all packages and the Sber CLI, and verifies module checksums. Tests use synthetic data and localhost; they need no bank account, Python or installed browser. GitHub Actions contains equivalent Linux/macOS jobs. See the [development guide](docs/DEVELOPMENT.md#local-checks) for lint, gopls, and formatter commands.

## Architecture and verification boundary

Core error, session, transport, authentication and bank-resource packages have separate responsibilities. MCP connects the SDK to local tools through the Sber CLI. See [package responsibilities](docs/ARCHITECTURE.md), [SDK contracts](docs/SDK.md) and [current verification](docs/STATUS.md).

Authentication now uses native Go HTTP requests for login/password, phone/password, card number, QR confirmation, and remembered PIN. The application does not need Playwright, Firefox, Chrome, or a JavaScript runtime. On 2026-10-08, all four initial login methods and PIN restoration completed on the real owner account. The QR session passed all 15 CLI read and local profile commands and the SDK authorization, products, and bounded history checks. See the [verification record](docs/STATUS.md) for current results and limits. Financial mutations, complete history coverage, and independent production review remain outside this evidence. Earlier reports in Git history describe their original snapshots.

## Contribute and report security issues

See [CONTRIBUTING.md](CONTRIBUTING.md) for local checks and contribution rules.
Report vulnerabilities through the private channel in [SECURITY.md](SECURITY.md).
Use synthetic data in issues, tests, and pull requests.

## License

The project uses the [MIT license](LICENSE).
[NOTICE](NOTICE) records upstream attribution.
The adapted CPython behavior retains its [PSF and historical notices](third_party/cpython/LICENSE).
