# Client cycle-4 repair — private candidate only

## Architecture and boundary

This is coordinated repair round 4 of the existing six-round budget. Historical rounds and FAIL artifacts remain intact. The starting bytes are the 171-file immutable `sdk-baseline/manifest.json`, not the live repository. The initial actual race execution repeats four failing families and 19 failed subcases. The scoped architecture was explained before production edits.

The opaque client still owns one lock-bearing lifetime, one cached Resources bundle, one transfer issuer and all acquired closing owners. Copies, shortcuts and later Portfolio snapshots share those objects. No getter reconstructs Resources. This repair changes only client production and the three explicitly assigned transfer resource files. Errors, authentication, session/cookies, HTTP/browser/crypto, models, parsers and history resources remain byte-identical dependencies; no foundation acceptance is implied.

### Error trust boundary

An exported error interface can be embedded in an external package, promoting its private marker while overriding Error. Interface membership is therefore not redaction provenance. `clientSafeError` now copies exact recognized classifications into owned values. Arbitrary Message fields, remote text on injected errors, foreign outer objects and private causes are discarded. Transport codes are fixed allowlisted literals; HTTP status codes are bounded. Mutable numeric auth hints are detached copies, not aliasing pointers. Derived PIN classification is retained without injected URLs. Parse classifications do not retain an injected raw field path.

Cancellation and deadline identity are reconstructed from known context sentinels and copied error trees; foreign As/Is implementations cannot manufacture privileged classifications. Retryable client cleanup retains its owned Close action with a sanitized copied cause. Error tree traversal has a 64-node budget, including typed-nil and nil children; over-budget branches become a generic safe failure. This bounds foreign graphs and does not promise preservation beyond the budget. A typed-nil boundary and budget defect found while strengthening the tests have their own actual RED/GREEN artifacts.

This boundary concerns incoming transport/factory/callback/renewal errors. The client's own validated response decoder retains the existing explicit bounded rejection metadata API; read decoding preserves the canonical source success gate. Foundation error-layout/privacy redesign is not part of this repair.

### After-send outcomes

Each mutation still makes one attempted POST, with no automatic renewal, redirect/retry or financial replay. A missing, malformed, duplicate-key, unavailable or nonconforming response now retains MutationUncertain and its safe source classification. In mutation decoding, only a strict original-byte object with explicit boolean false or string "false" establishes a definite API rejection. Missing/null/numeric/unknown success flags do not prove rejection. Successful true flags and the read decoder's canonical behavior are unchanged. Pre-send policy, endpoint, page, identity and cancellation errors do not acquire uncertainty.

A transport-returned APIRejected is still an unknown after-send transport outcome, not a validated rejection response. Its uncertainty takes priority at the START/Prepare rejection branch. The cached START guard cannot reset through a later Portfolio. This integration hole was observed in a separate real RED before its minimal repair.

### Bound transfer propagation

Start, Prepare and Confirm retain their existing one-shot attempt guards and shared issuer. Their unknown-outcome returns join a resource uncertainty marker with a newly sanitized error copy, preserving cancellation/deadline and cleanup semantics without retaining foreign causes. The original ten failing five-stage/two-context cases and two complete five-send controls now pass. Original resource validation, exact amounts, definitive rejection and replay-guard assertions remain unchanged.

### Initial closing-owner handoff

A private per-construction `clientInitialOwners` ledger reserves explicit business injection as borrowed and tracks acquired auth helper owners. The default unready-profile source wiring passes the same ledger from the guarded helper factory into first business adoption. Previously acquired/closed owners and honest stable-identity aliases fail before CookieJar access, publication, use or discard/Close. A fresh business owner and initial acquired owner history are transferred into the client's existing owned list. Borrowed reservations are never transferred as cleanup owners. Fresh failed candidates retain retryable cleanup.

The optional stable comparable ClientTransportOwner identity contract and its limitations are unchanged: hidden physical aliases or dishonest identity methods are not proven safe, and no global registry is introduced.

**Strict auth qualification:** the successful public default-PIN branch is source-inferred only and was NEVER executed. Evidence is actual client-helper normalization/guard/Close/adoption composition plus static source-wiring proof. No NewPINAuth/Login, SRP/PIN/credential POST, bank bootstrap or foundation repair was executed for this handoff. Existing tests requiring foundation auth execution, and the test constructing a real default HTTPTransport, are explicitly excluded from this worker's runtime selection and remain unchanged for parent accounting.

The exact old independent helper witness is preserved in `original/probes`. That witness explicitly called the old disconnected composition; no honest client-only fix can make that obsolete hardcoded wiring test a new ledger without a registry or changing public options. The cycle-4 copied witness changes only its helper construction/adoption wiring to match the actual repaired source. All original assertions remain; the adaptation diff and AST body comparison are retained. The historical exact-byte RED is not relabeled or erased, and no successful auth execution is claimed.

## TDD and evidence

Actual ordered current RED → minimal GREEN → regression artifacts cover the external embedding boundary, owned concrete error copies, after-send responses, bound cancellation and initial helper handoff. Supplemental executed RED/GREEN covers typed nil errors, nested classified rejection START replay and the error-node budget. Passing test-only strengthening is not described as a feature RED.

Commands use the explicit Go1.27.1 executable with GOTOOLCHAIN=local, GOPROXY=off, GOSUMDB=sum.golang.org, GOWORK=off, GOMAXPROCS=2 and readonly module flags. Complete raw JSONL, commands/cwd/environment/input hashes/exit codes, diffs, API/source proofs, original fixtures, race/stress, vet/build, native compiled execution and bounded fuzz evidence are kept under `repair-cycle4/client/`. Test, subtest, fuzz suite and fuzz seed events are counted separately from complete streams.

All existing baseline client tests and helpers remain unchanged. The 29-callable canonical source mapping and the archived original test contracts remain in the preservation proof, with actual runtime exclusions and source-only auth qualification made explicit rather than claiming all foundations executed. The 53 sealed expiry rows and six disclosed native differences are preserved as unchanged inherited evidence; no new source differential execution is claimed.

This is a sealed worker candidate, not independent acceptance, whole-SDK approval, live-bank proof or goal closure. No live repository application, staging/commit/publication, CI/install, owner secret/profile/account/HAR read, financial live POST/replay, tenant activity, security/provider change, refused foundation job reroute or prohibited goal-wait action occurred.
