# CLI writing rules

Use ASD-STE100 Simplified Technical English for current CLI guides, help text, and messages.
These rules apply to `sber` and `rental-check`.
Historical review records keep their original text and source hashes.

## Sentences and instructions

- Use approved ASD-STE100 words with their approved meanings.
- Use the active voice.
- Give one instruction in each procedural sentence.
- Keep procedural sentences to a maximum of 20 words.
- Keep descriptive sentences to a maximum of 25 words.
- Use the imperative form for instructions: "Use", "Check", "Read", or "Show".
- Use complete sentences for descriptions and error messages.
- Use short noun labels for headings.
- Use the same technical noun for the same item.
- Define technical nouns in the shared glossary below.
- Keep command names, option names, JSON fields, and code identifiers unchanged.

## Technical nouns

These technical nouns have these meanings in the CLI guides:

| Technical noun | Meaning |
| --- | --- |
| CLI | Command line interface. A program that reads command arguments. |
| Cobra | The Go library that reads CLI command arguments and produces help text. |
| command argument | A value that the caller supplies on the command line. |
| option | A named command setting, such as `--profile` or `--help`. |
| shell completion | A shell function that suggests command names or options from partial input. |
| profile | A local file that contains saved bank session data. |
| session | A saved state for access to the bank service. |
| cookie | A data item that a server uses to identify a session or store a setting. |
| deviceprint | Device information in a saved bank profile. |
| token | A value that a service uses for access checks. |
| metadata | Profile properties that the CLI can show with secret values removed. |
| context | A runtime state that can cancel a command. |
| JSON | JavaScript Object Notation. The CLI input and output data format. |
| HAR | HTTP Archive. A file that records HTTP requests and responses. |
| MCP | Model Context Protocol. A protocol for tools and model applications. |
| ASCII | A character encoding that includes the digits 0 through 9. |
| UTF-8 | A Unicode text encoding. |
| Unicode surrogate pair | Two Unicode code units that represent one character. |
| boolean | A value that is either `true` or `false`. |
| int64 | A signed integer type with 64 bits. |
| arithmetic overflow | A calculation that gives a value outside the permitted integer range. |
| timestamp | A value that specifies a date, time, and time zone offset. |
| standard input | The input stream that the caller gives to the command. |
| standard output | The output stream for command results and help text. |
| standard error | The output stream for error messages. |
| ledger | Records of tenants, rent periods, receipts, and evidence of complete history. |
| preview | A local evaluation of a supplied ledger. |
| minor unit | The smallest recorded currency unit. The `Minor` field holds an integer count of these units. |
| candidate decision | A ledger result that needs owner review before any further action. |

Introduce other technical nouns when a new command needs them.
Use technical nouns only for their defined subject meanings.

## Messages

State the condition or the action that failed.
Give a permitted next step when it helps the user.
Use fixed text for argument errors.
Do not include argument values, input data, profile paths, or private error details.

Example:

```text
The command arguments are not valid.
Use sber --help for command help.
```
