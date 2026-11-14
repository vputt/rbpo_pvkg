// Пакет config читает настройки приложения из переменных окружения.
// Значений настроек в коде и в репозитории нет; пример набора - .env.example.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Имена переменных окружения.
const (
	EnvAddr          = "PVKG_ADDR"
	EnvDBDSN         = "PVKG_DB_DSN"
	EnvSessionTTL    = "PVKG_SESSION_TTL"
	EnvLoginMaxFails = "PVKG_LOGIN_MAX_FAILS"
	EnvLoginLockFor  = "PVKG_LOGIN_LOCK_FOR"
)

// Значения по умолчанию взяты из документов проекта:
// срок сессии - решение D-01, порог попыток и срок блокировки - требование SR-09.
const (
	DefaultAddr          = "localhost:8080"
	DefaultSessionTTL    = 8 * time.Hour
	DefaultLoginMaxFails = 5
	DefaultLoginLockFor  = 15 * time.Minute
)

// ErrMissingDBDSN возвращается, когда не задана обязательная настройка PVKG_DB_DSN.
var ErrMissingDBDSN = fmt.Errorf(
	"не задана обязательная переменная окружения %s - строка подключения к PostgreSQL; пример набора настроек в .env.example",
	EnvDBDSN,
)

// Config - настройки одного запуска приложения.
type Config struct {
	// Addr - адрес, на котором слушает HTTP API.
	Addr string
	// DBDSN - строка подключения к PostgreSQL.
	// Содержит пароль: не печатать в журнал и не включать в ответы API.
	DBDSN string
	// SessionTTL - срок жизни серверной сессии (D-01).
	SessionTTL time.Duration
	// LoginMaxFails - число неудачных попыток входа до блокировки (SR-09).
	LoginMaxFails int
	// LoginLockFor - срок блокировки после превышения порога попыток (SR-09).
	LoginLockFor time.Duration
}

// Load читает настройки из окружения.
// Необязательные настройки, которые не заданы или заданы пустой строкой,
// получают значения по умолчанию. Если не задана PVKG_DB_DSN или значение
// какой-либо переменной нельзя разобрать, возвращается ошибка с именем
// переменной; значение DSN в текст ошибки не попадает.
func Load() (Config, error) {
	cfg := Config{
		Addr:          DefaultAddr,
		SessionTTL:    DefaultSessionTTL,
		LoginMaxFails: DefaultLoginMaxFails,
		LoginLockFor:  DefaultLoginLockFor,
	}

	if addr, ok := lookup(EnvAddr); ok {
		cfg.Addr = addr
	}

	dsn, ok := lookup(EnvDBDSN)
	if !ok {
		return Config{}, ErrMissingDBDSN
	}
	cfg.DBDSN = dsn

	sessionTTL, err := duration(EnvSessionTTL, cfg.SessionTTL)
	if err != nil {
		return Config{}, err
	}
	cfg.SessionTTL = sessionTTL

	loginMaxFails, err := count(EnvLoginMaxFails, cfg.LoginMaxFails)
	if err != nil {
		return Config{}, err
	}
	cfg.LoginMaxFails = loginMaxFails

	loginLockFor, err := duration(EnvLoginLockFor, cfg.LoginLockFor)
	if err != nil {
		return Config{}, err
	}
	cfg.LoginLockFor = loginLockFor

	return cfg, nil
}

// lookup возвращает значение переменной окружения без крайних пробелов.
// Незаданная переменная и переменная с пустым значением равнозначны: ok = false.
func lookup(name string) (string, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", false
	}
	return value, true
}

// duration читает длительность вида 8h, 30m, 15m30s.
func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw, ok := lookup(name)
	if !ok {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("переменная окружения %s=%q: ожидается длительность, например 8h, 30m или 15m30s", name, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("переменная окружения %s=%q: ожидается длительность больше нуля", name, raw)
	}
	return value, nil
}

// count читает целое число больше нуля.
func count(name string, fallback int) (int, error) {
	raw, ok := lookup(name)
	if !ok {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("переменная окружения %s=%q: ожидается целое число", name, raw)
	}
	if value <= 0 {
		return 0, fmt.Errorf("переменная окружения %s=%q: ожидается целое число больше нуля", name, raw)
	}
	return value, nil
}
