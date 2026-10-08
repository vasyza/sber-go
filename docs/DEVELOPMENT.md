# Development guide

## Repository structure

| Path | Content |
| --- | --- |
| Root package | Public SDK aliases, function forwards, and Go examples. |
| `internal/` | SDK implementation and private unit tests. |
| `mcp/` | Public SDK integration for local tools. |
| `cmd/` | Executable entry points. |
| `tests/sdk/` | Public SDK contract tests. |
| `tests/cli/` | Compiled CLI tests. |
| `tests/mcp/` | Public MCP package tests. |
| `tests/integration/` | Explicit live integration tests. |
| `testdata/` | Synthetic fixtures and source compatibility records. |
| `docs/` | Current usage, API, and development guides. |

Private unit tests stay in their implementation package to test unexported functions.
Group these tests by behavior in files such as `cookies_test.go` and `lifecycle_test.go`.
Use public APIs for tests under `tests/`.
Keep executable Go examples beside the public package.
Store large data fixtures in `testdata/`.
Preserve test cases and platform build constraints when you combine test files.

## Local checks

1. Install Go 1.27.1.
2. Start the complete checks from the repository root:

   ```sh
   make check
   ```

The checks examine formatting, run golangci-lint, gopls, vet and race tests, build the commands, and verify module checksums.
The default suite uses synthetic data and local servers.
It needs no bank account or installed browser.

GitHub Actions runs the same native checks on Ubuntu and macOS.
Transport and proxy tests use synthetic local TLS servers.

The Makefile sets the golangci-lint version for local checks and CI.
The first lint run downloads the release to `bin/` and verifies its checksum.
This download needs `curl` and an internet connection.
Later runs use the installed release.
The lint checks include files with the `live` build tag.

Run the lint checks separately:

```sh
make lint
```

Run the gopls checks separately:

```sh
make gopls-check
```

The Makefile sets the gopls version for local checks and CI.
The first run installs that version in `bin/` with `go install`.
This installation needs an internet connection.
The checks include source and test files for the current platform, with the `live` build tag.
They do not run live tests.
Warnings, errors, or a failed command stop the checks, even when gopls returns exit code zero.

Apply the configured formatters:

```sh
make fmt
```

Use a package path to select a smaller test group:

```sh
go test -race ./internal/ownerinput
go test -race ./tests/...
go test -run '^TestCLIDocumentation' ./internal/cli
```

The `live` build tag includes the tests under `tests/integration/`.
Those tests skip unless the caller also supplies `-sber-live`.
[STATUS.md](STATUS.md) gives the live test command and its verification limits.

## CLI writing policy

Use [ASD-STE100 Issue 9](https://www.asd-ste100.org/STE_downloads.html), dated 2025-01-15.
The official standard supplies the writing rules and dictionary.
The repository does not redistribute the dictionary.

These rules apply to terminal help, CLI messages, and these guides:

- [CLI.md](CLI.md): operator procedures.
- [CLI-REFERENCE.md](CLI-REFERENCE.md): commands, options, limits, and technical terms.
- [STATUS.md](STATUS.md): verification results and limits.
- This development guide.

Go API notes retain their engineering format.
Code blocks, API paths, JSON fields, and diagnostic labels keep their exact application spelling.
The [technical terms](CLI-REFERENCE.md#technical-terms) define subject words for the CLI.

1. Use approved words with their approved meanings and parts of speech.
2. Use one term for each defined item.
3. Use American English spelling.
4. Write procedure steps in the imperative form.
5. Put a condition before the instruction when it affects the action.
6. Give one action in each procedure sentence.
7. Use no more than 20 words in a procedure sentence.
8. Use no more than 25 words in a descriptive sentence.
9. Use no more than six sentences in a paragraph.
10. Use active voice unless a descriptive sentence needs passive voice.
11. Put instructions and limits outside information-only notes.
12. Use noun groups with no more than three words, except full official names.

Use complete sentences for descriptions and error messages.
Use short noun labels for headings and prompts.
State the condition or action that failed.
Give a permitted next step when it helps the user.
Use fixed argument errors without argument values, input data, profile paths, or private error details.

```text
The command arguments are not valid.
Use sber --help for command help.
```

The documentation suite compares commands and options with the executable registry.
It also checks examples, sentence length, paragraph length, contractions, and procedure structure.
These checks do not establish approved word meanings or full STE compliance.
Examine vocabulary and term use against Issue 9 before you change CLI text.
External certification and independent language review remain outside the verification record.

## Commit messages

Use `type(scope): description` for the subject.
Types include `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `build`, `ci`, `chore`, and `revert`.
Use an optional scope that names the affected package.
Use `!` or a `BREAKING CHANGE:` footer for an incompatible API change.
