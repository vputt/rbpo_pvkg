# Сервис «Найденные вещи»

Серверное приложение для учёта найденных вещей и обработки обращений за их получением.

## Документация проекта

Вся проектная документация формируется по мере прохождения этапов:

- [Паспорт проекта (PROJECT.md)](https://github.com/vputt/rbpo_pvkg/blob/main/PROJECT.md) - границы продукта, пользователи, сценарии и архитектура.
- [Требования безопасности (SECURITY_REQUIREMENTS.md)](https://github.com/vputt/rbpo_pvkg/blob/main/SECURITY_REQUIREMENTS.md) - защитные свойства и критерии приёмки.
- [Модель угроз (THREAT-MODEL.md)](https://github.com/vputt/rbpo_pvkg/blob/main/THREAT-MODEL.md) - потоки данных, границы доверия, угрозы и приоритеты.
- [Проектные решения безопасности (DESIGN-CASE.md)](https://github.com/vputt/rbpo_pvkg/blob/main/DESIGN-CASE.md) - выбранные способы защиты и планы проверок.
- [Обзор концепции безопасности (SECURITY-OVERVIEW.md)](SECURITY-OVERVIEW.md) - сценарии, граница доверия и связи угроз, требований и решений на одной странице.
- [Вклад участников (CONTRIBUTIONS.md)](https://github.com/vputt/rbpo_pvkg/blob/main/CONTRIBUTIONS.md) - конкретные результаты M1 и M2.
- [Декларация использования ИИ (AI_USAGE.md)](https://github.com/vputt/rbpo_pvkg/blob/main/AI_USAGE.md)

## Запуск

Нужен Go 1.26 или новее. Фактически проверенные окружения участников:

| Участник | Окружение |
| --- | --- |
| M1 | Go 1.27.0, macOS 15.6 (darwin/arm64) |
| M2 | Go 1.26.4, Windows (amd64) |

HTTP API использует стандартный пакет net/http, подключение к PostgreSQL - `pgx/v5`, миграции - `golang-migrate`.

### Переменные окружения

Настройки читаются только из окружения; их значений в коде нет. Пример полного набора - [.env.example](.env.example). Локальный файл `.env` в репозиторий не попадает (см. [.gitignore](.gitignore)). Приложение `.env` само не читает, переменные нужно экспортировать в окружение (см. ниже).

| Переменная | Обязательна | По умолчанию | Назначение |
| --- | --- | --- | --- |
| `PVKG_ADDR` | нет | `localhost:8080` | адрес, на котором слушает HTTP API |
| `PVKG_DB_DSN` | **да** | - | строка подключения к PostgreSQL |
| `PVKG_SESSION_TTL` | нет | `8h` | срок жизни серверной сессии (D-01) |
| `PVKG_LOGIN_MAX_FAILS` | нет | `5` | неудачных попыток входа до блокировки (SR-09) |
| `PVKG_LOGIN_LOCK_FOR` | нет | `15m` | срок блокировки после превышения порога (SR-09) |

Длительности записываются как `8h`, `30m`, `15m30s`. Переменная, заданная пустой строкой, считается незаданной и получает значение по умолчанию.

Без `PVKG_DB_DSN` сервер не запускается и сообщает, какой переменной не хватает:

```
настройки приложения: не задана обязательная переменная окружения PVKG_DB_DSN - строка подключения к PostgreSQL; пример набора настроек в .env.example
```

Сервер создаёт пул подключений из `PVKG_DB_DSN`. Недоступность базы не мешает запуску процесса: её состояние показывает `GET /ready`.

### Локальная PostgreSQL и миграции

До появления общего Docker Compose базу можно запустить одноразовым контейнером:

```powershell
docker run --name pvkg-postgres -e POSTGRES_USER=pvkg -e POSTGRES_PASSWORD=pvkg_local_password -e POSTGRES_DB=pvkg -p 55432:5432 -d postgres:17-alpine
docker exec pvkg-postgres pg_isready -U pvkg -d pvkg
```

Применение миграций из корня репозитория:

```powershell
docker run --rm --add-host=host.docker.internal:host-gateway -v "${PWD}/migrations:/migrations:ro" migrate/migrate:v4.20.1 -path=/migrations -database "postgres://pvkg:pvkg_local_password@host.docker.internal:55432/pvkg?sslmode=disable" up
```

Для интеграционных тестов один раз создать отдельную базу:

```powershell
docker exec pvkg-postgres createdb -U pvkg pvkg_test
$env:PVKG_TEST_DSN = "postgres://pvkg:pvkg_local_password@localhost:55432/pvkg_test?sslmode=disable"
go test -tags=integration ./...
```

Интеграционный тест откатывает и повторно применяет миграции в `pvkg_test`, поэтому `PVKG_TEST_DSN` нельзя направлять на рабочую базу.

### Запуск сервера

macOS и Linux (zsh, bash):

```sh
cp .env.example .env        # один раз, затем подставить локальные значения
set -a; source .env; set +a
go run ./cmd/server
```

Windows (PowerShell):

```powershell
$env:PVKG_ADDR = "localhost:8080"
$env:PVKG_DB_DSN = "postgres://pvkg:pvkg_local_password@localhost:55432/pvkg?sslmode=disable"
go run ./cmd/server
```

Сервер слушает адрес из `PVKG_ADDR`. Остановка - Ctrl+C.

В другом терминале:

```sh
curl -i http://localhost:8080/health
```

```powershell
Invoke-RestMethod http://localhost:8080/health
```

Ожидаемый ответ - HTTP 200 и `{"status":"ok"}`.

Проверка готовности приложения и базы:

```powershell
Invoke-RestMethod http://localhost:8080/ready
```

При доступной базе ответ - HTTP 200 и `{"status":"ready"}`. При недоступной базе `/ready` возвращает HTTP 503 и `{"status":"unavailable"}`, а `/health` продолжает отвечать HTTP 200. Ответ готовности не содержит строку подключения и ошибку драйвера.

## Текущий объём

Это ранний минимальный прототип, подготовленный для первого запуска проекта. В дальнейшем он будет дополняться основными сценариями и защитными механизмами.

Сейчас реализованы сервер, `/health`, `/ready`, пул подключений PostgreSQL, начальная схема БД, модель находки и хранилище в памяти. Хранилище назначает ID и статус `available`, сохраняет записи и возвращает каталог. Модель: [Item](internal/domain/item.go), контракт: [ItemRepository](internal/domain/item_repository.go), реализация: [memory.Store](internal/storage/memory/items.go).

Настройки запуска читаются из окружения пакетом [config](internal/config/config.go): адрес API уже используется сервером, остальные значения (срок сессии, порог и срок блокировки входа) пока только читаются и будут задействованы вместе с базой и входом.

Через HTTP доступны `GET /health` и `GET /ready`. Обработчики `POST /api/items` и `GET /api/items`, авторизация, обращения, выдача и репозитории PostgreSQL для предметных сущностей ещё не реализованы. Данные хранилища в памяти не сохраняются после перезапуска. Требования безопасности и проектные решения описывают целевое развитие продукта.

## Проверка

```powershell
go test ./...
go test -tags=integration ./...
go build ./...
go vet ./...
```

Два теста хранилища проверяют сохранение, независимость возвращённого каталога и создание уникальных ID при одновременной записи. Тесты пакета `internal/config` проверяют значения по умолчанию, чтение заданных значений, ошибку при отсутствии `PVKG_DB_DSN`, отказ от некорректных значений и то, что строка подключения не попадает в текст ошибок. Для проверки гонок: go test -race ./... (требуются CGO и C-компилятор).

Работоспособность процесса проверяется запросом `/health`, доступность базы - запросом `/ready`. Интеграционный тест применяет миграции и проверяет наличие пяти таблиц и частичного уникального индекса. Создание и чтение находок пока проверяются только на уровне хранилища.
