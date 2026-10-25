# РЖД монитор — бот MAX

Бот создаёт отслеживания билетов РЖД и отправляет уведомления в MAX.
Команды: `/start`, `/newmonitor`, `/monitors`, `/deleteall`, `/cancel`.
Кнопка запуска MAX (`bot_started`) также открывает справку. Диалог создания
отдельный для каждой пары пользователь/чат. Уведомления и список отслеживаний
привязаны к чату; в группе они общие для участников.

## Локальный запуск

Нужен Go 1.27 или новее. Создавать нового бота не требуется: используйте токен
существующего бота из настроек MAX для бизнеса.

```sh
cp .env.example .env
# Заполните MAX_BOT_TOKEN и DATABASE_URL в .env
set -a
. ./.env
set +a
go run . migrate
go run .
```

Приложение читает переменные окружения; автоматически `.env` не загружается.
По умолчанию используется Long Polling. Если у бота уже есть Webhook-подписка,
приложение завершится с объяснением конфликта. Удалите подписку через
`DELETE /subscriptions?url=...` перед переходом на polling либо используйте webhook.

## Рабочий запуск с Webhook

В `.env` задайте:

```dotenv
MAX_UPDATE_MODE=webhook
MAX_WEBHOOK_URL=https://bot.example.com/max/webhook
MAX_WEBHOOK_SECRET=your_random_secret_at_least_5_chars
MAX_LISTEN_ADDR=127.0.0.1:8080
```

Настройте reverse proxy (например, nginx) с публичного HTTPS-адреса
`https://bot.example.com/max/webhook` на `http://127.0.0.1:8080/max/webhook`.
MAX требует порт 443 и сертификат доверенного центра. Секрет: 5–256 символов
`A-Z`, `a-z`, `0-9`, `_`, `-`. Приложение само регистрирует подписку
`message_created,bot_started` при запуске и проверяет `X-Max-Bot-Api-Secret`.
Очередь событий находится в памяти; подтверждённые, но ещё не обработанные события
могут потеряться при аварийном завершении. Защита от повторных сообщений хранит
последние 10 000 ID в памяти и сбрасывается при перезапуске.

API по умолчанию: `https://platform-api2.max.ru`, токен передаётся в заголовке
`Authorization`. Docker-образ включает три сертификата Минцифры из
`certificates/`: корневой Russian Trusted Root CA и два промежуточных
Russian Trusted Sub CA (2022 и 2024). При сборке `update-ca-certificates`
добавляет их в системное хранилище Alpine; Go использует его автоматически.
Для запуска вне Docker установите эти сертификаты в системное хранилище
или задайте `SSL_CERT_FILE` с путём к полному доверенному CA bundle.
Проверка TLS включена.


Логи пишутся в stdout в формате JSON (одна запись на строку). Старая Telegram-команда `/shutdown` удалена.
Отслеживания хранятся в PostgreSQL и продолжают проверяться после перезапуска.

## PostgreSQL

Используется существующая PostgreSQL-база (например, стандартная `postgres`)
и отдельная схема `rzd_monitor`. `search_path` настраивать не нужно: все таблицы
в запросах указаны со схемой. Пример подключения:

```dotenv
DATABASE_URL=postgres://rzd_monitor:password@postgres:5432/postgres?sslmode=require
```

Пароль со специальными символами должен быть URL-encoded. Для локальной базы
без TLS можно использовать `sslmode=disable`, для проверки сертификата сервера —
`sslmode=verify-full` с доверенным CA. ID генерирует PostgreSQL; идентификаторы
чатов хранятся как BIGINT, даты — DATE. Подключение — через
[pgxpool](https://pkg.go.dev/github.com/jackc/pgx/v5/pgxpool).

### Миграции

Используется [golang-migrate](https://github.com/golang-migrate/migrate).
SQL-файлы `persistence/migrations/NNNNNN_name.up.sql` встроены в бинарник.
Команда `go run . migrate` (в контейнере — `rzd-monitor migrate`) применяет
только новые версии; повторный запуск безопасен. История хранится в
`rzd_monitor.schema_migrations`. PostgreSQL-блокировка предотвращает параллельное
применение миграций. Файлы уже применённых миграций не изменяйте: добавляйте
новый файл с очередным номером. Транзакционные миграции оборачивайте в BEGIN/COMMIT,
как в `000001_monitors.up.sql`.

Миграционная команда создаёт схему `rzd_monitor`; миграции создают и обновляют
таблицы в ней. Обычный запуск бота не выполняет DDL и проверяет версию схемы:
если миграции не применены или состояние dirty, запуск завершится ошибкой.
Первый запуск миграций требует права CREATE на базу и владения объектами схемы.
Запуск рассчитан на пустую базу: схема `rzd_monitor`, таблица отслеживаний,
индексы и таблица версий миграций создаются автоматически. Повторный запуск
сохраняет уже созданные объекты и данные.

При неуспешной миграции состояние остаётся dirty, автоматического force/reset нет.
Сначала проверьте ошибку и фактическую структуру БД, затем вручную восстановите
согласованное состояние и повторите запуск. Автоматического отката миграций нет:
`helm rollback` откатывает приложение, но не базу. Новые изменения схемы должны
быть совместимы с предыдущей версией бота, которая ещё работает во время hook Job.

Можно разделить роли: миграционный пользователь владеет схемой и выполняет DDL,
пользователь бота имеет USAGE на `rzd_monitor`, SELECT/INSERT/UPDATE/DELETE на
`rzd_monitor.monitors`, USAGE на identity sequence и SELECT на
`rzd_monitor.schema_migrations`. При добавлении таблиц выдавайте права в миграции
или настройте ALTER DEFAULT PRIVILEGES у владельца объектов.

Строка подключения и пароль не логируются. Операции базы пишутся в JSON-логи
как `database.query`, ошибки — `database.operation_failed` / `scheduler.load_failed`.
При ошибке сохранения бот не подтверждает создание отслеживания: можно повторить
ввод даты, сохранив уже введённые станции и поезд.

CSV больше не читается и не записывается; автоматического импорта старого CSV нет.
Незавершённые диалоги по-прежнему находятся в памяти. Запускайте одну реплику бота:
распределённая координация планировщика и диалогов между репликами не реализована.

## Docker

```sh
docker build -t registry.example.com/rzd-monitor:0.1.0 .
# .env должна содержать DATABASE_URL, доступный из контейнера
docker run --rm --env-file .env registry.example.com/rzd-monitor:0.1.0 migrate
docker run --rm --env-file .env registry.example.com/rzd-monitor:0.1.0
```

Многостадийная сборка использует Go 1.27, бинарник статический; runtime содержит
CA-сертификаты и tzdata, работает от UID/GID 65532. Миграции входят в тот же образ.
Для Kubernetes отправьте образ в свой registry и используйте новый неизменяемый
тег для каждого релиза. Для другой архитектуры используйте
`docker buildx build --platform linux/arm64 ...`.

## Helm

Чарт находится в `charts/rzd-monitor`. PostgreSQL в чарт не входит.
Secret с ключами `DATABASE_URL`, `MAX_BOT_TOKEN` и (для Webhook) `MAX_WEBHOOK_SECRET`
нужно создать заранее в namespace релиза: pre-install Job запускается раньше
обычных ресурсов чарта. Например, подготовьте файл с переменными и создайте Secret:

```sh
kubectl create namespace rzd-monitor
kubectl -n rzd-monitor create secret generic rzd-monitor --from-env-file=.env
helm upgrade --install rzd-monitor ./charts/rzd-monitor \
  --namespace rzd-monitor \
  --set image.repository=registry.example.com/rzd-monitor \
  --set image.tag=0.1.0 \
  --wait --timeout 5m
```

По умолчанию чарт использует polling для простого запуска. Для production задайте
`bot.mode=webhook` и `bot.webhookURL=https://your-domain/max/webhook`.
В этом режиме чарт создаёт ClusterIP Service на порту 8080. Настройте ваш HTTPS
Ingress/reverse proxy на этот Service с сохранением пути Webhook; Ingress и TLS
управляются вне чарта. `MAX_LISTEN_ADDR` в pod — `0.0.0.0:8080`.

Перед каждым install/upgrade [Helm hook](https://helm.sh/docs/topics/charts_hooks/)
запускает Job `<fullname>-migrate` с аргументом `migrate` из того же
образа. Deployment обновляется после успешного завершения Job; при ошибке релиз
останавливается, не обновляя Deployment. Успешный Job удаляется, неуспешный
сохраняется до следующей попытки. Логи ошибки:

```sh
kubectl -n rzd-monitor logs job/rzd-monitor-migrate
```

Deployment содержит одну реплику и стратегию Recreate, чтобы два бота одновременно
не опрашивали MAX и не дублировали уведомления. Это означает короткий перерыв
при обновлении. Есть startup/liveness `/livez` и readiness `/readyz` на порту 8081;
readiness проверяет PostgreSQL. Порт health не публикуется через Service.

Настройки — в `charts/rzd-monitor/values.yaml`: образ, ресурсы, Secret, параметры
Webhook, imagePullSecrets и дополнительные env/volumes. Через них можно подключить
CA bundle из заранее созданного ConfigMap/Secret и задать `SSL_CERT_FILE`.
Для отдельной роли миграций задайте `migration.existingSecret` и
`migration.databaseURLKey`; этот Secret тоже должен существовать до установки.
`migration.enabled=false` позволяет накатывать миграции отдельно в CI/CD.

## Проверка

```sh
go test -race ./...
go vet ./...
go build ./...
# Интеграционные тесты: пользователь с CREATEDB; тесты создают и удаляют свои базы
TEST_DATABASE_URL='postgres://user:password@localhost:5432/test_db?sslmode=disable' go test ./persistence -v
```

Документация MAX:
- [Обзор и авторизация](https://dev.max.ru/docs-api)
- [Отправка сообщений](https://dev.max.ru/docs-api/methods/POST/messages)
- [Long Polling](https://dev.max.ru/docs-api/methods/GET/updates)
- [Webhook](https://dev.max.ru/docs-api/methods/POST/subscriptions)

## Логи в Kubernetes

По умолчанию `LOG_LEVEL=INFO`. Доступны `DEBUG`, `INFO`, `WARN`, `ERROR`.
Тела запросов и ответов выводятся полностью в `request_body` и `response_body`,
без ограничения длины, включая не-JSON ответы. JSON-поля с token/secret/password,
Authorization и Cookie маскируются, секреты из переменных окружения также скрываются.
Заголовки не логируются. Тела могут содержать тексты сообщений и идентификаторы
пользователей. Переменная `LOG_BODY_LIMIT` больше не используется.

```yaml
env:
  - name: LOG_LEVEL
    value: INFO
```

События `http.request` и `http.response` связаны полем `request_id` в пределах
процесса; `service` — `max` или `rzd`. Ответ содержит `status`, `duration_ms`,
`response_body`; сетевые ошибки — `http.request_failed`. Тела HTTP-ответов
логируются после чтения или закрытия потока. Ошибочные HTTP-статусы и ошибки
чтения имеют уровень ERROR. Также логируются запуск/остановка, команды,
состояния диалога, Webhook-запросы, проверки билетов и операции хранилища.
`DEBUG` добавляет пропущенные/повторные события и обращения к хранилищу.

Просмотр: `kubectl logs -f -l app.kubernetes.io/component=bot`.

## Семейное купе: четыре места вместе

Монитор теперь ищет обычные купейные места (`CarType=Compartment`), исключая
запрещённые к продаже поезда/группы и группы с `HasPlacesForDisabledPersons=true`.
Последнее — консервативное исключение всей группы, а не определение ограничений
каждого отдельного места. `HasFssBenefit` не используется как признак спецмест.

Уведомление отправляется, только когда в подходящей группе
`EmptyCabinQuantity > 0`. Общее число свободных мест и количество нижних/верхних
не заменяют этот критерий. Уведомление сообщает о наличии пустого купе без
указания количества купе. По предоставленному примеру `EmptyCabinQuantity`,
вероятно, считает места в пустых купе: значение 24 при 24 местах не означает
24 купе. Отдельный запрос детализации не выполняется.

В логах `monitor.result` выводятся обычные купейные места, сумма исходных значений
`EmptyCabinQuantity` (`empty_cabin_quantity_sum`) и статус `unavailable` либо `available`.
Пока купе остаются доступными, уведомление повторяется при каждой минутной проверке.
