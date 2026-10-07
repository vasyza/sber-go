# CLI technical terms

This term list supplies the subject terms for the CLI manual.
It uses the technical noun and verb categories in ASD-STE100 Issue 9.
Command names, option names, JSON fields, and diagnostic labels keep their exact application spelling.

## Technical nouns

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
| CLI | The command line interface of the `sber` executable. |
| command | The selected CLI operation, such as `products`. |
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

## Technical verbs

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
