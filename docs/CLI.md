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

For installation and updates, follow the [README instructions](../README.md#install-the-cli).
The procedure below builds an executable in the source checkout.

Requirements:

- Linux or macOS.
- Go 1.27.1, as specified in `go.mod`.
- A local terminal for missing authentication values and SMS codes.
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

## Proxy settings

The CLI supports HTTP, HTTPS, and SOCKS5 proxies.
Each type supports connections with or without proxy authentication.
The saved setting applies to login, session restoration, network reads, and MCP.

1. Save the proxy address and its login values:

   ```sh
   ./bin/sber config set proxy 127.0.0.1:3128:user:password
   ```

2. Read the saved address:

   ```sh
   ./bin/sber config get proxy
   ```

3. Read all saved settings:

   ```sh
   ./bin/sber config list
   ```

Without a scheme, the CLI uses HTTP.
Use one of these alternative settings to select a scheme:

```sh
./bin/sber config set proxy http://127.0.0.1:3128:user:password
./bin/sber config set proxy https://127.0.0.1:3128:user:password
./bin/sber config set proxy socks5://127.0.0.1:1080:user:password
```

The syntax is `[scheme://]host:port[:username:password]`.
Replace the example address and login values with your proxy values.
Omit `:username:password` for a proxy without authentication.

Put brackets around an IPv6 address, as in `[::1]:1080`.
The password can contain colons.
If a value contains shell special characters, quote the complete value.
The CLI does not accept the `username:password@host` form.

`get` and `list` remove login values from their output.
A new setting replaces the previous address and login values.
The command writes no secret values to its output or errors.
The shell can retain command values in its history.
Other local processes can read process arguments.

The settings file is separate from the bank profile.
It is `config.json` in the same default user configuration directory.
The directory must have mode `0700`; the file must have mode `0600`.
The file contains proxy login values when authentication is selected.
Writes use a private lock and atomic replacement.
Help and offline metadata commands do not read this file.

To select a proxy for one command, use `--proxy`:

```sh
./bin/sber products --proxy socks5://127.0.0.1:1080:user:password
```

This option replaces both the saved address and its login values.
An explicit proxy does not read the saved settings.
To connect directly for one command, use `--no-proxy`:

```sh
./bin/sber products --no-proxy
```

The two options cannot be used together.
Without a saved or explicit proxy, network commands connect directly.
Environment proxy variables do not select a connection.
If a proxy fails, the request stops without a direct connection.
TLS validates both the destination and any HTTPS proxy.

To remove the saved proxy and its login values:

```sh
./bin/sber config unset proxy
```

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
The command stops before secret input if public preparation fails.
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

## Authentication values

All login methods and session restoration use the same authentication input rules.

| Variable | Value |
| --- | --- |
| `SBER_LOGIN` | Account login for the default login method. |
| `SBER_PASSWORD` | Account password for login or phone authentication. |
| `SBER_PINCODE` | Online banking PIN for restoration or new PIN enrollment. |
| `SBER_PHONE` | Phone number for `--method phone`. |
| `SBER_CARD_NUMBER` | Card number for `--method card`. |

The CLI first reads the process environment.
If a variable is absent, it reads the value from the selected env file.
If the selected value is empty or missing, it shows a hidden terminal prompt.
An explicit empty environment value selects terminal input instead of a file value.
The CLI always reads SMS codes and financial confirmations from the terminal.

To use a private env file:

1. Make a credentials file in a directory that you own.
2. Replace these example values with your authentication values:

   ```dotenv
   SBER_LOGIN='YOUR_LOGIN'
   SBER_PASSWORD='YOUR_PASSWORD'
   SBER_PINCODE='YOUR_ONLINE_BANKING_PIN'
   ```

3. Set the parent directory mode to `0700`.
4. Set the file mode to `0600`.
5. Select the file explicitly:

   ```sh
   ./bin/sber login --env-file "$HOME/.config/sber-go/credentials.env"
   ```

6. If the bank requests SMS confirmation, enter the code at the hidden prompt.

For card authentication, supply `SBER_CARD_NUMBER` and select the card method:

```sh
./bin/sber login --method card --env-file "$HOME/.config/sber-go/credentials.env"
```

The file must be a regular file that you own, with only one hard link.
Each path component must be literal and must not be a symbolic link.
The CLI rejects unsafe files before authentication starts.
It does not search for `.env` files automatically or change the process environment.

The file supports one `NAME=VALUE` assignment per line, an optional `export` prefix, quotes, and comments.
Single quotes preserve their contents.
Double quotes support escaped quotes and backslashes.
An unquoted hash starts a comment at the start of a value or after a space.
Values remain literal: the CLI does not expand variables or execute commands.
Multiline values are not supported.

The file limit is 64 KiB.
Each authentication value has a limit of 4096 bytes and cannot contain null or newline characters.
Unrelated variable names are ignored.
For repeated assignments, the last value applies.
PIN input must contain 4 through 12 digits and match the bank configuration.

`SBER_PINCODE` also supplies new PIN enrollment and its confirmation when the bank requires enrollment.
If a configured PIN is invalid or rejected, the command stops without sending the same PIN again.
Credentials do not become profile fields or command output.
Keep the environment and credentials file private.

## Native authentication

All authentication requests use the native Go HTTP transport.
The CLI needs no browser, driver, or JavaScript runtime.
The application retains certificate and hostname verification.
A bank security check can require interactive browser access.
The native command stops at that check.

## Select another login method

Use a new profile path for each procedure below.
The CLI does not replace an existing profile during login.

### Phone and password

1. Start phone authentication:

   ```sh
   ./bin/sber login --method phone --profile "$HOME/.config/sber-go/phone.json"
   ```

2. Enter the bank phone number at the hidden prompt.
3. Enter the online banking password at the hidden prompt.
4. Enter the SMS code if the bank requests it.
5. Make sure that the result contains `"profile_created":true`.

Use an 11-digit phone number that starts with 7.
Spaces, parentheses, hyphens, and a leading plus sign are permitted.
The CLI normally uses SRP for the password proof.
It validates the server proof before SMS confirmation.
If the bank requests another password method, the CLI uses that method through verified TLS.
If the bank supplies an RSA key, the CLI encrypts the password with that key.

### Card number

1. Start card authentication:

   ```sh
   ./bin/sber login --method card --profile "$HOME/.config/sber-go/card.json"
   ```

2. Enter the complete card number at the hidden prompt.
3. Enter the SMS code from the bank.
4. Enter and confirm a new online banking PIN if the bank requests it.
5. Make sure that the result contains `"profile_created":true`.

The CLI encrypts the card number with the bank public RSA key.
The command does not request a CVV, card PIN, or expiry date.
If account registration or credential recovery is necessary, complete it on the bank website.
The CLI does not reset a login or password as part of card authentication.

### QR code

1. Start QR authentication:

   ```sh
   ./bin/sber login --method qr --profile "$HOME/.config/sber-go/qr.json"
   ```

2. Open the bank application on your phone.
3. Scan the QR code in the terminal with that application.
4. Confirm the login in the application.
5. Make sure that the result contains `"profile_created":true`.

The command waits for a maximum of three minutes.
An expired QR or refused login stops the command.
The command does not start another QR automatically.
Treat the displayed QR as an authentication secret.
Do not share it or record the terminal output.

For a PNG image, select a new absolute private file path:

```sh
./bin/sber login --method qr --qr-output "$HOME/.config/sber-go/login-qr.png" \
  --profile "$HOME/.config/sber-go/qr.json"
```

The parent directory must be private.
The file has mode `0600` and is published without replacement.
Open the PNG locally and scan it in the bank application.
Delete the PNG after the login ends.

PIN restoration requires a PIN enrolled for the saved device identity.
A phone or QR session can lack that enrollment.
For such a profile, start its login method with a new private profile path.

## Restore a session

Use this procedure when the session expires.
The existing profile must contain the remembered device identity.
The PIN must be the PIN that the bank accepts for that identity.

1. Start session restoration:

   ```sh
   ./bin/sber refresh-session
   ```

2. If a hidden PIN prompt appears, enter the online banking PIN.
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

A data read with a terminal or configured PIN can restore an expired session once.
It reads the configured PIN or shows a PIN prompt.
It shows an SMS prompt if the bank requires confirmation.
It saves the validated session before it sends the data request again.
The `--no-renew` option disables this behavior.

A configured PIN lets session restoration run without terminal input:

```sh
./bin/sber refresh-session --env-file "$HOME/.config/sber-go/credentials.env"
./bin/sber products --env-file "$HOME/.config/sber-go/credentials.env"
```

Exported `SBER_PINCODE` also supplies these commands without `--env-file`.
If the bank requests an SMS code without a terminal, the command stops and keeps the old profile.
A batch read without a configured PIN does not restore an expired session.
MCP does not do interactive authentication.
Commands that change bank data do not restore a session or send a failed operation again.

The bank can end a session at any time.
A new login can end another web session.
Check the selected profile after each login.
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

2. If a hidden PIN prompt appears, enter the PIN.
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
| Interactive security check | Complete bank access in the website. Native authentication cannot continue through this check. |
| HTTP error or bank rejection | Read the verification record before another attempt. |
| Unsupported response format | Update the application for the bank protocol. |
| Authentication attempt limit | Stop login attempts and use the bank website. |
| Unknown operation result | Find the result in the bank website before another action. |

The CLI shows known error classifications only.
It does not show remote error text, support IDs, cookies, or secret input.
Authentication accepts configured environment values or an explicit private env file.
Secret values are not accepted through command options or MCP arguments.
SMS codes and financial confirmations require hidden terminal input.
Proxy login values use the explicit proxy address syntax.

Authentication errors include the local stage.
The stage identifies public configuration, owner input, credentials, PIN login, SMS confirmation, PIN enrollment, session validation, or cleanup.

## Select TLS trust

The default trust contains the embedded bank root and available canonical system PEM certificates.
The `--ca-bundle` option replaces that trust for one client.
For an invalid explicit bundle, the CLI stops without fallback to default trust.
The application does not modify the system certificate store.

Native connections use verified TLS and HTTP/1.1.
Direct TLS setup can make up to three connection attempts after a network closure.
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
