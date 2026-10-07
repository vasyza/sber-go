# Business SDK port — worker coordination

Source reference: pinned licensed `sber_unofficial/client.py`, `resources.py`, `entities.py`, `_http.py`, plus audited tests/inventory. Python remains reference only.

Two disjoint NEW-file scopes are active:
- Native business client owns `client*.go` and its tests: transport/session ownership, discovery, warmup, read renewal, persistence/swap/close and one-shot mutation.
- Typed resources/entities own `resources*.go`, `resource_contract.go`, `entities*.go` and their tests: all source APIs including pagination/portfolio and mutation parity.

## Stable shared primitive contract

Resource-owned `BusinessRequester` structurally requires:

```go
PostRead(context.Context, string, map[string]any) (map[string]any, error)
Mutate(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)
MutationSequence(context.Context, func(func(context.Context, string, map[string]any, map[string]string, string, bool) (map[string]any, error)) error) error
ExportSession() (SessionBundle, error)
ExportCredentials() (SberCredentials, error)
WarmUp(context.Context, bool) error
```

The client implements these methods. The nested mutation sender is sequence-owned and must not escape its callback; resource code must not call the outer locking `Mutate` from within a sequence. Client may expose `Close() error` without extending the resource interface.

The primitive and typed resource/entity implementations are now exercised together in a sealed business baseline: parent verified all 35 client and 39 resource files, repeated owned race gates, and compiled the combined library (107 ordinary tests/612 subtests plus three fuzz suites/17 seeds; scoped race/vet/build pass). This is **not independent acceptance**. Concrete-client cached resource convenience/Portfolio binding remains pending strict-TDD integration and fresh review under `deleg_77571d33`; the existing bundle must be retained to share workflow guards across snapshots. Known shared generic financial-export type loss remains a blocker even though each entity's own explicit export is correct. Whole-root/auth/foundation and live gates remain separate.

## Safety/acceptance

- Distinct exact read/mutation/warmup allowlists from `_http.py`; raw IDs validated before source-compatible normalization.
- Business reads may renew once per rejected epoch; no financial POST replay or blind retry.
- Mutations remain present for full parity, behind explicit default-off resource and client policy. Neither policy narrows a bank-issued session's scope.
- History retains N+1/page cap/filter/window contracts; synthetic pagination does not prove the bank's real cap or full incoming-history coverage. Missing/malformed/uncertain input never authorizes reminders.
- Requester fixture scripts and fresh synthetic profile paths only; default constructors do not contact the bank. No live login, credentials, tenant sends, publication or installation.
- Opaque-API worker owns existing frontend/model/parser/auth callsite changes; these two workers must not edit those files. New resource code adapts to actual getter constructors, not fabricated symbols.
- Each slice needs real vertical RED/GREEN tests and frozen owned gates. Whole SDK acceptance waits for integration, fresh independent reviews, CI and separately authorized owner verification.
