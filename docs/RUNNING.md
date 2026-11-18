# Billsplit: запуск и эксплуатация

Пошаговая инструкция: как поднять проект с нуля, куда заходить, как запускать на телефоне, как останавливать, тестировать и чинить типичные проблемы.

---

## 1. Что нужно установить

| Что | Зачем | Как поставить (macOS) |
|---|---|---|
| **Docker Desktop** (запущенный) | весь бэкенд и инфраструктура | https://www.docker.com/products/docker-desktop |
| **Node.js 22+** и npm | мобильное/веб-приложение (Expo) | `brew install node` |
| Go 1.27+ *(необязательно)* | запуск Go-сервисов и тестов без Docker | `brew install go` |
| protoc + плагины *(необязательно)* | только если меняешь `.proto` | `brew install protobuf && make tools` |
| **Expo Go** на телефоне *(необязательно)* | открыть приложение на iPhone/Android | App Store / Google Play |

Docker Desktop должен иметь хотя бы **6 ГБ памяти** (Settings → Resources): Kafka, Postgres и OCR-модель вместе занимают около 3–4 ГБ.

---

## 2. Первый запуск (бэкенд + веб)

```bash
cd ~/Desktop/Billsplit

# 1. сгенерировать секреты (пароли БД, JWT, токены) в ./secrets — один раз
make secrets

# 2. собрать и поднять всё: 7 Go-сервисов, OCR, web, Postgres, Redis, Kafka, мониторинг
make up                             # = make secrets + docker compose up -d --build
```

Первая сборка занимает **5–10 минут**: скачиваются образы и OCR-модели. Дальше запуск идёт за секунды.

Проверить, что всё поднялось:

```bash
docker compose ps                   # все сервисы Up, у postgres/redis/kafka/ocr — (healthy)
curl -s localhost:8088/healthz -o /dev/null -w '%{http_code}\n'   # 200 — gateway готов
```

Затем открой **http://localhost:8081**, зарегистрируйся, и можно пользоваться.

---

## 3. Адреса

| Что | Адрес | Логин / примечание |
|---|---|---|
| **Веб-приложение** | http://localhost:8081 | регистрация прямо в приложении |
| **API (gateway)** | http://localhost:8088 | `POST /billsplit.v1.<Service>/<Method>`, JSON |
| Grafana (метрики, дашборд) | http://localhost:3000 | `admin` / пароль в `secrets/grafana_admin_password`, дашборд **Billsplit → Billsplit Overview** |
| Prometheus | http://localhost:9090 | Status → Targets: все 7 сервисов должны быть **UP** |
| Kafka UI (топики, сообщения, DLQ) | http://localhost:8085 | без логина |
| Postgres | `localhost:5432`, база `billsplit` | админ: `billsplit` / пароль в `secrets/postgres_password`; роли сервисов `auth_svc`… — пароли в `secrets/<сервис>_db_password` |
| Kafka (с хоста) | `localhost:29092` | внутри docker-сети: `kafka:9092` |
| Health-пробы gateway | http://localhost:8088/healthz, `/livez` | readiness (503 во время остановки) / liveness |

Внутренние сервисы (`auth`, `social`, `split`, `finance`, `notify`, `ocr`, Redis) **наружу не открыты**: они доступны только внутри docker-сети и требуют сервисный токен.

### Как загрузить банковскую выписку

**Finance → Import**. Затем выбери файл:
- **Freedom Bank (Казахстан)**: PDF или CSV из приложения банка (Выписка → Super Card). Операции в разных валютах (KZT, USD, TRY…) импортируются каждая в своей валюте. Переводы между своими счетами не считаются расходом, ожидающие операции (Pending) пропускаются.
- **Bank CenterCredit (BCC)**: PDF на английском или русском. Если загрузить обе версии одной выписки, дублей не будет.
- **Kaspi Bank**: PDF выписки Kaspi Gold. Парсер написан по известному формату и ещё не проверен на настоящем файле. Если не сработает, пришли пример.
- **Любой другой банк** (Halyk, Jusan, Forte, европейские…): универсальный разбор PDF и CSV по строкам с датой и суммой. Если формат не распознан, импорт сообщит «Не удалось распознать формат выписки: пришлите пример файла».
- **Американские банки** (Chase, BofA, Wells Fargo, Capital One…): CSV или OFX/QFX.

Повторная загрузка того же периода, в том числе в другом формате (PDF после CSV), дубли не создаёт. Курсы тенге и других валют, которых нет у ECB, берутся у Национального банка РК.

### Общий счёт с человеком

**Friends** → нажми на человека. Откроется один общий счёт с ним по всем общим группам и чекам. Долги в разных группах взаимно зачитываются: в «Квартире» ты должен $30, в «Поездке» тебе должны $10, итог — ты должен $20. Внизу видно, из каких групп и чеков сложилась сумма.

Кнопка **Settle up all** («Рассчитаться») или **Mark all as received** («Отметить, что получил») закрывает долги во всех группах одним действием. Список людей с ненулевым балансом есть и на главной (Groups → People).

### Как распознать чек (веб и телефон)

1. **Groups** → открыть группу (или **+ New group**).
2. Нажать **Scan receipt**.
3. Выбрать файл: **Gallery** (фото), **Camera** (на телефоне) или **PDF / File**. Подходят электронные чеки в PDF и сканы.
4. При желании указать валюту чека и чаевые, затем нажать **Upload receipt**.
5. Откроется экран «Reading your receipt…». Обычно распознавание занимает 1–3 секунды.
6. Появятся позиции. Каждый участник **нажимает на то, что брал**: долгое нажатие задаёт доли для общих позиций. Плательщик может поправить позиции (**Edit items**) и нажать **Finalize**, после чего долги появятся в балансе группы.

Названия товаров: на чеках Walmart и Target сокращения вида «GV HH 320Z» автоматически заменяются понятными названиями по штрихкоду (база Open Food Facts / Open Products Facts, бесплатно), например «Great Value Half & Half». Оригинальный текст с чека виден мелким шрифтом под названием. С ключом `ANTHROPIC_API_KEY` названия расшифровывает ещё и Claude.

Без фото: **Enter manually** (ввести позиции чека) или **Add expense** (одна сумма: аренда, такси; делится поровну, в процентах, точными суммами или по долям).

Если распознавание затянулось дольше 20 секунд, на экране появится кнопка **Enter items manually**. Сервер сам повторит распознавание через 2 минуты; если чек так и не распознается за час, он будет помечен как нераспознанный.

---

## 4. Мобильное приложение (Expo)

```bash
cd ~/Desktop/Billsplit/apps/mobile
npm install                         # один раз
```

> **Важно про порт 8081.** Web-контейнер из docker-compose уже занимает 8081, а Expo dev-сервер по умолчанию тоже стартует на 8081. Запускай dev-сервер на другом порту (`--port 8082`), как в командах ниже, либо останови контейнер: `docker compose stop web`.

### В браузере (режим разработки, с hot reload)
```bash
npx expo start --web --port 8082    # откроется http://localhost:8082
```

### На iPhone / Android через Expo Go
1. Телефон и Mac должны быть **в одной Wi-Fi сети**.
2. Узнай IP Mac: `ipconfig getifaddr en0` (например, `192.168.1.23`).
3. Запусти, указав адрес API по этому IP: `localhost` с телефона указывает на сам телефон.
   ```bash
   EXPO_PUBLIC_API_URL=http://192.168.1.23:8088 npx expo start --port 8082
   ```
4. Отсканируй QR-код: на iPhone камерой, на Android из Expo Go.

Текущий адрес сервера виден в приложении: **Settings → About**. После смены `EXPO_PUBLIC_API_URL` перезапускай с очисткой кэша: `npx expo start -c --port 8082`.

### Эмуляторы (необязательно)
Xcode и Android Studio **не нужны**: для разработки хватает веба и Expo Go на телефоне. Если эмулятор всё-таки установлен:
```bash
npx expo start --ios --port 8082       # iOS Simulator — только если уже есть Xcode
npx expo start --android --port 8082   # Android Emulator; API берётся с http://10.0.2.2:8088 автоматически
```

### Проверки фронтенда
```bash
npx tsc --noEmit                       # типы
npx eslint src                         # линтер
npx expo export --platform web         # продакшен-сборка веба в dist/
```

Push-уведомления и universal links на телефоне требуют EAS-проекта, см. раздел 8 и `apps/mobile/README.md`.

---

## 5. Быстрая проверка, что всё работает (через API)

```bash
# регистрация → получаем токен
TOKEN=$(curl -s localhost:8088/billsplit.v1.AuthService/Register \
  -d '{"email":"me@test.com","password":"password123","name":"Me","currency":"USD"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["accessToken"])')

# создать группу
curl -s localhost:8088/billsplit.v1.GroupService/CreateGroup \
  -H "Authorization: Bearer $TOKEN" -d '{"name":"Trip"}'

# валюты: публичный метод, токен не нужен
curl -s localhost:8088/billsplit.v1.FinanceService/ListCurrencies -d '{}' | head -c 300
```

Все методы описаны в `api/proto/billsplit/v1/*.proto`. Формат JSON:
- поля в camelCase;
- `*Cents` приходят строками;
- фото и выписки передаются в base64.

---

## 6. Повседневные команды

```bash
docker compose ps                               # статус
docker compose logs -f gateway split            # логи выбранных сервисов (make logs — все Go-сервисы)
docker compose logs --since 10m ocr-worker      # логи за последние 10 минут
docker compose restart split                    # перезапуск одного сервиса
docker compose up -d --build split              # пересобрать и перезапустить после изменения кода
docker compose up -d --build                    # пересобрать всё
```

### Остановка (graceful shutdown)

```bash
docker compose stop          # остановить, данные сохраняются
docker compose down          # остановить и удалить контейнеры, данные (volume) сохраняются
docker compose down -v       # ВСЁ удалить вместе с базой (чистый старт)
```

Что происходит при остановке каждого Go-сервиса (SIGTERM от Docker):

1. `/healthz` и gRPC health начинают отвечать **503 / NOT_SERVING**, чтобы балансировщик перестал слать трафик.
2. **2 секунды** (`SHUTDOWN_DRAIN_DELAY`) сервис ещё обслуживает запросы.
3. Новые соединения больше не принимаются, текущие HTTP- и gRPC-запросы **доделываются**.
4. Kafka-консьюмеры дообрабатывают сообщение, которое уже взяли, и коммитят его. Остальные сообщения остаются в Kafka и придут после рестарта: ничего не теряется и не попадает в DLQ.
5. Outbox **досылает накопившиеся события** в Kafka.
6. Закрываются Kafka-продюсеры (с досылкой буфера), Redis и Postgres.

Вся процедура ограничена `SHUTDOWN_TIMEOUT` (25 секунд). Docker ждёт 35 секунд (`stop_grace_period`). Повторный Ctrl+C или превышение таймаута завершает процесс принудительно. Обычно остановка занимает около 2 секунд. В логах это выглядит так:

```
msg="shutdown started" signal=terminated timeout=25s drain_delay=2s
msg="consumer stopped" topic=billsplit.receipt.parsed
msg="grpc: stopping, waiting for in-flight requests"
msg="grpc: stopped gracefully"
msg="outbox flushed on shutdown" events=0
msg="shutdown complete" took=2.004s
```

### База данных

```bash
docker compose exec postgres psql -U billsplit -d billsplit            # админ
docker compose exec postgres psql -U split_svc -d billsplit            # как сервис split (видит только схему split)
# внутри psql:  \dn — схемы,  \dt split.* — таблицы сервиса,  SELECT * FROM split.outbox; — неотправленные события
```

### Kafka и Redis

```bash
docker compose exec kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
docker compose exec kafka /opt/kafka/bin/kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --all-groups
docker compose exec redis redis-cli --scan --pattern 'fx:*'            # закэшированные курсы валют
```

---

## 7. Тесты

```bash
make test               # юнит-тесты (go test -race ./...)
make test-integration   # интеграционные: поднимают настоящие Postgres и Redis через testcontainers (нужен Docker)
make lint               # go vet
make proto              # перегенерировать код из .proto (после make tools)
```

Без локального Go то же самое можно запустить в контейнере:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.27 go test ./...
```

CI (`.github/workflows/ci.yml`) на каждый push и PR выполняет:
- `go vet`, юнит- и интеграционные тесты;
- проверку, что сгенерированный proto-код актуален;
- `tsc` и `eslint` фронтенда, web-сборку;
- сборку Docker-образов.

---

## 8. Настройки и ключи (`.env`)

Скопируй `cp .env.example .env`, заполни нужное, затем выполни `docker compose up -d`.

| Переменная | Что делает | Где взять |
|---|---|---|
| `JWT_SECRET` | подпись токенов, ≥ 32 символов в проде | `openssl rand -base64 48` |
| `INTERNAL_TOKEN` | секрет для вызовов между сервисами, ≥ 32 символов в проде | `openssl rand -base64 48` |
| `OCRSPACE_API_KEY` | бесплатный запасной OCR (25 000 запросов в месяц) | https://ocr.space/ocrapi |
| `ANTHROPIC_API_KEY` | Claude vision: самое точное распознавание сложных чеков (платно) | https://console.anthropic.com |
| `ANTHROPIC_MODEL` | модель Claude (по умолчанию `claude-opus-5-5`) | — |
| `EXPO_ACCESS_TOKEN` | защищённая отправка push (необязательно) | expo.dev → Access tokens |
| `WEB_URL` | адрес веба для ссылок-приглашений | по умолчанию `http://localhost:8081` |
| `SHUTDOWN_TIMEOUT`, `SHUTDOWN_DRAIN_DELAY` | тайминги graceful shutdown | по умолчанию 25s / 2s |
| `RATE_LIMIT_*` | лимиты запросов на gateway | см. `.env.example` |

Цепочка распознавания чеков: **RapidOCR** (свой сервис, всегда включён) → **OCR.space** (если задан ключ) → **Claude** (если задан ключ). Платный Claude вызывается только тогда, когда бесплатные варианты не справились.

**Push на телефоне:**
1. Выполни `cd apps/mobile && npx eas-cli@latest init`.
2. Настрой ключи APNs и FCM: `npx eas-cli credentials`.
3. Собери development build: `npx expo run:ios` / `npx expo run:android`.

Без этого push отключены, но список уведомлений в приложении (колокольчик) работает.

---

### Секреты (Docker secrets)

Все пароли, токены и API-ключи лежат файлами в `./secrets/` (в `.gitignore` и `.dockerignore`) и монтируются в контейнеры как Docker secrets в `/run/secrets/<имя>`. Сервисы получают только путь (`JWT_SECRET_FILE=/run/secrets/jwt_secret` и т.п.), поэтому значений нет ни в `docker-compose.yml`, ни в `docker inspect`, ни в образах.

| Файл | Что это |
|---|---|
| `jwt_secret`, `internal_token` | подпись токенов пользователей и токен между сервисами |
| `postgres_password`, `<сервис>_db_password` | суперпользователь (только для админа) и отдельная роль каждого сервиса |
| `redis_password`, `grafana_admin_password` | Redis (`requirepass`) и вход в Grafana |
| `ocrspace_api_key`, `anthropic_api_key`, `expo_access_token` | необязательные ключи: пустой файл = функция выключена |

Сменить пароль БД: отредактируй файл в `secrets/`, затем `make db-passwords && docker compose up -d`. Эта команда меняет пароли в существующей базе, данные не трогает. Сменить `jwt_secret`: отредактируй файл и выполни `docker compose up -d`, после этого всем пользователям нужно войти заново.

В проде используй менеджер секретов (Docker Swarm secrets, Kubernetes Secrets, Vault). Приложение читает их так же, через переменные `*_FILE`.

## 9. Частые проблемы

| Симптом | Причина и решение |
|---|---|
| `port is already allocated` | порт занят другим приложением: `lsof -i :8088` (или 8081, 5432, 3000…), освободи его или поменяй порт в `docker-compose.yml` |
| Expo: «port 8081 is being used» | это web-контейнер: запускай `npx expo start --port 8082` или `docker compose stop web` |
| Телефон: «Network request failed» | в `EXPO_PUBLIC_API_URL` указан `localhost`; нужен IP Mac (`ipconfig getifaddr en0`), телефон в той же Wi-Fi |
| Чек долго в статусе «Reading your receipt…» | после рестарта сервисов Kafka перераспределяет партиции (до ~20 секунд); если сервис был недоступен, ocr-worker повторяет попытки ~30 секунд, а split сам перезапускает распознавание зависших чеков через 2 минуты. Можно не ждать и нажать **Enter items manually** |
| `429 rate limit exceeded` | сработала защита: больше 5 попыток Login/Register/Refresh подряд с одного IP; подожди несколько секунд |
| После обновления всех разлогинило | refresh-токены теперь одноразовые и хранятся в Redis, старые недействительны; войди заново |
| `permission denied for database` при старте сервиса | volume базы создан до появления ролей; выполни `docker compose down -v && docker compose up -d` (удалит данные) |
| Сервис постоянно перезапускается | `docker compose logs <сервис>`; чаще всего Postgres или Kafka ещё не готовы, подожди healthcheck |
| Не хватает памяти, Kafka падает | увеличь память Docker Desktop до 6–8 ГБ |
| Нужно начать с чистой базы | `docker compose down -v && docker compose up -d --build` |

---

## 10. Структура проекта (коротко)

```
cmd/<service>/           точки входа: gateway, auth, social, split, finance, notify, ocr-worker
internal/<service>/      код сервиса: repository.go → service.go → grpc.go
internal/platform/       общее: config, logger, metrics, postgres, outbox, lifecycle (graceful shutdown), fx, ratelimit…
api/proto/               контракты API          pkg/pb/   сгенерированный код
migrations/<service>/    SQL-миграции (применяются автоматически при старте сервиса)
services/ocr/            Python-сервис RapidOCR
apps/mobile/             Expo-приложение (iOS, Android, Web)
deploy/                  Postgres-роли, Prometheus, Grafana
```

Подробнее об архитектуре: [README.md](../README.md).
