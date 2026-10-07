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

The owner also signed in through Yandex. Its locally exported HAR exposed importer defects, now covered by synthetic tests. Its authenticated reuse probe returned HTTP 403 and did not publish a profile. The successful SDK flow used an independently rendered public browser snapshot and native remembered PIN login. Cold primary login without observed browser initialization previously returned HTTP 500 at final navigation; it is not claimed as a successful real E2E. CLI explicit browser initialization is wired and tested with synthetic dependencies; the ordinary public browser provider was verified separately against the real login document.

GitHub Actions configuration is present; no remote CI run is claimed. Historical source manifests, review failures and parity reports are preserved. A new independent production review, exact bank UI reconciliation, live Linux account access and real MCP client calls were not performed. Full history coverage remains UNKNOWN. These checks do not constitute production acceptance or approval of the entire upstream parity inventory.

For the owner-operated live check, follow [AUTH.md](AUTH.md). Supported commands and platform boundaries are in [CLI.md](CLI.md); MCP configuration is in [MCP.md](MCP.md).
