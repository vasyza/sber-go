# Verification status

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

Real bank verification remains limited to the earlier results below.

## Earlier verification

Before this cleanup, local Linux and macOS checks passed with Go 1.27.1.
Those checks included formatting, vet, race tests, builds, and module checksums.
The suites used synthetic data and local servers.
They covered private profile publication, hidden terminal input, transport limits, and public API contracts.
Linux checks also covered inode publication and terminal failure fixtures.

The Cobra migration passed the full Linux race suite, vet, build, module checks, and CLI documentation tests.
Both commands use Cobra v1.10.2.
The `sber` command has 20 operational commands.

The MCP migration uses the official Go SDK v1.8.0.
It supports protocol revisions `2026-07-28` and `2025-11-25`.
Synthetic tests covered the official client and all six application handlers.
Independent MCP review covered the adapter and application integration.
That review did not approve the complete SDK or live bank behavior.
[MCP.md](MCP.md) gives the protocol limits.

Earlier rental review covered the frozen offline engine.
A later CLI review covered the timestamp input repair.
Those records apply to their recorded source hashes.
The Cobra migration changes parser and message source bytes.
Earlier approval does not automatically cover changed bytes.

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
Real Linux account access and real MCP-to-bank calls were not tested.
The recorded bank checks do not prove current session validity.

Remote CI and a new independent production review are outside this record.
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
