# Sber CLI operator manual

This manual uses ASD-STE100 Issue 9 as its writing standard.
The [command reference](CLI-REFERENCE.md) gives the options for each command.
The [term list](CLI-REFERENCE.md#technical-terms) defines the technical words in this manual.

## Purpose and limits

The CLI supplies the implemented APIs of the unofficial Sber SDK.
It uses the saved default profile for the current user.
Use `--profile PATH` to select a different profile.
The bank controls authorization, data availability, and session lifetime.

The default mode lets the CLI read data.
Commands that change bank data first show an offline plan.
To change bank data, you must supply `--execute` and give terminal confirmation.
MCP supplies six tools for data reads and local session control.

A session file does not show whether the bank will accept a request.
Local tests use synthetic data and local servers.
The [verification record](STATUS.md) gives the results for real bank requests.

## Install the CLI

Requirements:

- Linux or macOS.
- Go 1.27.1, as specified in `go.mod`.
- A local terminal for hidden input.
- Your own account on the bank website.

1. Open a terminal in the repository directory.
2. Build the CLI:

   ```sh
   make build
   ```

3. Read the help:

   ```sh
   ./bin/sber --help
   ```

4. To read the options for a command, use its help:

   ```sh
   ./bin/sber help operations
   ```

The `bin/sber` file is the CLI executable.
The application includes the verified Russian Trusted Root CA.
It does not download a certificate at startup.
TLS validates the certificate chain and the server hostname.

## Command syntax

Cobra v1.10.2 reads the command arguments and produces help text.
Operational commands use the default profile when you omit `--profile PATH`.
Use two hyphens for option names.
Old forms such as `-profile` are not valid.

Use `sber --help` to show the command catalog.
Use `sber help COMMAND` or `sber COMMAND --help` to show command options.
Use `-h` as the short form of `--help`.
A help request does not read a profile or send a bank request.

The CLI writes help to standard output.
The CLI writes fixed argument errors to standard error.
These errors do not include argument values or private paths.
The CLI does not supply shell completion commands.

## Default profile

The CLI selects one default profile for the current operating system user.
The command directory does not affect this selection.

| Platform | Default profile |
| --- | --- |
| Linux | `~/.config/sber-go/profile.json`. |
| macOS | `~/Library/Application Support/sber-go/profile.json`. |

On Linux, an absolute `XDG_CONFIG_HOME` value replaces `~/.config`.
The remaining path is `sber-go/profile.json`.
The CLI makes missing private directories during login.
Help and status do not make profile directories.

If no default profile exists, data commands stop with a login instruction.
The CLI does not start authentication during those commands.
Use `sber login` to make the default profile.

## Make a profile

The profile contains session cookies and device identity.
It does not contain your password, PIN, or SMS code.
The CLI makes missing private directories.
The profile must be a regular file with mode `0600` in a directory with mode `0700`.

1. Start login:

   ```sh
   ./bin/sber login
   ```

2. At the `Login` prompt, enter your account login.
3. At the `Password` prompt, enter your password.
4. If the bank shows an SMS challenge, enter the code at the hidden prompt.
5. If the bank shows PIN enrollment, enter the specified number of digits.
6. At the confirmation prompt, enter the same new PIN.
7. Make sure that the result contains `"profile_created":true`.

The CLI loads public configuration before it shows a secret prompt.
Browser preparation stops before secret input if the selected runtime cannot start.
The online banking PIN is different from a card PIN.
The CLI shows the specified PIN length before input.
An incorrect length or confirmation does not cause a new login attempt.

If the bank gives a definite PIN policy rejection, the CLI shows new PIN prompts in the same login process.
Each bank attempt must have new owner input and confirmation.
The CLI stops after three bank rejections.
It does not send a PIN again after a connection error or unknown result.

The command does not replace an existing profile.
If the default profile already exists, use `sber refresh-session` to restore it.
A private lock prevents simultaneous enrollment at the same path.
Publication occurs only after successful authentication and authentication cleanup.

If the bank shows CAPTCHA or WebAuthn, complete that step in the bank website.
The CLI stops at these challenges.
It does not send a password, PIN, or SMS attempt again automatically.

## Restore a session

Use this procedure when the session expires.
The existing profile must contain the remembered device identity.
The PIN must be the PIN that the bank accepts for that identity.

1. Start session restoration:

   ```sh
   ./bin/sber refresh-session
   ```

2. At the hidden prompt, enter the online banking PIN.
3. If the bank shows an SMS challenge, enter the code from that SMS.
4. Make sure that the result contains `"session_refreshed":true`.
5. Do a session check:

   ```sh
   ./bin/sber check-session --no-renew
   ```

If authentication or cleanup stops with an error, the CLI keeps the old profile.
A successful command writes the new session to the same private file.
The write is atomic.
If publication status is unknown, examine the profile before another attempt.

An interactive data read can restore an expired session once.
It shows a PIN prompt and an SMS prompt, if necessary.
It saves the validated session before it sends the data request again.
The `--no-renew` option disables this behavior.

A batch command without terminal input does not show secret prompts.
MCP does not do interactive authentication.
Commands that change bank data do not restore a session or send a failed operation again.

The bank can end a session at any time.
A session can end less than one year after login.
The result of a session check applies to that request only.

If the bank no longer accepts the device identity or PIN, use `login` with a new profile path.
If the bank changes its protocol, update the application before another attempt.

## Select another profile

The `--profile PATH` option selects a different private file for one command.
It applies to that command only.
Use the same option for login, data reads, and restoration of that profile.

```sh
./bin/sber status --profile /absolute/private/path/profile.json
```

## Copy a remembered identity

This procedure makes a new profile through PIN authentication.
The source profile remains at its original path.

1. Start PIN login with a new destination:

   ```sh
   ./bin/sber login \
     --remembered-profile "$HOME/.local/share/sber-go/session.json" \
     --profile "$HOME/.local/share/sber-go/session-next.json"
   ```

2. Enter the PIN at the hidden prompt.
3. If the bank shows an SMS challenge, enter the code from that SMS.
4. Make sure that the result contains `"profile_created":true`.

## Read bank data

1. Read the products:

   ```sh
   ./bin/sber products --no-renew
   ```

2. Select the applicable identifiers from that result.
3. Read data with the applicable command:

   ```sh
   ./bin/sber accounts --no-renew
   ./bin/sber cards --no-renew
   ./bin/sber portfolio --no-renew
   ./bin/sber card-info --card-id 123 --no-renew
   ./bin/sber card-limits --card-id 123 --no-renew
   ```

The value `123` is a synthetic example.
The card ID is the product identifier, not the card number.
`portfolio` reads products once and keeps the account relationships in that response.
An absent limit appears as `null`; the actual card limits remain unknown.

## Read operation history

1. Select an explicit date range.
2. Read the history:

   ```sh
   ./bin/sber operations \
     --from 2026-08-01 --to 2026-08-31 --limit 30 --max-pages 100 --no-renew
   ```

3. Read the coverage metadata with the operations.
4. To read one page, use `operations-page`:

   ```sh
   ./bin/sber operations-page \
     --from 2026-08-01 --to 2026-08-31 --limit 30 --offset 0 --no-renew
   ```

5. To read an operation, use its `id`:

   ```sh
   ./bin/sber operation-details \
     --operation-id EXAMPLE_OPERATION --no-renew
   ```

Dates use the `YYYY-MM-DD` format.
Timestamps can include a UTC offset.
A date or timestamp without an offset uses Moscow time.
Both boundaries are inclusive.
Without dates, history uses a fixed 120-day window in Moscow time.

`operations` removes duplicate operation IDs and orders the result by date.
A page cap or request failure gives an error without a successful partial result.
History coverage can remain incomplete after an empty final page.
`WindowCompleteness` remains `unknown` without independent coverage evidence.
If history is incomplete, an absent payment remains unknown.

## Read analytics

1. Select the date range and the income type.
2. Read the totals:

   ```sh
   ./bin/sber analytics \
     --from 2026-08-01 --to 2026-08-31 --income-type outcome \
     --between-own=false --no-renew
   ```

The `outcome` value selects expenditure.
The `income` value selects income.
Analytics sends the time boundaries with the `+03:00` offset.
The command reference gives the inclusion and display options.

## Inspect and export a profile

1. Read file metadata without a bank request:

   ```sh
   ./bin/sber status
   ```

2. Read profile metadata with secret values removed:

   ```sh
   ./bin/sber inspect-session
   ```

3. Read credential metadata with secret values removed:

   ```sh
   ./bin/sber inspect-credentials
   ```

4. Write a private session copy to a new file:

   ```sh
   ./bin/sber export-session \
     --destination "$HOME/.local/share/sber-go/session-copy.json"
   ```

`status` does not read the profile contents or make directories.
`inspect-session` reads the profile but does not contact the bank.
Credential metadata does not contain usable cookie values.
`export-session` contains secret session values in the destination file only.
It does not replace an existing destination.

## Change bank data

A plan contains only the supplied command values.
It does not show whether the bank will accept the change.
The default plan sends no bank request and does not open the profile.

1. Show a card name change plan:

   ```sh
   ./bin/sber card-rename \
     --card-id 123 --name "Travel card"
   ```

2. Make sure that the plan contains the correct card ID and name.
3. To let the CLI change the name, add `--execute` to the same command.
4. At the hidden prompt, type `CONFIRM`.

1. Show a transfer plan:

   ```sh
   ./bin/sber transfer-own \
     --source account:SOURCE_ID --destination account:DESTINATION_ID \
     --amount 10.50 --currency RUB
   ```

2. Make sure that the source, destination, amount, and currency are correct.
3. To let the CLI start the transfer, add `--execute` to the same command.
4. At the first hidden prompt, type `CONFIRM` to start workflow preparation.
5. Read the plan after preparation.
6. At the second hidden prompt, type `CONFIRM` to send the final transfer.

The transfer workflow remains in one CLI process.
Preparation does not itself move money.
The SDK uses the recorded confirmation sequence without a payment SMS challenge.
An unknown confirmation branch gives exit code `4`.

If exit code `4` occurs, do not start the command again.
Use the bank website to find the operation result first.
The CLI does not send an uncertain financial request again.

## Start MCP

1. Restore the profile in a terminal before the MCP client starts.
2. Set the MCP client to start this command:

   ```sh
   ./bin/sber mcp
   ```

3. Use the tools listed in [MCP.md](MCP.md).

MCP uses standard input and standard output for its protocol only.
It does not write terminal prompts to that stream.
For an expired MCP session, restore the session in a terminal and start the server again.
Closing a local client does not log out the account at the bank.

## Output and errors

Ordinary commands write one JSON result to standard output.
Diagnostics and confirmation plans go to standard error.
Exit code `0` shows success for the specified command.
A successful offline command does not show bank authorization.

Amounts are exact decimal strings.
The CLI does not convert them to binary floating-point numbers.
Product IDs and operation IDs remain usable in subsequent commands.
Display text masks card numbers.
Data output can contain private financial information.

| Exit code | Meaning | Action |
| --- | --- | --- |
| `0` | Command success. | Read the result and its coverage metadata. |
| `2` | Invalid command, options, or initial context. | Read `sber help COMMAND`. |
| `3` | Input, authentication, bank, storage, cleanup, or output error. | Use the diagnostic to select the procedure below. |
| `4` | Unknown result after an operation that changes bank data. | Read the result in the bank website before another action. |
| `130` | Cancellation during execution. | Examine any publication result before another login attempt. |

| Diagnostic class | Action |
| --- | --- |
| Expired session | Use the session restoration procedure. |
| Existing destination | Select a new path. |
| Unsafe private file | Make sure that the file and directory permissions are correct. |
| Invalid CA bundle | Select a readable PEM file with verified trust anchors. |
| Untrusted TLS certificate | Update the application or select a verified CA bundle. |
| TLS hostname or validity error | Make sure that the destination and system time are correct. |
| Browser initialization error | Make sure that all three browser paths are correct. |
| HTTP error or bank rejection | Read the verification record before another attempt. |
| Unsupported response format | Update the application for the bank protocol. |
| Authentication attempt limit | Stop login attempts and use the bank website. |
| Unknown operation result | Find the result in the bank website before another action. |

The CLI shows known error classifications only.
It does not show remote error text, support IDs, cookies, or secret input.
It does not read secrets from `.env`, environment variables, command options, or MCP arguments.
Primary login errors include the local authentication stage.
The stage identifies public configuration, owner input, credentials, SMS confirmation, PIN enrollment, session validation, or cleanup.

## Select TLS trust

The default trust contains the embedded bank root and available canonical system PEM certificates.
The `--ca-bundle` option replaces that trust for one client.
For an invalid explicit bundle, the CLI stops without fallback to default trust.
The application does not modify the system certificate store.

Native connections use verified TLS and HTTP/1.1.
TLS setup can make up to three connection attempts after a network closure.
All attempts use the same request timeout.
The client sends application data only after successful TLS validation.
A certificate error stops connection setup.
A connection error after HTTP transmission does not start another HTTP attempt.

1. Compare the selected CA fingerprint with the fingerprint from its verified source.
2. Select the PEM bundle:

   ```sh
   ./bin/sber check-session \
     --ca-bundle /absolute/verified/trust.pem --no-renew
   ```

The [certificate record](../internal/transport/certificates/README.md) gives the source, fingerprint, validity, and update procedure.

## Use public browser initialization

Use this procedure when the bank makes ordinary browser initialization necessary before native authentication.
The browser receives no login, password, PIN, or SMS input from the CLI.
It renders the public login document only.

Requirements:

- The Firefox build for the Playwright version in `go.mod`.
- The installed Playwright driver for that build.
- A dedicated private Firefox profile with verified NSS certificate trust.
- Three absolute paths for the browser profile, driver, and executable.

1. Prepare the runtime and trust as specified in [AUTH.md](AUTH.md).
2. Select all three paths for session restoration:

   ```sh
   ./bin/sber refresh-session \
     --browser-profile /absolute/private/firefox-profile \
     --playwright-driver /absolute/installed/playwright-driver \
     --firefox-executable /absolute/installed/firefox
   ```

3. Enter the PIN at the hidden terminal prompt.
4. If the bank shows an SMS challenge, enter the code from that SMS.

The same options apply to `login` and interactive data reads.
The Go CA bundle does not configure the Firefox NSS store.
TLS verification and the browser sandbox remain enabled.
