# Проверка хранения паролей: первая часть критерия SR-09

Проверка выполнена на коммите `cdf7f4e` (ветка `feat/auth`, кусок 5).

Окружение: macOS 15.6, Apple M4 (darwin/arm64), Go 1.27.0, PostgreSQL 18.3 (Postgres.app), база `pvkg_test`. Docker-демон на машине не запущен, поэтому стенд поднят локальным сервером, а интеграционные тесты получили `PVKG_TEST_DSN` на отдельную базу.

## Сборка и статические проверки

```text
go build ./...                  PASS
go vet ./...                    PASS
go vet -tags=integration ./...  PASS
gofmt -l .                      PASS (вывод пуст)
git diff --check                PASS
```

## Юнит-тесты

```text
=== RUN   TestCreateStoresHashInsteadOfPassword
--- PASS: TestCreateStoresHashInsteadOfPassword (0.58s)
=== RUN   TestSamePasswordGetsDifferentHashes
--- PASS: TestSamePasswordGetsDifferentHashes (0.36s)
=== RUN   TestCreateRejectsInvalidInput
--- PASS: TestCreateRejectsInvalidInput (0.00s)
=== RUN   TestHashPasswordRejectsLongPassword
--- PASS: TestHashPasswordRejectsLongPassword (0.00s)
ok  	github.com/vputt/rbpo_pvkg/internal/service	2.842s
```

## Интеграционные тесты

```text
=== RUN   TestInitialSchema
--- PASS: TestInitialSchema (0.14s)
=== RUN   TestPasswordStorage
--- PASS: TestPasswordStorage (1.37s)
=== RUN   TestLoginFailureCounters
--- PASS: TestLoginFailureCounters (0.26s)
ok  	github.com/vputt/rbpo_pvkg/internal/storage/postgres	1.995s
```

`TestPasswordStorage` закрывает первую часть критерия SR-09: две записи создаются с одинаковым паролем, значения `password_hash` читаются прямо из базы, оба не содержат пароля, различаются между собой, имеют префикс `$2a$12$` и длину 60 символов; верный пароль проходит проверку, неверный - нет. Негативные случаи того же теста: повторный логин отклоняется с `ErrLoginTaken`, неизвестный логин даёт `ErrUserNotFound`.

`TestLoginFailureCounters` проверяет операции, на которых кусок 7 построит ограничение подбора: счётчик растёт по одному запросу, срок блокировки считается от `CURRENT_TIMESTAMP` сервера БД и попадает в интервал 14-15 минут при запросе на 15 минут, сброс обнуляет счётчик и снимает блокировку, а все три операции с несуществующим `id` возвращают `ErrUserNotFound`.

## Критерий приёмки на реальной таблице

Учебные записи созданы командой `cmd/seed-users` с разными паролями, затем содержимое таблицы прочитано в `psql`:

```text
   login    | role  | prefix  | len | failed_login_count | locked_until
------------+-------+---------+-----+--------------------+--------------
 demo-user  | user  | $2a$12$ |  60 |                  0 |
 demo-staff | staff | $2a$12$ |  60 |                  0 |
```

Поиск самого пароля в сохранённых значениях и сравнение двух значений между собой:

```text
 plaintext_rows
----------------
              0

 hashes_equal
--------------
 f
```

## Команда учебных записей

Отказы при неполных или небезопасных настройках:

```text
не задана переменная окружения PVKG_SEED_USER_PASSWORD - пароль учебной записи; значения по умолчанию у него нет
переменная окружения PVKG_SEED_USER_PASSWORD: пароль учебной записи короче 12 байт
пароли PVKG_SEED_USER_PASSWORD и PVKG_SEED_STAFF_PASSWORD совпадают: учебным записям нужны разные пароли
```

Успешный и повторный запуск:

```text
создана запись demo-user, роль user, id 1
создана запись demo-staff, роль staff, id 2
запись demo-user уже существует, пароль не изменён
запись demo-staff уже существует, пароль не изменён
```

Пароли задавались локально через окружение, в репозиторий и в этот отчёт они не попадают. Ни один из трёх отказов не доходит до базы, пароль не печатается ни в одном сообщении.

## Стоимость хеширования

```text
goos: darwin
goarch: arm64
pkg: github.com/vputt/rbpo_pvkg/internal/service
cpu: Apple M4
BenchmarkVerifyPassword-10    	      10	 179956342 ns/op
```

Одно сравнение пароля - 0,18 с. Предположение D-04 о «не меньше 0,2 с» на этом оборудовании не подтвердилось и исправлено по факту; порог повышения стоимости (сравнение быстрее 0,1 с) не достигнут, поэтому стоимость остаётся 12. Измерение на машине второго участника ещё предстоит.

## Пути записи пароля

Поиск по `.go` и `.sql` без тестов показывает, что `password_hash` упоминается только в схеме и в репозитории пользователей:

```text
migrations/000001_initial_schema.up.sql:4:    password_hash TEXT NOT NULL,
internal/storage/postgres/users.go:36:		INSERT INTO users (login, password_hash, role)
internal/storage/postgres/users.go:54:		SELECT id, login, password_hash, role, failed_login_count, locked_until
```

Единственный `INSERT` в таблицу `users` принимает уже готовый хеш, а получить его можно только через сервис, который хеширует пароль. Вставки пароля в открытом виде в миграциях нет. Поиск вызовов журналирования с упоминанием пароля не дал результатов.

## Ограничения проверки

- Проверена только первая часть критерия SR-09. Вход, выдача токена, ответ `429` и собственно блокировка после пяти неудачных попыток относятся к кускам 6 и 7; здесь проверены лишь операции над счётчиком и сроком блокировки.
- Стоимость хеширования измерена на одной машине (Apple M4). Для второго участника значение может отличаться, поэтому измерение повторяется в его окружении.
- Стенд - локальный PostgreSQL вместо Docker-контейнера из `README.md`. Схема применялась теми же файлами миграций.
- Устойчивость `bcrypt` к подбору не проверялась и проверяться не может: подтверждено только то, что в базе нет пароля в открытом или обратимом виде, что значения различаются при одинаковом пароле и что стоимость в хеше равна 12.
