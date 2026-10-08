# Verification status

## Configured authentication input — 2026-10-08

All CLI authentication methods accept configured credentials from the process environment or an explicit private env file.
Missing values use hidden terminal input.
SMS codes and financial confirmations remain terminal-only.
A configured PIN permits session restoration and expired-session read renewal without terminal input.

Synthetic tests cover login, phone, card, remembered-device login, PIN enrollment, and session restoration with both credential sources.
They check environment precedence, literal file parsing, private permissions, cancellation, cleanup, secret redaction, and failed publication.
Fixed PIN failures stop without repeated input or another PIN enrollment request.
A required SMS without a terminal stops and preserves the old profile.

The complete local `make check` passed on Linux AMD64 with Go 1.27.1.
It included formatting, golangci-lint, gopls diagnostics, vet, race tests, builds, and module verification.

These checks use synthetic data and local dependencies.
Real bank authentication was not repeated for this change.
The earlier live authentication record remains separate.

## Open source preparation — 2026-10-08

The command and public Go package are named `sber`.
The module path remains `github.com/vasyza/sber-go`.
The README supplies public installation and update commands.

The audited source baseline was commit `7cccdbe`.
Gitleaks 8.30.1 checked all 29 existing commits, including merges and both remote branch histories.
It found no secrets.
The scan also covered the 19 available Actions logs, the closed pull request, and its comments.
The other two Actions runs had no jobs or logs.
Local environment files, profiles, HAR files, and APK files were not tracked.

Govulncheck 1.8.0 reported no vulnerabilities for the checked macOS build with Go 1.27.1.
Runtime dependency license files declare MIT, BSD, or Apache-2.0 terms.
The project retains upstream MIT attribution and the adapted CPython PSF notices.
The bundled CA is a public certificate.

The complete local `make check` passed.
It included golangci-lint, gopls diagnostics, vet, race tests, builds, and module verification.
The baseline also passed the GitHub Linux and macOS jobs.
Bank authentication and financial execution were not repeated for this publication audit.

## SDK repository scope — 2026-10-08

The repository supplies the Sber SDK, its CLI, and local MCP integration.
The `cmd/` directory contains one command entry point: `sber`.
Reference catalogs use relative source paths and retain audited hashes and contract data.

The complete `make check` passed on macOS ARM64 with Go 1.27.1.
It covered formatting, vet, race tests, SDK and CLI builds, and module checksums.
The checks used synthetic data and local servers.
The authentication record below gives the earlier real bank verification.

## Native authentication migration — 2026-10-08

This record describes the current native migration.
The later sections describe checks of earlier repository states.

The SDK and CLI use Go HTTP requests for each authentication method.
Playwright, the browser package, runtime options, and browser installation steps were removed.
Native HTTP, HTTPS, and SOCKS5 proxy support remains available.
TLS chain and hostname verification remain enabled.
The application retains its embedded verified CA.

| Real check on macOS ARM64 | Result |
| --- | --- |
| Login and password, SMS, and online PIN enrollment | PASS. A new native CLI profile was created. |
| Phone and password with SMS | PASS. A new native CLI profile was created. |
| Card number with SMS and online PIN enrollment | PASS. A new native CLI profile was created. |
| QR confirmation in the bank application | PASS. A new native CLI profile was created. |
| Native PIN session restoration | PASS. New primary, card, and default profiles were restored. |
| All 15 CLI read and local profile commands | PASS on the primary, QR, and default sessions. |
| SDK authorization, products, and bounded history | PASS on primary, phone, card, and QR sessions. |

The complete local `make check` passed with Go 1.27.1 on macOS ARM64.
This includes formatting, vet, the full race suite, command builds, and module verification.
Synthetic tests check SRP proofs, RSA encryption, challenge transitions, cancellation, private files, redaction, and failed publication.
The CLI documentation checks passed for command syntax, sentence limits, and paragraph limits.
The CLI dependency graph contains no browser runtime package.
A Linux AMD64 cross-build also passed.

GitHub CI has not run for this working tree.
No independent production review is included in this record.

The bank handoff returned HTTP 500 with the default Go User-Agent.
A declared SDK compatibility header completed the same handoff without a browser.
The native default now includes that header and preserves explicit headers.
A synthetic regression checks its SDK declaration and explicit override behavior.

One phone attempt received the bank-defined `password_bypass` transition.
Synthetic tests now cover that transition at identification and proof verification.
The current successful phone run used SRP and checked the server proof.
The RSA password transition was not separately completed in a real bank session.

A new login ended a previously active web session during these checks.
Each final data check used the applicable current session.
The result does not guarantee a fixed session lifetime.
An expired session requires PIN restoration or another explicit login.

The old default profile received HTTP 403 during PIN restoration.
That failed command preserved its saved profile.
The already validated new primary profile then became the default profile.
PIN restoration and all read checks passed on that default profile.
The bank did not supply the reason for the old profile rejection.

Bank values, profiles, QR images, SMS codes, and response bodies remain outside the repository.
No financial mutation was sent.
Card credential registration and password recovery remain explicit website continuations.
Full history coverage and financial mutation acceptance remain unverified.

## Repository cleanup

The cleanup keeps the same production behavior and test cases.
Public API tests are under `tests/`.
Private unit tests remain beside their implementation and use fewer files.
Large source fixtures are under `testdata/datetime/` and `testdata/financial/`.
The source inventory and manifest are under `testdata/compat/`.

| Check | Result |
| --- | --- |
| Full race suite | PASS. |
| Vet and build | PASS. |
| Formatting and module checksums | PASS. |
| Test declaration and build tag preservation | PASS. All 715 declarations and constraints remain. |
| Fixture preservation | PASS. Moved fixtures retain their original bytes. |
| Documentation links and CLI writing checks | PASS. |
| macOS ARM64 test compilation | PASS. All 20 test packages and the live integration package compile. |

The complete checks ran on Linux AMD64 with Go 1.27.1.
The macOS check compiled tests without execution.

This cleanup adds no bank verification or independent production approval.

## Default profile follow-up — 2026-10-08

The CLI now uses a saved profile for the current operating system user.
Login, reads, session restoration, and MCP use this profile without a path option.
The `--profile PATH` option selects another file for one command.
[CLI.md](CLI.md#default-profile) gives the profile locations.

Linux `make check` passed with Go 1.27.1.
Synthetic tests covered default login, private publication, profile reuse, PIN restoration, and explicit overrides.
The native command tests used isolated home and configuration directories.
They checked profile selection across different command directories and redacted metadata.

Missing and unsafe profiles stop before client creation.
Help and initial argument errors skip default profile resolution.
Export cannot replace its default source profile.
The macOS ARM64 command and CLI tests compiled successfully without test execution.

The live follow-up below checks the default profile on macOS.

## CLI authentication and live follow-up — 2026-10-08

The native macOS ARM64 checks passed with Go 1.27.1.
The checks included race tests, vet, builds, formatting, and module checksums.

Primary login passed with password proof, SMS confirmation, and PIN enrollment.
It created a separate private profile after session validation.
PIN restoration passed for the default profile and the new profile.
The new profile completed the read tests before its restoration check.
The earlier login and restoration failures did not occur in this run.
This result does not identify the cause of those earlier failures.

PIN authentication failures now include the local stage.
Known connection, cookie, response, and challenge failures have fixed diagnostic messages.
The tests check cleanup, secret redaction, and one authentication attempt.

All 15 native read and local profile commands passed on the new profile.
The SDK session, product, and bounded history checks passed.
The MCP stdio check passed all six tools through the official client.
Reads stopped after the MCP session-close call.

The default profile later returned an expired-session result.
PIN restoration recovered that profile without a new password login.
Native reads then checked authorization, products, and details and limits for each returned card.
These commands used the default profile from a different working directory.
The profile file and its parent retained modes 0600 and 0700.

Both financial commands passed their offline plan checks.
Those checks sent no bank request.
Private values and response bodies remain outside this record.

## Proxy settings — 2026-10-08

The CLI supports explicit HTTP, HTTPS, and SOCKS5 proxies with optional authentication.
`config set proxy ADDRESS` saves one setting for network commands.
The `--proxy` and `--no-proxy` options select a connection for one command.
Proxy login values are removed from output and errors.
The private settings file is separate from bank profiles.

Linux `make check` passed with Go 1.27.1.
Synthetic tests covered authenticated proxy connections, TLS rejection, cancellation, and absence of direct fallback.
They covered private atomic settings, file locks, unsafe paths, and configuration precedence.
Compiled CLI tests used synthetic home directories.
SDK renewal tests retained the selected proxy after session restoration.

Firefox option and SOCKS5 adapter tests passed with race checks.
The native Firefox tests compiled locally.
The environment network policy prevented the local Firefox download.
GitHub Actions installs a scoped runtime for native TLS and proxy tests on Ubuntu and macOS.
These tests use local servers and synthetic login values.
No proxy test uses a bank connection or an owner profile.

## Earlier verification

Before this cleanup, local Linux and macOS checks passed with Go 1.27.1.
Those checks included formatting, vet, race tests, builds, and module checksums.
The suites used synthetic data and local servers.
They covered private profile publication, hidden terminal input, transport limits, and public API contracts.
Linux checks also covered inode publication and terminal failure fixtures.

The Cobra migration passed the full Linux race suite, vet, build, module checks, and CLI documentation tests.
The CLI uses Cobra v1.10.2.
The `sber` command has 20 operational commands.

The MCP migration uses the official Go SDK v1.8.0.
It supports protocol revisions `2026-07-28` and `2025-11-25`.
Synthetic tests covered the official client and all six application handlers.
Independent MCP review covered the adapter and application integration.
That review did not approve the complete SDK or live bank behavior.
[MCP.md](MCP.md) gives the protocol limits.

## Real bank verification

Owner-operated macOS checks passed on 2026-10-07.
Primary login used native password proof, SMS confirmation, and five-digit PIN enrollment.
The command validated the session and published a new private profile.
Existing-profile restoration and one interactive expired-read restoration also passed.

The native CLI checks covered all 15 read and local profile commands.
They included card details, limits, analytics, operation details, and private export.
The SDK check covered warm-up, typed products, and one history page.
That page used a limit of five and a seven-day window.
The bounded CLI history used a page cap of two.

These checks used the embedded CA and ordinary public browser initialization.
TLS checked certificate chains and hostnames.
Connection setup allowed up to three attempts before HTTP transmission.
Transmitted financial and authentication requests retained their no-replay policy.
Private credentials, identifiers, and response bodies remain outside repository records.

## Repeat the bounded live test

1. Complete owner login or session restoration in your local terminal.
2. Start both read tests with the selected private profile:

   ```sh
   go test -tags=live -run '^TestLive(CLIReadOnly|ReadOnly)$' -count=1 -v ./tests/integration \
     -args -sber-live -sber-profile "$HOME/.local/share/sber-go/session.json"
   ```

Without `-sber-live`, these tests skip before profile access, builds, or bank requests.
The native command test supplies `--no-renew` to bank reads.
The command does not collect secret input or start a financial operation.

## Acceptance limits

Complete history coverage remains `UNKNOWN`.
Exact financial values were not reconciled with the bank interface.
Real card renaming, transfers, and payment SMS confirmation were not tested.
Real Linux account access was not tested.
The recorded bank checks do not prove current session validity.

Remote CI has not run for the local follow-up edits.
A new independent production review is outside this record.
Full upstream parity and external STE certification remain unverified.

## Historical records

Old review logs, repair receipts, and handoff notes are available in
[Git history](https://github.com/vasyza/sber-go/tree/71cd6ccc3ba2a3a2780810e655b21ea7494ec4b6/docs).
The old root migration log is also available in
[Git history](https://github.com/vasyza/sber-go/blob/71cd6ccc3ba2a3a2780810e655b21ea7494ec4b6/MIGRATION.md).
Their failures and review decisions apply to the original snapshots.
Removing duplicate documents does not change those decisions.
The frozen upstream inventory remains reference evidence in `testdata/compat/parity.json`.
Its pending statuses do not describe the current Go test results.
