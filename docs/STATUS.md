# Verification — 2026-10-07

Scope: local modular SDK, CLI, MCP and their synthetic/localhost contracts. Starting snapshot: `104c8d86ca63f9b5d0c6e6b8dafd3d53d635eb28`; development branch: `refactor/modular-sdk`.

| Check | Result |
| --- | --- |
| macOS ARM64, Go 1.27.1: `make check` | PASS: formatting, full vet/race suite, all packages, both binaries, module verification |
| Native Linux ARM64 in official `golang:1.27.1`: `go vet ./...`, `go test -race ./...`, `go build ./...`, CLI build/help, `go mod verify` | PASS |
| Native macOS `sber --help` and absent-profile `status` | PASS; offline status reports no profile and no authorization check |
| Original client diagnostic regression, including mutation-sequence route | PASS; one synthetic request per route, explicit rejection metadata remains accessible |
| Linux real enrollment + SDK serializer + profile reopen + refusal to replace | PASS; synthetic auth/hidden-input dependencies, actual filesystem publication |
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

The full package commands ran without bank credentials or test-name exclusions. Linux-only inode/terminal protections ran in the native Linux container; macOS tests exercise supported storage and fail-closed unsupported enrollment. Conditional helper-process/privileged-fixture skips inside the original suite retain their original purpose.

GitHub Actions configuration is present; no remote CI run is claimed. Historical source manifests, review failures and parity reports are preserved. A new independent production review, actual browser-to-bank continuity, real login, authenticated reads and bank UI reconciliation were not performed. This document records local implementation checks, not production acceptance or approval of the entire upstream parity inventory.

For the owner-operated live check, follow [AUTH.md](AUTH.md). Supported commands and platform boundaries are in [CLI.md](CLI.md); MCP configuration is in [MCP.md](MCP.md).
