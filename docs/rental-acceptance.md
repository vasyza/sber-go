# Rental domain acceptance — offline

Это отдельная прикладная матрица, не closure SDK parity. Все 15 критериев теперь `independently_verified_offline`: engine review 1 — PASS, без correctness/security blockers. Parent проверил original frozen manifest из 16 файлов, raw independent test evidence и unchanged production hashes. Review baseline: 28 regular tests, 87 regular subtests и 21 fuzz seed case. После двух test-only регрессий для observation coverage и advance DueAt проходят 30 regular tests, 92 regular subtests и прежний 21 fuzz seed case; race/vet/build pass. Два nonblocking замечания закрыты, production-код не менялся. Verdict и parent readback — `research/go-migration/rental-independent-review-1.json`. Ни coverage, ни synthetic cases не доказывают actual contracts/history или разрешение отправлять сообщения; отдельный CLI review ещё не закрыт.

- `rental.explicit-contract` — Обязательства, tenant IDs, валюты, ledger start, anchors и due dates вводятся явно; не выводятся из платежей.
- `rental.exact-allocation` — Все деньги в exact minor units с проверкой overflow; никаких binary floats/conversions.
- `rental.partial-payments` — Несколько подтверждённых поступлений покрывают один период частично/полностью без потери остатка.
- `rental.cash` — Подтверждённая owner наличная оплата учитывается отдельно от банковского наблюдения; synthetic 5000 покрывают два явно заданных 2500 периода.
- `rental.prepayment` — FIFO только по явно заданным обязательствам одного tenant; будущие периоды и excess credit не теряются.
- `rental.immutable-anchors` — Поздняя/ранняя оплата не изменяет contract anchor, period start/end или due date.
- `rental.receipt-dedup` — Stable receipt ID+совпадающая семантика учитываются один раз; конфликт одинаковых IDs fail closed.
- `rental.exact-tenant-mapping` — Никакого fuzzy matching, сходных имён, выдуманных получателей/Telegram IDs.
- `rental.unmapped-owner-review` — Непривязанные/неподтверждённые/ambiguous funds возвращаются для owner review и блокируют возможное ложное напоминание.
- `rental.known-positive-vs-unknown-debt` — Подтверждённое покрытие позволяет PAID; положительный observed remainder без доказанной полноты остаётся UNKNOWN, а не unpaid.
- `rental.history-proof` — Default false, gap/truncation/pagination uncertainty/stale coverage/no owner reconciliation => no reminder candidate.
- `rental.not-due` — `DueAt > AsOf` не производит reminder candidates. Срок оплаты независим от `Start`: авансовое обязательство с `DueAt <= AsOf < Start` при полной сверенной истории может дать metadata candidate, но не разрешение на отправку.
- `rental.invalid-input` — Missing price/date/required ID, conflicting/overlapping periods, invalid amounts/currency/overflow => error/UNKNOWN без candidate.
- `rental.determinism` — Explicit AsOf, неизменные входы, стабильный порядок; concurrent evaluations/race and conservation properties.
- `rental.no-live-delivery` — Чистый package: нет bank auth/network/delivery/wall clock. Отдельный CLI — только offline preview, не банк/MCP/cron или отправка арендаторам; live gate отдельный.

## Live gate

Реальные tenant mappings, договорные даты/тарифы и ledger start отсутствуют. Они не угадываются; seven historical notifications не считаются зачислениями. Engine проверяется на synthetic fixtures и не подключается к реальному банку или Telegram. Для live-этапа потребуются явные owner mappings/условия и подтверждённая полная история; до этого любые actual tenant reminders запрещены.
