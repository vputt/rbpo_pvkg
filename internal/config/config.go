// Пакет config читает настройки приложения из переменных окружения.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvAddr          = "PVKG_ADDR"
	EnvDBDSN         = "PVKG_DB_DSN"
	EnvSessionTTL    = "PVKG_SESSION_TTL"
	EnvLoginMaxFails = "PVKG_LOGIN_MAX_FAILS"
	EnvLoginLockFor  = "PVKG_LOGIN_LOCK_FOR"
)

// Срок сессии взят из D-01, порог попыток и срок блокировки - из SR-09.
const (
	DefaultAddr          = "localhost:8080"
	DefaultSessionTTL    = 8 * time.Hour
	DefaultLoginMaxFails = 5
	DefaultLoginLockFor  = 15 * time.Minute
)

var ErrMissingDBDSN = fmt.Errorf(
	"не задана обязательная переменная окружения %s - строка подключения к PostgreSQL; пример набора настроек в .env.example",
	EnvDBDSN,
)

type Config struct {
	Addr string
	// DBDSN содержит пароль: не печатать в журнал, в ответы API и в сообщения об ошибках.
	DBDSN         string
	SessionTTL    time.Duration
	LoginMaxFails int
	LoginLockFor  time.Duration
}

// Load читает настройки из окружения. Незаданная или пустая переменная получает
// значение по умолчанию; обязательна только PVKG_DB_DSN.
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

func lookup(name string) (string, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", false
	}
	return value, true
}

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
