# CLI writing rules

Use [ASD-STE100 Issue 9](CLI-STYLE.md) for CLI guides, help text, and messages.
These rules apply to `sber` and `rental-check`.
The [shared term list](CLI-TERMS.md) defines computer and bank terms.
Historical review records keep their original text and source hashes.

## Messages and instructions

Use approved words with their approved meanings and parts of speech.
Use the active voice.
Give one action in each procedure sentence.
Keep procedure sentences to a maximum of 20 words.
Keep descriptive sentences to a maximum of 25 words.

Use complete sentences for descriptions and error messages.
Use short noun labels for headings and prompts.
Use the same technical noun for the same item.
Keep command names, option names, JSON fields, and code identifiers unchanged.

State the condition or the action that failed.
Give a permitted next step when it helps the user.
Use fixed text for argument errors.
Do not include argument values, input data, profile paths, or private error details.

Example:

```text
The command arguments are not valid.
Use sber --help for command help.
```

## Rental technical nouns

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

Introduce other technical nouns when a new command needs them.
The [writing policy](CLI-STYLE.md) gives the verification limits for these rules.
