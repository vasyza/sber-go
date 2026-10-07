# Offline native rental-check

`bin/rental-check` — отдельный native Go preview уже реализованного pure rental engine. Это **не банковский login CLI, не интеграция с MCP/cron и не разрешение отправлять сообщения**. Existing Python launcher и owner-установка не изменяются.

```sh
go build -o bin/rental-check ./cmd/rental-check
./bin/rental-check < explicit-ledger.json
```

STDIN принимает только **некреденциальный explicit rental ledger**. PIN/password/cookies/HAR/банковские profiles этому формату не принадлежат; через чат их не передавайте. Автоматического поиска файлов/профилей, сетевого обращения и получения текущего времени нет.

## Строгий формат

- Одна полная JSON-документная структура, максимум 1 MiB (1048576 bytes), без truncation/trailing JSON.
- Canonical names строго совпадают с exported Go полями `rental.Input`: `AsOf`, `Tenants`, `Periods`, `Receipts`, `Evidence`. Поля вложенных объектов также exact-case: `ID`, `TenantID`, `Minor`, `Currency`, `Confirmed`, `CoverageStart`, `OwnerReconciled` и т. д.; полный typed shape описан в `rental/README.md`/`rental/evaluate.go`.
- Неизвестные поля, регистровые aliases и decoded duplicate keys отвергаются до struct decode. Это важно: обычный Go JSON decoder трактует `Confirmed` и `confirmed` как одно поле, хотя JSON names различны.
- UTF-8/surrogate integrity проверяется до декодирования. Денежные Minor — int64 JSON integers; fractional/overflow values не округляются.
- Все supplied timestamps проверяются **до `time.Time` struct decode**: four-digit year, two-digit month/day/hour/minute/second, uppercase `T`, optional **dot** fraction с 1–9 ASCII digits, uppercase `Z` либо signed fixed-width `HH:MM` offset (hours 00–23, minutes 00–59). Затем native parser проверяет фактический календарь/часы. Single-digit hours, comma fractions, malformed offsets, leap seconds и **любые >9 fractional digits, включая trailing zeroes**, — schema error; округления/усечения нет.
- Правило времени применяется через typed schema traversal к `AsOf`, `Tenant.LedgerStart`, `Period.Start/End/DueAt`, `Receipt.ReceivedAt`, `Evidence.CoverageStart/CoverageThrough/ObservedAt`. Supported fractions 0–9 digits, valid offsets и разные spelling одного exact instant допустимы; byte-for-byte decode→encode equality не требуется. Explicit `null` timestamp отвергается; omitted/zero required contract times остаются native ledger errors, а omitted/zero proof boundaries — UNKNOWN.
- Каждая supplied `Evidence` **object** обязана явно содержать boolean `HasGaps`, `Truncated`, `PageUncertain`. Missing/null/wrong-type negative flag — schema error (exit 3, static stderr, no stdout), а не invented false/true evidence. Explicit unsafe `true` блокирует negative completeness proof. `Evidence` absent/null/empty допустим; omitted `Complete`/`OwnerReconciled` остаются false и не доказывают полноту. Missing proof boundaries также не авторизуют debt candidates; подтверждённое достаточное funding по-прежнему может доказать PAID.
- Presence rule — намеренное prerelease stdin schema hardening, **не изменение typed `rental.Input`/`CollectionEvidence` zero semantics и не SDK datetime compatibility**. Dates/IDs/prices/currency/mapping вводятся явно и затем проверяются engine. Никаких выдуманных реальных tenants или договорных сроков.

## Результат

Выводятся explicit copies `periods`, `allocations`, `credits`, `totals`, `owner_review`, `candidate_decisions`, `as_of`. Всегда присутствуют `reminders_enabled: false` и `bank_authorization_checked: false`. Candidate metadata не является authorisation/Telegram destination и никуда не отправляется.

Доказательство полноты в `Evidence` — assertion владельца о coherently reconciled ledger/all channels, не проведённая этим CLI банковская проверка. Без него observed positive remainder остаётся UNKNOWN. Не трактуйте fixture flags как доказанную реальную полноту.

Exit 0 — successful offline preview (включая UNKNOWN); 2 — invalid I/O/arguments; 3 — JSON/schema/size/read failure; 4 — invalid/equivocal/overflow ledger; 5 — encoding/output failure. Ошибки не включают raw input, arguments или private causes; partial decisions не выдаются при invalid input. Short output write не считается успехом.

## Проверка CLI repair cycle 1 — independent review pending

Initial CLI implementation прошла собственные gates, но **independent CLI review 1: FAIL** выявил silent subnanosecond truncation с изменением receipt allocation, DueAt и coverage decisions, а также native normalization malformed clock/fraction/offset spellings. Original failed verdict, frozen bytes и synthetic fixtures сохранены: `research/go-migration/rental-cli-review-1.json`, `rental-cli-review-1-parent-verification.json`, `rental-cli-review-1/`.

Bounded repair cycle 1 реализован с **3 separate actual RED→GREEN tracers** (precision, lexical grammar, supplied negative proof presence); complete package regression gate выполнялся после каждого GREEN. Последующее test-only strengthening production semantics не меняло. Current CLI suite: **27 regular tests, 217 subtests, 3 fuzz seed suites / 177 seed cases, no test skips**. Scoped package race/vet/build pass (`internal/rentalcli`, `internal/strictjson`, `rental`, `cmd/rental-check`); command package не содержит собственных test functions, но запускается real native subprocess regressions. Structured temporal-cutoff и proof-acceptance fuzz — **20000 actual executions each**, не только panic/live-flag checks.

Actual old/new binaries повторно запущены на всех **379 original synthetic review inputs + 10 parent controls**: **778 executions**, 368 cases с byte-identical outcome, 21 intentional new schema rejection, no unexpected change. Original bad precision/offset/omitted-proof cases теперь exit 3 с empty stdout; supported 1 ns controls сохраняют PAID/DUE/NOT_DUE/UNKNOWN, receipt availability и обе observation freshness границы. Full logs, copied fixture/source SHA manifests и proposed review snapshot — `research/go-migration/rental-cli-fix-1/` (в соседнем research tree).

Approved pure rental engine и `internal/strictjson` production bytes не изменены; текущие dependency tests/testdata включены в frozen review evidence. **Эти passing gates не являются повторным independent review или CLI acceptance**, не доказывают whole SDK port/real bank completeness и не сбрасывают separate SDK review budget. Реальные данные, login, owner profiles/credentials, tenant delivery, cron, publishing и установка не подключены. Повторное независимое review исправленной frozen CLI boundary остаётся обязательным.
