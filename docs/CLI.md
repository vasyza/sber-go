# CLI

Сборка: `make build`. Справка: `./bin/sber --help`. Каждая команда требует явный `--profile PATH`; автоматического поиска профилей нет.

| Команда | Поведение |
| --- | --- |
| `login` | Linux: скрытый ввод, primary login, OTP и подтверждение онлайн-PIN при запросе банка; атомарное создание нового профиля |
| `status` | Только файловая metadata, профиль не читается |
| `inspect-session` | Офлайн-чтение redacted metadata |
| `check-session` | Проверка выбранной сессии через warm-up |
| `products`, `accounts`, `cards` | Типизированные снимки продуктов |
| `operations` | Постраничная история с metadata полноты |
| `operations-page` | Одна страница и следующий offset |
| `mcp` | MCP stdio с шестью реализованными инструментами |

```sh
./bin/sber login --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber inspect-session --profile "$HOME/.local/share/sber-go/profile.json"
./bin/sber operations --profile "$HOME/.local/share/sber-go/profile.json" \
  --resource account:EXPLICIT_ID --from 2026-01-01 --to 2026-01-31 \
  --limit 30 --max-pages 100
```

Логин, пароль, OTP и PIN вводятся только в локальном терминале. `--password` и другие secret-аргументы отвергаются без печати значения. `login` держит приватный межпроцессный lock и не заменяет существующий профиль. CAPTCHA и WebAuthn требуют отдельного SDK/UI-сценария владельца; CLI останавливается без автоматических повторов. На macOS доступны SDK auth, готовые профили и MCP; скрытый CLI login ограничен Linux.

Профиль — regular 0600, parent — 0700. `status` не создаёт директории; `inspect-session` не доказывает авторизацию. `login` сохраняет проверенный session bundle после cleanup, без пароля/PIN/OTP.

История: `--limit` 1–100, `--max-pages` 1–10000 (default 100), `--offset` неотрицательный. Даты проверяются до открытия клиента. `--force-update` передаётся в чтение продуктов. Без дат используется ограниченное окно SDK, а не вся история.

Суммы точные, display-текст маскирует PAN. `WindowCompleteness: "unknown"` сохраняется после последней страницы. Page cap или ошибка backend дают nonzero exit и пустой stdout. SDK `Collect` отдельно предоставляет partial data вместе с ошибкой для явной обработки приложением.

Exit: 0 — выполнено; 2 — неверные аргументы/контекст; 3 — ошибка профиля, запроса, публикации, cleanup или вывода; 130 — отмена активной команды. MCP stdout содержит только протокол, diagnostics — stderr. После ошибки публикации сначала выполните `status`: fsync может завершиться ошибкой после появления файла.

Предыдущий offline-срез сохранён в [history/CLI-WIP.md](history/CLI-WIP.md). Проверки — [STATUS.md](STATUS.md).
