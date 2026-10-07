# CLI command reference

The [technical term section](#technical-terms) defines the terms in this reference.
Use `sber help COMMAND` to read executable help without a profile or bank request.
For unknown options or extra positional arguments, the CLI stops before it opens a client.

## Syntax

```text
sber COMMAND --profile PATH [options]
sber help COMMAND
sber COMMAND --help
sber --help
```

Options follow the command name.
String options accept `--name VALUE` or `--name=VALUE`.
A Boolean option without a value means `true`.
Use `--name=false` to select `false`.
The `--name false` form is invalid for a Boolean option.

Each `--card-id` on `card-info` adds one ID.
For other options, another occurrence replaces the previous value.
Two card IDs are invalid for `card-limits` and `card-rename`.
A path is a filename, not secret input.
Every operational command must have an explicit `--profile` path.

## Commands

| Command | Result or action | Bank access |
| --- | --- | --- |
| `login` | Make a new private profile through primary or PIN authentication. | Authentication. |
| `refresh-session` | Restore the selected profile through PIN authentication. | Authentication. |
| `status` | Read file metadata. | None. |
| `inspect-session` | Read profile metadata without secret values. | None. |
| `products` | Read account and card snapshots. | Product read. |
| `accounts` | Read account snapshots. | Product read. |
| `cards` | Read card snapshots. | Product read. |
| `portfolio` | Read products and their relationships from one response. | Product read. |
| `card-info` | Read card details. | Card read. |
| `card-limits` | Read limits for the first returned card. | Card read. |
| `operations` | Read history with pagination and coverage metadata. | History reads. |
| `operations-page` | Read one history page and its next offset. | History read. |
| `operation-details` | Read one operation. | Operation read. |
| `analytics` | Read totals for a selected period. | Analytics read. |
| `check-session` | Send one forced session check. | Session check. |
| `export-session` | Write a session copy to a new private file. | None. |
| `inspect-credentials` | Read credential metadata without secret values. | None. |
| `card-rename` | Show a plan; use `--execute` for one name change. | None for a plan; one mutation for execution. |
| `transfer-own` | Show a plan; use `--execute` for one transfer workflow. | None for a plan; a workflow for execution. |
| `mcp` | Start the local MCP server. | Explicit MCP data requests. |

## Common options

| Option | Default | Commands | Meaning |
| --- | --- | --- | --- |
| `--help`, `-h` | `false`. | All commands. | Show command help without profile access or bank requests. |
| `--profile PATH` | None; required. | All operational commands. | Select one private profile. |
| `--ca-bundle PATH` | Embedded CA with available system PEM trust. | Authentication and client commands. | Replace the trust bundle for the selected client. |
| `--timeout DURATION` | `30s`. | Client commands. | Limit each request to 1 through 120 seconds. |
| `--no-renew` | `false`. | Data reads and `check-session`. | Disable interactive session restoration. |
| `--force-update` | `false`. | `products`, `accounts`, `cards`, `portfolio`. | Get new product data. |

Offline `status` and `inspect-session` accept `--profile` and help options.
Authentication commands have a fixed request timeout of 60 seconds.
MCP, export, credential inspection, and mutations do not accept `--no-renew`.
They do not do interactive session restoration.

## Authentication options

| Option | Default | Commands | Meaning |
| --- | --- | --- | --- |
| `--remembered-profile PATH` | None. | `login`. | Use a remembered identity for PIN login into a new file. |
| `--browser-profile PATH` | None. | Authentication and interactive data reads. | Select the private Firefox profile. |
| `--playwright-driver PATH` | None. | Authentication and interactive data reads. | Select the installed matching driver. |
| `--firefox-executable PATH` | None. | Authentication and interactive data reads. | Select the installed matching Firefox executable. |

The three browser paths must all be absent or all be absolute paths.
They control public rendering before native authentication.
They do not provide browser authentication or configure NSS trust automatically.

## History options

| Option | Default | Commands | Limits or meaning |
| --- | --- | --- | --- |
| `--resource ID` | No resource filter. | `operations`, `operations-page`. | Select a resource with its type prefix. |
| `--from DATE` | Start of the default window. | `operations`, `operations-page`. | Inclusive start date or timestamp. |
| `--to DATE` | End of the default window. | `operations`, `operations-page`. | Inclusive end date or timestamp. |
| `--limit NUMBER` | `30`. | `operations`, `operations-page`. | 1 through 100 operations per page. |
| `--max-pages NUMBER` | `100`. | `operations`. | 1 through 10000 pages. |
| `--offset NUMBER` | `0`. | `operations-page`. | Zero or greater. |
| `--operation-id ID` | None; required. | `operation-details`. | 1 through 128 letters, digits, underscores, or hyphens. |

Date ranges must have correct syntax and order.
History resource IDs start with `account:`, `card:`, or `ct-account:`.
Their suffix contains 1 through 128 letters, digits, underscores, or hyphens.
The maximum page count limits requests; full history coverage can remain unknown.

## Card options

| Option | Default | Commands | Limits or meaning |
| --- | --- | --- | --- |
| `--card-id ID` | None; required. | `card-info`, `card-limits`, `card-rename`. | Positive numeric product ID. |
| `--name TEXT` | None; required. | `card-rename`. | New card name. |

A card ID contains 1 through 16 ASCII digits.
Its numeric value must not be more than `9007199254740991`.
`card-info` accepts 1 through 100 IDs through repeated `--card-id` options.
`card-limits` and `card-rename` must have exactly one ID.
The bank request uses JSON numbers for card IDs.

A card name contains 1 through 56 characters.
It can contain Latin letters, Cyrillic letters, digits, spaces, commas, periods, and hyphens.
A name with only spaces is invalid.

## Analytics options

| Option | Default | Meaning |
| --- | --- | --- |
| `--from DATE` | None; required. | Inclusive start date or timestamp. |
| `--to DATE` | None; required. | Inclusive end date or timestamp. |
| `--income-type TYPE` | `outcome`. | Select `income` or `outcome`. |
| `--between-own` | `true`. | Include transfers between your products. |
| `--open-banking` | `false`. | Include external bank data. |
| `--show-categories` | `true`. | Include category totals. |
| `--show-products` | `true`. | Include product totals. |

## Export option

| Option | Default | Command | Meaning |
| --- | --- | --- | --- |
| `--destination PATH` | None; required. | `export-session`. | Select a new absolute private path. |

An export destination must not exist.
The command uses private enrollment publication.
It does not write session values to standard output.

## Mutation options

| Option | Default | Commands | Meaning |
| --- | --- | --- | --- |
| `--execute` | `false`. | `card-rename`, `transfer-own`. | Let the CLI send the operation after terminal confirmation. |
| `--source ID` | None; required. | `transfer-own`. | Select the source product. |
| `--destination ID` | None; required. | `transfer-own`. | Select a different destination product. |
| `--amount DECIMAL` | None; required. | `transfer-own`. | Select the exact positive amount. |
| `--currency CODE` | `RUB`. | `transfer-own`. | Select three uppercase letters. |
| `--purpose TEXT` | Empty. | `transfer-own`. | Supply up to 210 characters without control characters. |

Transfer resource IDs start with `account:`, `card:`, or `transactionAccount:`.
The suffix contains 1 through 128 letters, digits, underscores, or hyphens.
The amount can contain up to 15 integer digits and two decimal places.
Exponents, signs, commas, and zero are invalid amount inputs.
The bank workflow must supply both selected resources in the requested currency.

The first `CONFIRM` lets the CLI start preparation.
The second `CONFIRM` lets the CLI send final transfer confirmation.
The implemented confirmation sequence does not include a payment SMS branch.
An unknown final result gives exit code `4` without a successful JSON result.

## Implemented API mapping

This table describes the implemented native SDK routes.
It is not a catalog of all bank APIs.

| SDK method | CLI commands | Route |
| --- | --- | --- |
| Primary and PIN authentication | `login`, `refresh-session`, interactive restoration. | Native authentication state machine. |
| `Products.Get`, `Accounts.List`, `Cards.List`, `Resources.Portfolio` | `products`, `accounts`, `cards`, `portfolio`. | `/main-screen/rest/v2/m1/web/section/meta` |
| `Cards.Info`, `Cards.Limits` | `card-info`, `card-limits`. | `/ufs-carddetail/rest/card/v1/cardInfo` |
| `Operations.Collect`, `Operations.Page` | `operations`, `operations-page`. | `/uoh-bh/v1/operations/list` |
| `Operations.Details` | `operation-details`. | `/uoh-bh/v1/operation/details` |
| `Analytics.Amounts` | `analytics`. | `/pfpv_alf_mb/v1.00/alf/amounts` |
| `Session.WarmUp` | `check-session`. | `/api/warmUpSession` |
| `Session.Export`, `Session.Credentials` | `export-session`, `inspect-credentials`. | Local session data. |
| `Cards.Rename` | `card-rename --execute`. | `/ufs-productdetail/rest/v1/changeProductName` |
| `Transfers.Start`, `Transfers.Prepare`, `Transfers.Confirm` | `transfer-own --execute`. | `/me2me/v1/workflow`, `/bh-confirmation/v3/workflow2` |

The CLI combines transfer methods because workflow ownership lasts for one process.
It does not expose arbitrary request paths or financial request replay.
Iterator helpers and entity methods use the same typed SDK routes.

## Technical terms

This term list supplies the subject terms for the CLI manual.
It uses the technical noun and verb categories in ASD-STE100 Issue 9.
Command names, option names, JSON fields, and diagnostic labels keep their exact application spelling.

### Technical nouns

Computer terms use the computer science category.
Bank products and operation terms use the service and product terminology category.
Numbers, units, and dates use the measurement and time category.

| Term | Meaning |
| --- | --- |
| account | A bank product that holds money. |
| analytics | Totals for bank operations in a selected period. |
| ALPN | TLS negotiation of the application protocol; this client supplies HTTP/1.1. |
| API | An interface for application requests. |
| authentication | The process that gives access to the bank account. |
| authorization | The bank decision to accept a request. |
| bank | The Sber service that owns the account and its protocol. |
| batch command | A command without interactive terminal input. |
| CA | A certificate authority that supplies a trust anchor. |
| CA bundle | A PEM file that contains trust anchors. |
| CAPTCHA | A bank challenge that makes human interaction necessary. |
| card | A bank payment product with an associated account. |
| card ID | The product identifier that the bank API accepts. |
| card number | The payment card number; it differs from the card ID. |
| CLI | The command line interface of the `sber` and `rental-check` executables. |
| Cobra | The Go library that reads CLI arguments and produces help text. |
| command | The selected CLI operation, such as `products`. |
| command argument | A value that the caller supplies on the command line. |
| cookie | A name, value, and scope in a browser session. |
| coverage metadata | Information about the known limits of returned history. |
| cryptographic proof | Data that shows possession of the expected authentication secret. |
| credential | Data that gives authentication or session access. |
| device identity | The remembered browser and device data in a profile. |
| directory | A location that contains files. |
| executable | The application file that the operating system starts. |
| enrollment | Initial authentication, optional PIN creation, and first private profile publication. |
| exit code | The numeric result that a command gives to its caller. |
| Firefox | The browser that does optional public initialization. |
| Go | The language and toolchain that build this repository. |
| history | The returned list of bank operations. |
| hidden input | Terminal input with character echo disabled. |
| hostname | The server name that TLS validates. |
| HTTP | The protocol for bank requests and responses. |
| ID | An identifier for a product, operation, or workflow. |
| JSON | The structured data format for CLI results. |
| local server | A test server on the same computer. |
| login | The account identifier or the authentication command, as specified by context. |
| MCP | The protocol for local SDK tools over standard input and output. |
| metadata | Information about data, without its secret values. |
| mode | The file or directory permission bits, such as `0600`. |
| NSS | The certificate store that the Firefox profile uses. |
| offset | The starting position for a history page. |
| operation | A bank history item or bank data change, as specified by context. |
| option | A named command parameter, such as `--profile`. |
| page cap | The maximum number of history pages for one command. |
| password | The secret used with the account login. |
| path | The file location supplied to a command. |
| PEM | The text encoding of certificates in a CA bundle. |
| PIN | The online banking code for a remembered identity. |
| Playwright driver | The installed runtime that controls Firefox. |
| portfolio | Accounts, cards, and relationships from one product response. |
| private file | A regular owner file with private permissions. |
| product | An account or card returned by the products API. |
| profile | The private file that contains session and device data. |
| publication | The atomic step that puts a validated profile at its destination. |
| protocol | The rules for requests, responses, and state transitions. |
| read-only mode | A policy that lets the CLI read data and disables bank data changes. |
| request | One application message to the bank. |
| response | The bank message for a request. |
| root CA | A trusted certificate authority at the top of a certificate chain. |
| SDK | The native Go library that the CLI uses. |
| session | The bank access state at a specified time. |
| shell completion | A shell function that suggests command names or options from partial input. |
| SMS code | A temporary bank code sent by SMS. |
| standard error | The process stream for diagnostics and confirmation plans. |
| standard input | The process stream for terminal input or the MCP protocol. |
| standard output | The process stream for JSON results or the MCP protocol. |
| synthetic data | Test data that has no owner account information. |
| terminal | The local device for hidden owner input. |
| timestamp | A date and time with an optional UTC offset. |
| TLS | The protocol that validates and encrypts a bank connection. |
| TLS setup | Connection establishment and certificate validation before an application request. |
| token | A secret session value in the bank cookie pair. |
| transfer | A bank operation that moves money between products. |
| trust anchor | A verified CA certificate accepted by the client. |
| UTC offset | The time difference from Coordinated Universal Time. |
| WebAuthn | A bank authentication challenge that makes an owner device necessary. |
| workflow | The ordered bank states for one transfer. |

### Technical verbs

These verbs refer to computer processes and applications.
For their use as nouns, define separate noun terms.

| Verb | Meaning |
| --- | --- |
| build | Compile the Go source into an executable. |
| close | Release the local client and its transport. |
| copy | Make a separate file or data value. |
| configure | Set the specified application parameters or certificate store. |
| disable | Prevent the specified application behavior. |
| download | Get a file from a remote source. |
| encrypt | Change secret input into ciphertext for the bank protocol. |
| enter | Supply a value at a terminal prompt. |
| install | Prepare the specified runtime for use. |
| log in | Make a bank session through authentication. |
| open | Read or start the specified file, terminal, or application. |
| restore | Make a new bank session from a remembered identity. |
| render | Execute the public page scripts and read the resulting browser document. |
| save | Write the validated session to its private file. |
| type | Supply characters through terminal input. |
| update | Install or produce a new application version. |
| validate | Determine whether data obeys the specified application contract. |

`read`, `send`, `select`, `start`, `stop`, `use`, and `write` use their approved dictionary meanings.
The full standard and dictionary remain the authoritative source for word use.

### Rental technical nouns


These terms supplement the shared term list for the offline rental preview.
Use them only for their defined subject meanings.

| Technical noun | Meaning |
| --- | --- |
| ASCII | A character encoding that includes the digits 0 through 9. |
| UTF-8 | A Unicode text encoding. |
| Unicode surrogate pair | Two Unicode code units that represent one character. |
| boolean | A value that is either `true` or `false`. |
| int64 | A signed integer type with 64 bits. |
| arithmetic overflow | A calculation that gives a value outside the permitted integer range. |
| context | A runtime state that can cancel a command. |
| HAR | HTTP Archive. A file that records HTTP requests and responses. |
| ledger | Records of tenants, rent periods, receipts, and evidence of complete history. |
| preview | A local evaluation of a supplied ledger. |
| minor unit | The smallest recorded currency unit. The `Minor` field holds an integer count of these units. |
| candidate decision | A ledger result that needs owner review before any further action. |
