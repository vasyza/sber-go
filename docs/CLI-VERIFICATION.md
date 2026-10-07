# CLI verification record

Date: 2026-10-07.
This record applies to the implemented command catalog on `refactor/modular-sdk`.
The [command reference](CLI-REFERENCE.md) maps those commands to SDK methods and routes.

## Local verification

| Check | Result | Boundary |
| --- | --- | --- |
| macOS ARM64, Go 1.27.1: `make check`. | PASS. | Formatting, vet, full race suite, package builds, both commands, and module checks. |
| Linux ARM64, official `golang:1.27.1`. | PASS. | Full vet and race suite, package builds, CLI build and help, and module checks; network disabled. |
| Command catalog and options. | PASS. | All commands use the same option registry as executable help. |
| Authentication and cleanup. | PASS. | Synthetic primary, PIN, SMS, PIN enrollment, public preparation, and cleanup outcomes. |
| Hidden terminal input. | PASS. | Real local terminal descriptors with synthetic secret values. |
| Private session publication. | PASS. | Real local files, private permissions, locks, atomic writes, and refusal to replace new destinations. |
| Card API request contract. | PASS. | Card IDs use JSON numbers; numeric bounds remain enforced. |
| TLS establishment. | PASS. | HTTP/1.1 negotiation, bounded recovery before HTTP, shared timeout, certificate rejection, cancellation, and no POST replay. |
| Mutation plans and confirmation. | PASS. | Synthetic bank requests; default plans send no request. |
| Transfer uncertainty and cancellation. | PASS. | Synthetic workflow guards; no financial request replay. |
| Operator documentation. | PASS. | Command names, options, parsed examples, sentence limits, paragraph limits, and contractions. |

The local suite uses no bank credentials or owner data.
The live tests are separate from this suite.

## Real session verification on macOS

The owner supplied all secret input through local hidden terminal prompts.
The CLI used the embedded root CA and ordinary public Firefox initialization.
TLS validation and the browser sandbox stayed enabled.

| Check | Result |
| --- | --- |
| Existing-profile restoration with `refresh-session`. | PASS. |
| Interactive `check-session` after expiration, with one restoration and one read retry. | PASS. |
| Native CLI E2E for all read and local profile commands below. | PASS. |
| SDK authorization, products, and one history page. | PASS. |
| Primary login into a new profile, with password proof, SMS, and PIN enrollment. | PASS. |

The final native CLI and SDK E2E both passed on the new primary profile.

The native CLI E2E covered these commands:

- `check-session`, `status`, `inspect-session`, and `inspect-credentials`.
- `products`, `accounts`, `cards`, and `portfolio`.
- `card-info` and `card-limits` for a selected returned card ID.
- `operations-page`, `operation-details`, and `operations` for a seven-day window.
- `analytics` for that window.
- `export-session` into a private temporary directory.

The test used a history page limit of five.
History collection used a page limit of 100 and a cap of two pages.
An available operation supplied the ID for the details command.
The test kept identifiers and financial results in transient memory.
It wrote only command stages and counts to test output.
The temporary session copy had mode `0600` and was removed after the test.

Card details initially gave HTTP 500 because the request used string IDs.
The bank interface used JSON numbers for the same route.
The corrected numeric request passed the real CLI E2E.

An earlier E2E stopped during history collection.
A separate history request and the final E2E passed without another login.
That first failure remains unclassified; it does not show a fixed session lifetime.
The CLI does not send an HTTP request again after a connection error.

Later read checks stopped during TLS setup before HTTP transmission.
A separate public TLS probe showed connection resets without application requests.
The client now supplies HTTP/1.1 through ALPN and bounds recovery before HTTP to three connection attempts.
All attempts share one timeout, and certificate failures stop immediately.
Local TLS tests show exactly one POST after connection recovery.
They also show no replay after a transmitted POST loses its response.

The final E2E passed after these changes.

## Repeat the bounded live test

1. Complete owner login or session restoration in your local terminal.
2. Start both read tests with the selected private profile:

   ```sh
   go test -tags=live -run '^TestLive(CLIReadOnly|ReadOnly)$' -count=1 -v ./integration \
     -args -sber-live -sber-profile "$HOME/.local/share/sber-go/session.json"
   ```

Without `-sber-live`, the tests skip before profile access, builds, or bank requests.
The native command test supplies `--no-renew` to bank reads.
It does not collect secret input or initiate a financial operation.

## Acceptance limits

The implemented routes form the SDK catalog; they do not include every bank API.
Real card renaming, transfers, and payment SMS confirmation were not tested.
Real Linux account access and a real MCP client were not tested.
Complete history coverage remains `UNKNOWN`.
Exact financial values were not reconciled with the bank interface.
This record does not include remote CI, independent production review, or external STE certification.

Earlier primary attempts stopped before profile publication.
The final owner-operated primary command published a validated private profile.
Public preparation now precedes secret prompts, and known failures show a local stage.
The writing rules and technical term policy are in [CLI-STYLE.md](CLI-STYLE.md).
Historical review results remain in [STATUS.md](STATUS.md) and [HANDOFF.md](HANDOFF.md).
