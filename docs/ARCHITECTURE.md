# Package structure

| Package | Responsibility |
| --- | --- |
| `sber` (root) | Public facade, aliases and compatible function forwards |
| `internal/errs` | Redacted errors and safe context classification |
| `internal/session` | Session models, cookies, headers, config, identity and platform storage |
| `internal/transport` | TLS-verified HTTP, bounded decoding and bootstrap contracts |
| `internal/auth` | SRP/RSA primary login, PIN/OTP/CAPTCHA and cleanup |
| `internal/bank` | Client lifecycle/renewal, `BusinessRequester`, domain models, parsers, resources and transfer workflows |
| `browser` | Explicit Firefox public bootstrap and frozen cookie adoption |
| `internal/ownerinput`, `internal/enrollment` | Hidden terminal input and private lock/no-replace publication |
| `internal/cli`, `cmd/sber` | Validation, application orchestration and signal-aware command |
| `internal/mcptools`, `internal/mcpwire` | Tool schemas and bounded/cancelable protocol engine |
| `mcp` | Handlers connecting one selected client to the protocol |
| `rental`, `internal/rentalcli`, `cmd/rental-check` | Independent reconciliation and offline JSON command |
| `internal/strictjson`, `internal/srp`, `internal/rsaoaep` | Protocol/crypto primitives |

Dependencies flow root/app → bank → auth/transport/session → errors/primitives. Session, transport and auth never import the facade. Browser bootstrap is injected through transport interfaces, avoiding a bank-to-browser cycle. Rental does not import the bank SDK. MCP wire/catalog do not authenticate or read profiles.

The bank package is one cohesive resource/lifecycle boundary. Files separate client lifecycle (`client_*`), resource services (`resources_*`), bound entities (`entities_*`), money (`money.go`), models (`models*.go`), deliberate export (`export.go`), dates/time filters and response parsers. Financial export recognizes its own native models without trusting arbitrary caller structs.

`BusinessRequester` supports synthetic or caller-owned implementations. Resources do not own transport cleanup. The concrete client owns transports, serializes operations/renewal and invalidates workflows on close. Mutation sequences hold one lifetime gate and stop after failure; uncertain outcomes are not replayed.

The CLI has one command registry and option parser (`commands.go`, `run.go`). Offline inspection, new-profile enrollment, existing-profile refresh, interactive read renewal, PIN enrollment, and mutation confirmation each have separate application modules. The application validates arguments before opening a client. SDK resources own request contracts and response models; the CLI owns hidden input, output streams, exit codes, and safe diagnostics. Non-streaming JSON is published only after command success and client cleanup. Documentation tests check the registry, parsed examples, and the selected STE writing limits.

Transport TLS establishment (`tls_establishment.go`) advertises the supported HTTP/1.1 protocol and can recover a peer closure before HTTP transmission. Its three-attempt bound shares the original request deadline and cancellation. Certificate/protocol failures stop immediately. Established business connections retain one HTTP request per connection; transmitted financial/authentication POSTs retain their no-replay policy.

Tests live beside implementations. Root external tests preserve public-consumer coverage. Fixtures are repository-relative. `internal/testutil` is imported only by tests. Constructors perform no bank business request; authentication and selected-profile opening are explicit. History coverage and pagination termination remain separate facts.
