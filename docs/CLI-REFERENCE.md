# CLI command reference

This reference uses the terms in [CLI-TERMS.md](CLI-TERMS.md).
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
| `--profile PATH` | None; required. | All operational commands. | Select one private profile. |
| `--ca-bundle PATH` | Embedded CA with available system PEM trust. | Authentication and client commands. | Replace the trust bundle for the selected client. |
| `--timeout DURATION` | `30s`. | Client commands. | Limit each request to 1 through 120 seconds. |
| `--no-renew` | `false`. | Data reads and `check-session`. | Disable interactive session restoration. |
| `--force-update` | `false`. | `products`, `accounts`, `cards`, `portfolio`. | Get new product data. |

Offline `status` and `inspect-session` accept only `--profile`.
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
