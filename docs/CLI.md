# Native Go CLI — текущий offline срез

Бинарник собран локально в `bin/sber`. Это частично перенесённый CLI, **не готовый банковский коннектор**. Он не заменяет установленный Python launcher и не подключён к Hermes/MCP или cron.

## Рабочие команды

```sh
./bin/sber status --profile /explicit/private/path/profile.json
./bin/sber inspect-session --profile /explicit/private/path/profile.json
```

- `status` проверяет только безопасную файловую metadata; содержимое профиля не открывает и состояние не создаёт.
- `inspect-session` — явно выбранная владельцем offline-команда: принимает private profile v1–v4 и выводит только redacted metadata, без cookie values/deviceprints/token state. Это не банковская авторизация и не проверка текущей сессии.
- Оба результата содержат `bank_authorization_checked: false`.
- Нет автоматического поиска owner профилей и нет secret loaders из аргументов, окружения или pipe. Unsupported login/password/PIN arguments отвергаются без печати их значений.
- Exit 0 — выполненная offline-команда; 2 — некорректный вызов/отменённый контекст; 3 — unsafe/invalid profile metadata или ошибка вывода.

`login`, `doctor`, операции/балансы, profile/HAR import, MCP и rental commands ещё не wired. Не передавайте банковские секреты через Telegram. До завершения полного review нельзя использовать этот срез для подключения счёта или делать выводы о задолженности.

## Реально выполненная проверка

- RED→GREEN: новый `status`, затем `inspect-session`, затем сборка и запуск native command.
- `go test -race ./internal/cli` — 6 tests pass.
- `go vet ./internal/cli ./cmd/sber` и `go build -o bin/sber ./cmd/sber` — pass на Go 1.27.1.
- Реальный `bin/sber status` с отсутствующим synthetic path вернул `profile_exists: false` и `bank_authorization_checked: false`; owner paths не использовались.

Локальные raw logs и smoke result находятся вне git в `research/go-migration/cli-evidence`. Independent review этого CLI-среза ещё не выполнено; passing tests не являются release approval.
