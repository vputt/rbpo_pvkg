package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const testDSN = "postgres://pvkg:pvkg_local_password@localhost:55432/pvkg?sslmode=disable"

// Задаёт все переменные пакета, чтобы тесты не зависели от окружения машины:
// отсутствующий в values ключ получает пустую строку, то есть считается незаданным.
func setEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for _, name := range []string{EnvAddr, EnvDBDSN, EnvSessionTTL, EnvLoginMaxFails, EnvLoginLockFor} {
		t.Setenv(name, values[name])
	}
}

func TestLoadAppliesDefaults(t *testing.T) {
	setEnv(t, map[string]string{EnvDBDSN: testDSN})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}
	if cfg.Addr != DefaultAddr {
		t.Errorf("Addr = %q, ожидался %q", cfg.Addr, DefaultAddr)
	}
	if cfg.DBDSN != testDSN {
		t.Errorf("DBDSN = %q, ожидался %q", cfg.DBDSN, testDSN)
	}
	if cfg.SessionTTL != DefaultSessionTTL {
		t.Errorf("SessionTTL = %v, ожидался %v", cfg.SessionTTL, DefaultSessionTTL)
	}
	if cfg.LoginMaxFails != DefaultLoginMaxFails {
		t.Errorf("LoginMaxFails = %d, ожидался %d", cfg.LoginMaxFails, DefaultLoginMaxFails)
	}
	if cfg.LoginLockFor != DefaultLoginLockFor {
		t.Errorf("LoginLockFor = %v, ожидался %v", cfg.LoginLockFor, DefaultLoginLockFor)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	setEnv(t, map[string]string{
		EnvAddr:          " 127.0.0.1:9090 ",
		EnvDBDSN:         testDSN,
		EnvSessionTTL:    "30m",
		EnvLoginMaxFails: "3",
		EnvLoginLockFor:  "1m30s",
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q, ожидался %q", cfg.Addr, "127.0.0.1:9090")
	}
	if cfg.SessionTTL != 30*time.Minute {
		t.Errorf("SessionTTL = %v, ожидался %v", cfg.SessionTTL, 30*time.Minute)
	}
	if cfg.LoginMaxFails != 3 {
		t.Errorf("LoginMaxFails = %d, ожидался %d", cfg.LoginMaxFails, 3)
	}
	if cfg.LoginLockFor != 90*time.Second {
		t.Errorf("LoginLockFor = %v, ожидался %v", cfg.LoginLockFor, 90*time.Second)
	}
}

func TestLoadWithoutDBDSN(t *testing.T) {
	setEnv(t, map[string]string{EnvAddr: "localhost:8080"})

	_, err := Load()
	if !errors.Is(err, ErrMissingDBDSN) {
		t.Fatalf("Load() вернул ошибку %v, ожидалась ErrMissingDBDSN", err)
	}
	if !strings.Contains(err.Error(), EnvDBDSN) {
		t.Errorf("в тексте ошибки нет имени переменной %s: %q", EnvDBDSN, err.Error())
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]struct {
		name  string
		value string
	}{
		"срок сессии не длительность":       {name: EnvSessionTTL, value: "восемь часов"},
		"срок сессии не больше нуля":        {name: EnvSessionTTL, value: "0s"},
		"срок блокировки отрицательный":     {name: EnvLoginLockFor, value: "-15m"},
		"порог попыток не число":            {name: EnvLoginMaxFails, value: "пять"},
		"порог попыток не больше нуля":      {name: EnvLoginMaxFails, value: "0"},
		"порог попыток задан длительностью": {name: EnvLoginMaxFails, value: "5m"},
	}

	for title, c := range cases {
		t.Run(title, func(t *testing.T) {
			setEnv(t, map[string]string{EnvDBDSN: testDSN, c.name: c.value})

			_, err := Load()
			if err == nil {
				t.Fatalf("Load() ошибки не вернул, ожидалась ошибка разбора %s=%q", c.name, c.value)
			}
			if !strings.Contains(err.Error(), c.name) {
				t.Errorf("в тексте ошибки нет имени переменной %s: %q", c.name, err.Error())
			}
		})
	}
}

func TestLoadErrorsDoNotContainDSN(t *testing.T) {
	setEnv(t, map[string]string{EnvDBDSN: testDSN, EnvSessionTTL: "позже"})

	_, err := Load()
	if err == nil {
		t.Fatal("Load() ошибки не вернул, ожидалась ошибка разбора срока сессии")
	}
	if strings.Contains(err.Error(), "pvkg_local_password") {
		t.Errorf("в тексте ошибки оказалось значение DSN: %q", err.Error())
	}
}
