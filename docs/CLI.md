# sber CLI

The `sber` command checks a local profile.
It does not make bank requests.
The CLI uses [Cobra v1.10.2](https://github.com/spf13/cobra/releases/tag/v1.10.2).
This version was the latest stable release on 7 October 2026.

Use Linux for the profile file safety checks.
Use Go 1.27.1 to build the command.

## Build and show help

1. Build the command.

   ```sh
   go build -o bin/sber ./cmd/sber
   ```

2. Show command help.

   ```sh
   ./bin/sber --help
   ./bin/sber status --help
   ./bin/sber inspect-session --help
   ```

You can use `-h` instead of `--help`.
You can also use `sber help`, `sber help status`, or `sber help inspect-session`.
A help request does not read a profile.

## Commands

Use `--profile PATH` to give the profile file path.
Both profile commands need this option and a path.
You can also use `--profile=PATH`.

```sh
./bin/sber status --profile /explicit/private/path/profile.json
./bin/sber inspect-session --profile /explicit/private/path/profile.json
```

| Command | Function |
| --- | --- |
| `status` | Check the profile file properties. |
| `inspect-session` | Show the properties of a private profile. |

The `status` command checks file type, ownership, permissions, and links.
It does not read the file contents or make a profile.
An absent profile gives `profile_exists: false`.
The command does not make missing directories.

The `inspect-session` command accepts profile versions 1 through 4.
The output contains the cookie count and information about which deviceprints exist.
The output does not contain cookie values, deviceprint values, or tokens.
A successful profile read does not prove current bank access.

The CLI does not search for owner profiles.
Do not put a login, password, or PIN in command arguments.
The CLI does not read secrets from environment variables or standard input.
The CLI rejects commands and options that it does not support.
It also rejects shell completion commands.

Bank login, balances, bank operations, profile import, HAR import, and MCP service commands remain unavailable.
Use the separate [rental-check command](RENTAL-CLI.md) for an offline ledger preview.

## Output and exit codes

The CLI writes command results as JSON to standard output.
It writes help text to standard output and error messages to standard error.
Error messages do not contain argument values, profile paths, or private error details.

All command results contain `bank_authorization_checked: false`.
The `command` field contains the command name.
The `status` result also contains `profile_exists`.
The `inspect-session` result also contains `metadata`.

| Exit code | Meaning |
| --- | --- |
| 0 | The command or help request is complete. |
| 2 | The command arguments, context, or output are not valid. |
| 3 | A profile check, profile read, or output write failed. |

For an argument error, the CLI writes:

```text
The command arguments are not valid.
Use sber --help for command help.
```

## Development checks

Run these checks with synthetic data:

```sh
go test -race ./internal/cli ./internal/command
go vet ./internal/cli ./internal/command ./cmd/sber
go build -o bin/sber ./cmd/sber
```

Follow the [CLI writing rules](CLI-WRITING.md) when you change help text or messages.
The earlier CLI guide remains in Git history.
See [HANDOFF.md](HANDOFF.md) for the SDK issues and the limits of earlier reviews.
The Cobra migration does not give production approval for the SDK.
