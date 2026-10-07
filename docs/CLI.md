# CLI

Сборка: `make build`. Справка: `./bin/sber --help`. Каждая команда требует явный `--profile PATH`; автоматического поиска профилей нет.

| Команда | Поведение |
| --- | --- |
| `login` | Linux/macOS: primary login или вход по PIN с `--remembered-profile`, OTP при запросе банка; атомарное создание нового профиля |
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

Логин, пароль, OTP и PIN вводятся только в локальном терминале. `--password` и другие secret-аргументы отвергаются без печати значения. `login` держит приватный межпроцессный lock и не заменяет существующий профиль на Linux и macOS. CAPTCHA и WebAuthn требуют отдельного SDK/UI-сценария владельца; CLI останавливается без автоматических повторов.

Если системный набор CA не доверяет сертификату банка, укажите полученный из доверенного источника PEM-файл явно. Параметр `--ca-bundle PATH` поддерживается при входе, чтении и запуске MCP. Выбранный набор действует только внутри клиента SDK; системное хранилище сертификатов не изменяется. Этот параметр сохраняйте во всех последующих онлайн-командах:

```sh
./bin/sber login --profile "$HOME/.config/sber-sdk/profile.json" \
  --ca-bundle "$HOME/Downloads/Russian_Trusted_CA.pem"
./bin/sber check-session --profile "$HOME/.config/sber-sdk/profile.json" \
  --ca-bundle "$HOME/Downloads/Russian_Trusted_CA.pem"
```

Проверки цепочки TLS и имени сервера включены. Нечитаемый/некорректный CA, недоверенный сертификат, таймаут и неподдерживаемая конфигурация страницы дают отдельные безопасные сообщения; сырые ответы и remote error text не печатаются. Автоматического чтения `.env` в CLI нет.

Повторный вход по действующему PIN интернет-банка:

```sh
./bin/sber login --remembered-profile "$HOME/.config/sber-sdk/profile.json" \
  --profile "$HOME/.config/sber-sdk/profile-next.json" \
  --ca-bundle "$HOME/Downloads/Russian_Trusted_CA.pem"
```

Команда читает выбранный профиль устройства, запрашивает существующий PIN скрыто и обрабатывает один OTP при необходимости. Новый PIN при этом не создаётся. Файл назначения должен отсутствовать; исходный профиль не заменяется.

Для явного рендеринга публичной страницы входа перед нативной авторизацией задайте вместе `--browser-profile`, `--playwright-driver`, `--firefox-executable`. Все три пути абсолютные, Firefox/драйвер заранее установлены и совместимы с версией в `go.mod`. Нужен отдельный приватный профиль Firefox с проверенным NSS trust; `--ca-bundle` действует на Go-клиент. Браузер не получает секретные поля и не выполняет авторизацию. Настройка — [AUTH.md](AUTH.md).

Профиль — regular 0600, parent — 0700. `status` не создаёт директории; `inspect-session` не доказывает авторизацию. `login` сохраняет проверенный session bundle после cleanup, без пароля/PIN/OTP.

При создании онлайн-PIN CLI сообщает требуемую банком длину. Неверную длину, нецифровой ввод или несовпадение подтверждения можно исправить в том же процессе; повторная авторизация и отправка PIN на сервер при этом не выполняются.

История: `--limit` 1–100, `--max-pages` 1–10000 (default 100), `--offset` неотрицательный. Даты проверяются до открытия клиента. `--force-update` передаётся в чтение продуктов. Без дат используется ограниченное окно SDK, а не вся история.

Суммы точные, display-текст маскирует PAN. `WindowCompleteness: "unknown"` сохраняется после последней страницы. Page cap или ошибка backend дают nonzero exit и пустой stdout. SDK `Collect` отдельно предоставляет partial data вместе с ошибкой для явной обработки приложением.

Exit: 0 — выполнено; 2 — неверные аргументы/контекст; 3 — ошибка профиля, запроса, публикации, cleanup или вывода; 130 — отмена активной команды. MCP stdout содержит только протокол, diagnostics — stderr. После ошибки публикации сначала выполните `status`: fsync может завершиться ошибкой после появления файла.

Предыдущий offline-срез сохранён в [history/CLI-WIP.md](history/CLI-WIP.md). Проверки — [STATUS.md](STATUS.md).
