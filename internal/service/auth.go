package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

// Длина токена сессии взята из D-01.
const sessionTokenLength = 32

var (
	ErrInvalidCredentials = errors.New("неверный логин или пароль")
	ErrLoginLocked        = errors.New("вход в запись временно заблокирован")
	ErrInvalidToken       = errors.New("токен сессии не распознан")
)

// Transactor выполняет работу в одной транзакции. По D-04 чтение записи, сравнение
// пароля и обновление счётчика попыток должны идти в одной транзакции, иначе
// параллельные попытки теряют счёт и порог не срабатывает.
type Transactor interface {
	InTransaction(ctx context.Context, fn func(users domain.UserRepository, sessions domain.SessionRepository) error) error
}

type AuthSettings struct {
	SessionTTL    time.Duration
	LoginMaxFails int
	LoginLockFor  time.Duration
}

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
}

type Auth struct {
	transactor Transactor
	settings   AuthSettings
	// dummyHash сравнивается с паролем, когда логина нет: без этого быстрый ответ
	// сам сообщает, что такой записи не существует (D-04).
	dummyHash string
}

func NewAuth(transactor Transactor, settings AuthSettings) (*Auth, error) {
	if settings.LoginMaxFails <= 0 || settings.LoginLockFor <= 0 || settings.SessionTTL <= 0 {
		return nil, errors.New("срок сессии, порог попыток и срок блокировки должны быть больше нуля")
	}

	unknown := make([]byte, sessionTokenLength)
	if _, err := rand.Read(unknown); err != nil {
		return nil, fmt.Errorf("подготовка хеша-заглушки: %w", err)
	}
	dummyHash, err := HashPassword(base64.RawURLEncoding.EncodeToString(unknown))
	if err != nil {
		return nil, err
	}
	return &Auth{transactor: transactor, settings: settings, dummyHash: dummyHash}, nil
}

// Login проверяет пароль и создаёт сессию. Неизвестный логин и неверный пароль
// возвращают одну и ту же ошибку: ответы входа не должны отличаться (D-04).
// Запись под блокировкой получает ErrLoginLocked до проверки пароля, в том числе
// с верным паролем, и сама блокировка при этом не продлевается.
func (a *Auth) Login(ctx context.Context, login, password string) (LoginResult, error) {
	login = strings.TrimSpace(login)

	var result LoginResult
	var outcome error
	err := a.transactor.InTransaction(ctx, func(users domain.UserRepository, sessions domain.SessionRepository) error {
		user, now, err := users.ByLoginForUpdate(ctx, login)
		if errors.Is(err, domain.ErrUserNotFound) {
			_ = VerifyPassword(a.dummyHash, password)
			outcome = ErrInvalidCredentials
			return nil
		}
		if err != nil {
			return err
		}

		if user.LockedAt(now) {
			outcome = ErrLoginLocked
			return nil
		}

		failures := user.FailedLoginCount
		if user.LockedUntil != nil {
			// Срок истёк: блокировка снята, и у владельца снова полный набор попыток.
			if err := users.ResetLoginFailures(ctx, user.ID); err != nil {
				return err
			}
			failures = 0
		}

		if err := VerifyPassword(user.PasswordHash, password); err != nil {
			failures, err = users.IncrementLoginFailures(ctx, user.ID)
			if err != nil {
				return err
			}
			if failures >= a.settings.LoginMaxFails {
				if _, err := users.LockLogin(ctx, user.ID, a.settings.LoginLockFor); err != nil {
					return err
				}
			}
			outcome = ErrInvalidCredentials
			return nil
		}

		if failures > 0 {
			if err := users.ResetLoginFailures(ctx, user.ID); err != nil {
				return err
			}
		}

		token, tokenHash, err := newSessionToken()
		if err != nil {
			return err
		}
		session, err := sessions.Create(ctx, tokenHash, user.ID, a.settings.SessionTTL)
		if err != nil {
			return err
		}
		result = LoginResult{Token: token, ExpiresAt: session.ExpiresAt}
		return nil
	})
	if err != nil {
		return LoginResult{}, err
	}
	return result, outcome
}

func newSessionToken() (string, []byte, error) {
	raw := make([]byte, sessionTokenLength)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("генерация токена сессии: %w", err)
	}
	hash := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw), hash[:], nil
}

// SessionTokenHash повторяет хеш по токену, пришедшему от клиента.
func SessionTokenHash(token string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != sessionTokenLength {
		return nil, ErrInvalidToken
	}
	hash := sha256.Sum256(raw)
	return hash[:], nil
}
