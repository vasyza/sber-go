# SDK4 combined candidate — verified offline, independent acceptance pending

All three `deleg_1a39a121` implementation workers returned. No candidate SDK source has been applied to the live repository and no repair round 5 has started. Budget remains 4 consumed of 6 total; original FAILs remain.

## Intake and exact bytes

Parent `verify_and_combine.py prepare` verified **3,207 distinct source/artifact targets** (deduplicated, not the sum of overlapping worker manifests). The unchanged 171-file starting manifest remains SHA256 `0ce870e61c0206620e5a28486b15f3e5fbf0262dbf262757215cecc3e377d446`.

The disjoint private overlays contain 48 changed/added owned files: financial 11, dates 17, client 20. The combined private snapshot has **206 actual source/fixture/context files**. Production changes come only from each worker's assigned scope. Auth/session/cookies/HTTP/browser/crypto and `errors.go` were not modified. Financial sidecar `../independent-oracle.json` was copied with its exact hash; manually compiled tests were executed from the package fixture directory.

Combined seal: `research/go-migration/repair-cycle4/parent-combined/review-manifest.json`, SHA256 `e9df701bf7e1770b3bb0f18a16351edb23cb8da27098d7236a30c2c57bfc1d72`.

## Retained failure and test-contract maintenance

The date worker ended with one real bounded-fuzz FAIL, not all-green. The unchanged inherited assertion incorrectly required DATETIME rejection to imply DATE-first constructor rejection. Parent independently executed the original pure helpers and original Page body using Python **3.12.3**, with the transport replaced by a synthetic capture (no SDK import or auth/network). The original application accepts `0001010100` as DATE `0001-01-01` and emits its full-day bounds, although DATETIME rejects the same string. Another exact control, `2024W09200`, means DATE `2024-02-27` but DATETIME `2024-02-26T00:00:00`; inclusion of the latter in the former's separately inferred window is not a source guarantee.

Parent reproduced both stale assertions as two deterministic failing seeds on the combined candidate. Only the private parent active copy of `parsers_review_cycle3_fuzz_test.go` was amended: constructor rejection requires both independent grammars to reject, and accepted DATE-first civil bounds are asserted directly instead of assuming the independently parsed DATETIME is contained. All original worker/baseline tests, failures and seals are unchanged. Production was not narrowed or changed for this step. This is **test-only contract maintenance, not a production RED→GREEN fix**; fresh date review must independently audit it. The initial parent oracle control-selection setup failure and file-local vet warnings (missing package companion declarations) are retained separately, not called production regressions.

## Actual combined execution

`combined_gates.py` invoked explicit Go1.27.1 with readonly modules, `GOTOOLCHAIN=local`, `GOPROXY=off`, `GOSUMDB=sum.golang.org`, `GOWORK=off`, `GOMAXPROCS=2`.

- Scoped root-package race: **350 ordinary tests / 49,595 subtests / 30 fuzz seed suites / 268 seeds**, all executed events pass, no failure or skip events.
- An actual compiled race artifact was executed with the identical selected test pattern and package fixture cwd; the same event counts pass and the artifact hash is unchanged.
- Scoped `go vet`, `go build`, module verification: exit 0.
- **Nine real bounded fuzz targets / 90,000 reported executions**, all exit 0, including the corrected inherited DATE/DATETIME contract target. Executions are not unique contracts.
- Added-line static scan over actual assigned production diffs: zero matches. This is not an independent semantic/security verdict.
- Final parent readback verified **253 acyclic parent artifact targets** and reverified the original 3,207 worker/reference targets after execution.

Seven inherited public default-PIN/default-HTTP tests are **explicitly excluded** from the combined runtime pattern, with source and assertions retained byte-for-byte. Initial successful public PIN ownership handoff remains **client-helper composition plus source wiring only**, never public-PIN execution or live auth proof. Historical financial-worker full-snapshot gate is retained as its own reported/raw scope; it is not foundation/auth acceptance or a substitute for the explicit parent exclusions.

The combined root-package pattern is not complete `go test -race ./...` for the live repository, full parity closure, CI, installation, or bank history verification. Accepted strictjson/rental/catalog production remains outside these changes. History stays UNKNOWN; reminder/delivery/real-contract mapping and owner gate remain closed. The refused foundation repair, forbidden goal-wait outcome, and separately blocked MCP step were not rerouted.

## Evidence and next gate

Under `research/go-migration/repair-cycle4/parent-combined/`: `worker-readback.json`, `initial-combined-manifest.json`, `source-witness.json`, `parent-oracle-setup-qualification.json`, `02-legacy-implication-confirmed-receipt.json`, `test-only-contract.diff`, `test-only-contract-decision.json`, actual race/non-test/native/fuzz receipts and full logs, `test-selection.json`, `native-artifact.json`, `production-static-scan.json`, `review-manifest.json`, `parent-verification.json`, `artifact-manifest.json`, `final-readback.json`.

Fresh independent financial, date/application and client reviews must inspect the **same combined 206-file seal**, not isolated passing worker copies. They may add synthetic probes in their own copies, but must not fix production, modify these seals or execute the seven excluded branches. No live repository application, release, owner auth or goal closure follows from implementation gates alone.
