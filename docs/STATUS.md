# Verification — 2026-10-07

Scope: local modular SDK, CLI, MCP, synthetic/localhost contracts, and an explicitly authorized bounded real authentication/read E2E on macOS. Starting snapshot: `104c8d86ca63f9b5d0c6e6b8dafd3d53d635eb28`; development branch: `refactor/modular-sdk`.

| Check | Result |
| --- | --- |
| macOS ARM64, Go 1.27.1: `make check` | PASS: formatting, full vet/race suite, all packages, both binaries, module verification |
| Native Linux ARM64 in official `golang:1.27.1`: `go vet ./...`, `go test -race ./...`, `go build ./...`, CLI build/help, `go mod verify` | PASS |
| Native macOS `sber --help` and absent-profile `status` | PASS; offline status reports no profile and no authorization check |
| Original client diagnostic regression, including mutation-sequence route | PASS; one synthetic request per route, explicit rejection metadata remains accessible |
| Linux/macOS real enrollment + SDK serializer + profile reopen + refusal to replace | PASS; synthetic auth/hidden-input dependencies, actual filesystem publication |
| Linux/macOS native PTY hidden input, restoration, cancellation, concurrent prompt rejection and read/restore failures | PASS; synthetic secrets, real terminal descriptors |
| Linux/macOS enrollment locks, late destination creation, unsafe candidates, parent/lock swaps and cleanup | PASS; real filesystem and helper-process boundaries |
| Separately enabled `integration/TestLiveReadOnly` | PASS on macOS with the explicitly selected private session: warm-up, typed products, one history page (limit five, seven-day window); compiled and SKIPPED without `-sber-live` on both platforms |
| Native real remembered-device PIN authentication | PASS on macOS: verified TLS, PIN SRP proof, seamless navigation, frontend readiness and private profile publication; repeated successfully through the CLI production PIN factory |
| Ordinary public Firefox bootstrap | PASS: strictly validated PIN configuration, observed browser identity and cookies; native auth retained them |
| Synthetic seamless/HAR import regressions | PASS: effective main URL, SameSite policy casing, Chromium session-cookie expiry sentinel, explicit deletion priority and unsafe/ambiguous URL rejection |
| Auth connection reuse and request time budget | PASS on both platforms: verified local TLS connection reuse, body/empty POST no-replay checks, configured handshake budget |
| Embedded bank root CA and explicit replacement trust | PASS on both platforms: verified root fingerprint/self-signature, default authentication/business trust, operation without system CA files, preserved system trust and invalid explicit bundle rejection; original hostname/expiry/environment negative controls pass |
| Real bank TLS using embedded CA without an external certificate path | PASS on macOS: native public login-document GET returned HTTP 200 with ordinary chain and hostname verification |
| MCP discovery, protocol versions, strict schemas and SDK application handlers | PASS |
| Public root consumer tests and executable Go examples | PASS |

A Go AST declaration inventory also confirmed that all 196 original root public names and all 621 distinct original test/fuzz/example names remain present. This is a preservation check, separate from semantic test results.

Additional bounded fuzz runs used two workers and a five-second fuzz budget each:

| Target | Executions | Result |
| --- | ---: | --- |
| `FuzzClientCycle4ErrorBoundary` | 22,106 | PASS |
| `FuzzExactIntegerAgainstRationalOracle` | 281,718 | PASS |
| `FuzzResourceTransferAmountExact` | 61,611 | PASS |

The new Linux enrollment integration first failed because ordinary no-follow session saving cannot open a procfs directory capability. The repair duplicates and verifies the trusted open directory descriptor and creates a fixed new candidate with exclusive creation. Tests preserve existing regular/symlink targets. Ordinary profile saves keep their no-follow policy.

Native macOS enrollment and PTY tests first failed because the CLI rejected non-Linux platforms. The repaired CLI shares its input/enrollment state machines and supplies small native terminal/publication implementations for each platform. macOS uses exclusive rename in a private staging namespace; Linux retains pinned procfs publication. macOS test paths resolve only generated `/var` fixtures to their physical paths, while real profile paths remain subject to literal no-symlink validation.

The full package commands ran without bank credentials or test-name exclusions. Common terminal and enrollment protections now run natively on both platforms; Linux-only inode/procfs and seccomp protections still run in the native Linux container. Conditional helper-process/privileged-fixture skips inside the original suite retain their original purpose.

The real E2E passed on two freshly authenticated sessions, including the one produced through the CLI's production remembered-PIN factory. Only success stages and counts were printed. Real secret input for that factory was supplied by an owner-approved local adapter outside tracked source; production CLI secret input remains hidden terminal only, verified separately with native PTYs. Credentials, cookies, deviceprints, private HAR/response bodies and session profiles were not put in repository fixtures or this report. No financial mutations, whole-history scan, automatic credential retries or challenge bypass were performed.

The later embedded-CA follow-up passed the complete macOS/Linux checks and a native public HTTPS probe without `CABundle` or an external bank certificate file. The authenticated E2E rerun without `-sber-ca-bundle` stopped at session validation; a bounded diagnostic classified the selected saved session as `AuthenticationExpired`. It did not read products/history or perform another PIN/OTP login. The earlier authenticated PASS results above precede this follow-up; a fresh owner login is required to repeat that E2E.

The owner also signed in through Yandex. Its locally exported HAR exposed importer defects, now covered by synthetic tests. Its authenticated reuse probe returned HTTP 403 and did not publish a profile. The successful SDK flow used an independently rendered public browser snapshot and native remembered PIN login. Cold primary login without observed browser initialization previously returned HTTP 500 at final navigation; it is not claimed as a successful real E2E. CLI explicit browser initialization is wired and tested with synthetic dependencies; the ordinary public browser provider was verified separately against the real login document.

GitHub Actions configuration is present; no remote CI run is claimed. Historical source manifests, review failures and parity reports are preserved. A new independent production review, exact bank UI reconciliation, live Linux account access and real MCP client calls were not performed. Full history coverage remains UNKNOWN. These checks do not constitute production acceptance or approval of the entire upstream parity inventory.

For the owner-operated live check, follow [AUTH.md](AUTH.md). Supported commands and platform boundaries are in [CLI.md](CLI.md); MCP configuration is in [MCP.md](MCP.md).

## CLI completion follow-up — 2026-10-07

The current CLI has 20 commands with one argument/help registry and separate modules for offline inspection, primary enrollment, remembered-PIN restoration, interactive read renewal, typed reads, and guarded mutation plans. The operator documentation targets ASD-STE100 Issue 9, with a command/API reference, technical terms, writing policy, executable examples, and a separate [CLI verification record](CLI-VERIFICATION.md). Automated checks cover registry agreement, example parsing, sentence/paragraph limits, and contractions; no full dictionary checker or independent language certification is claimed.

The owner-operated primary login succeeded through native password proof, SMS confirmation, five-digit online-PIN enrollment, session validation and private no-replace publication. Explicit existing-profile restoration and one interactive expired-read restoration also passed on macOS. The final native CLI E2E passed all 15 read/local-profile commands on the newly created primary profile; the SDK authorization/products/history E2E passed on that same profile. Real card details and limits required numeric JSON IDs instead of the historical string payload and now pass. All real checks used the embedded CA, verified TLS and ordinary public browser initialization. Secrets, identifiers and financial response bodies remain outside repository evidence.

Intermittent checks stopped during TLS establishment before HTTP transmission; a public TLS-only probe reproduced peer connection resets. The transport now advertises HTTP/1.1 through ALPN and allows at most three connection attempts after a transient peer closure before sending HTTP. All attempts share the request deadline and cancellation; certificate/protocol errors stop immediately. Synthetic TLS tests verify exact POST counts, bounded failures, trust rejection, request cancellation and close. Transmitted HTTP requests are not automatically replayed. The final CLI and SDK live suites passed after this change.

The final local macOS and Linux checks include the new CLI/authentication/transport changes. Real card renaming and transfers remain untested; default CLI plans do no bank I/O and execution requires explicit local confirmation. Complete history coverage remains UNKNOWN. No remote CI, new independent production review, live Linux bank account access, or real MCP client is claimed. The earlier verification and historical review records above retain their original scope.

## MCP Go SDK migration follow-up — 2026-10-08

The transport now uses the official MCP Go SDK v1.8.0 for protocol 2026-07-28
and legacy 2025-11-25. Integration preserves all six application handlers,
including local session close with `readOnlyHint: false`; financial handlers
remain excluded. The official SDK client and wire regressions use synthetic
handlers, in-memory pipes, and an injected synthetic bank client. See
[the adapter profile](MCP-GO-SDK.md) for protocol and application limits.

Linux `make check` passed after merge integration: formatting, full vet/race
suite, all packages, both command binaries, and module verification. The
first full race run encountered a terminal-input test's transcript poll
error; ten focused repetitions and the subsequent full check passed.
Independent MCP review covers the adapter and application integration only.
No bank calls, owner authentication, live MCP verification, or whole-SDK
production acceptance were performed in this follow-up.
