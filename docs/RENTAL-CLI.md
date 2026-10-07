# rental-check CLI

The `rental-check` command reads a rental ledger from standard input.
It writes an offline preview as JSON.
It uses the same Cobra version as the [sber CLI](CLI.md).

The command does not make bank requests, get the current time, or send reminders.
It does not search for files or profiles.
Give all dates, tenant identifiers, prices, currency codes, and tenant assignments in the ledger.

## Build and show help

1. Build the command with Go 1.27.1.

   ```sh
   go build -o bin/rental-check ./cmd/rental-check
   ```

2. Show command help.

   ```sh
   ./bin/rental-check --help
   ```

3. Give the command a ledger through standard input.

   ```sh
   ./bin/rental-check < explicit-ledger.json
   ```

You can use `-h` instead of `--help`.
You can also use `rental-check help`.
A help request does not read the ledger.
The command does not accept ledger file arguments or other options.
It rejects shell completion commands.

Do not put a login, password, PIN, cookie, HAR, or bank profile in the ledger.

## Input format

Use one complete JSON document.
The maximum input size is 1 MiB (1,048,576 bytes).
The command rejects incomplete input, extra JSON documents, and input above this limit.

Use these field names with the exact letter case:

- `AsOf`
- `Tenants`
- `Periods`
- `Receipts`
- `Evidence`

Use the exact field names in nested objects.
For example, `Confirmed` and `confirmed` are different names.
The command rejects unknown fields, names with the wrong letter case, and duplicate keys after JSON escape decoding.
It checks UTF-8 and Unicode surrogate pairs before it reads field values.

Use JSON integers for all `Minor` values.
The values must fit in an `int64`.
The command does not round fractional values or values outside that range.

See [the rental data types](../rental/README.md) and [rental/evaluate.go](../rental/evaluate.go) for all fields and ledger rules.

## Timestamps

Use this timestamp format:

```text
YYYY-MM-DDTHH:MM:SS[.fraction](Z|+HH:MM|-HH:MM)
```

The brackets show an optional fraction.
The parentheses show the permitted time zone forms.
Do not put the brackets or parentheses in a timestamp.

Use four digits for the year.
Use two digits for each month, day, hour, minute, and second.
Use uppercase `T` and `Z`.
Use a decimal point before a fraction with 1 through 9 ASCII digits.
A time zone offset needs two hour digits (00 through 23) and two minute digits (00 through 59).

The command checks the calendar date and clock values.
It rejects comma fractions, leap seconds, malformed offsets, and fractions with more than 9 digits.
It rejects excess fractional digits even when those digits are zero.
It does not round or remove excess digits.

These rules apply before native timestamp decoding to:

- `AsOf`
- `Tenant.LedgerStart`
- `Period.Start`, `Period.End`, and `Period.DueAt`
- `Receipt.ReceivedAt`
- `Evidence.CoverageStart`, `Evidence.CoverageThrough`, and `Evidence.ObservedAt`

Different valid timestamp forms can specify the same instant.
A `null` timestamp is not valid.
A missing or zero required contract time causes a ledger error.
A missing or zero evidence time does not prove complete history.

## Evidence of complete history

Each supplied `Evidence` object must contain these three boolean fields:

- `HasGaps`
- `Truncated`
- `PageUncertain`

A missing field, a `null` value, or a value of the wrong type causes an input format error.
A `true` value in any of these fields prevents proof of complete history.

You can omit `Evidence`, set it to `null`, or give an empty list.
Missing `Complete` and `OwnerReconciled` fields keep their value of `false`.
Missing evidence cannot prove that rent is due.
Confirmed receipts can still prove `PAID` when they cover the required amount.

The owner supplies evidence of complete history.
The CLI does not check the bank or confirm that evidence.
Without proof of complete history, a rent shortfall gives `UNKNOWN`.
Compare each candidate decision with the contracts, tenant assignments, and complete bank and cash records.

## Output and exit codes

The output contains `periods`, `allocations`, `credits`, `totals`, `owner_review`, `candidate_decisions`, and `as_of`.
It always contains `reminders_enabled: false` and `bank_authorization_checked: false`.
A candidate decision does not give permission to send a message.

The CLI writes JSON and help text to standard output.
It writes error messages to standard error.
Error messages do not contain raw input, argument values, or private error details.
Invalid input produces no decision output.
An output write failure can leave partial output.
A short write causes an output error.

| Exit code | Meaning |
| --- | --- |
| 0 | The offline preview or help request is complete. A preview can contain `UNKNOWN`. |
| 2 | The arguments, context, input, or output are not valid. |
| 3 | The input read, size check, JSON check, or input format check failed. |
| 4 | The ledger contains invalid or inconsistent data, or an arithmetic overflow occurred. |
| 5 | The preview preparation or output write failed. This includes a help output failure. |

For an argument error, the CLI writes:

```text
The command arguments are not valid.
Use rental-check --help for command help.
```

## Development checks

Run these checks with synthetic data:

```sh
go test -race ./internal/rentalcli ./internal/command ./internal/strictjson ./tests/rental
go vet ./internal/rentalcli ./internal/command ./cmd/rental-check
go build -o bin/rental-check ./cmd/rental-check
```

Follow the [writing policy](DEVELOPMENT.md#cli-writing-policy) when you change help text or messages.
The [verification record](STATUS.md) gives current limits and links to earlier review records.
