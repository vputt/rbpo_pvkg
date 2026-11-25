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
	ErrInvalidToken       = errors.New("токен сессии не распознан")
)

type LoginResult struct {
	Token     string
	ExpiresAt time.Time
}

type Auth struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	ttl      time.Duration
	// dummyHash сравнивается с паролем, когда логина нет: без этого быстрый ответ
	// сам сообщает, что такой записи не существует (D-04).
	dummyHash string
}

func NewAuth(users domain.UserRepository, sessions domain.SessionRepository, ttl time.Duration) (*Auth, error) {
	unknown := make([]byte, sessionTokenLength)
	if _, err := rand.Read(unknown); err != nil {
		return nil, fmt.Errorf("подготовка хеша-заглушки: %w", err)
	}
	dummyHash, err := HashPassword(base64.RawURLEncoding.EncodeToString(unknown))
	if err != nil {
		return nil, err
	}
	return &Auth{users: users, sessions: sessions, ttl: ttl, dummyHash: dummyHash}, nil
}

// Login проверяет пароль и создаёт сессию. Неизвестный логин и неверный пароль
// возвращают одну и ту же ошибку: ответы входа не должны отличаться (D-04).
func (a *Auth) Login(ctx context.Context, login, password string) (LoginResult, error) {
	user, err := a.users.ByLogin(ctx, strings.TrimSpace(login))
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		_ = VerifyPassword(a.dummyHash, password)
		return LoginResult{}, ErrInvalidCredentials
	case err != nil:
		return LoginResult{}, err
	}

	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, tokenHash, err := newSessionToken()
	if err != nil {
		return LoginResult{}, err
	}
	session, err := a.sessions.Create(ctx, tokenHash, user.ID, a.ttl)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Token: token, ExpiresAt: session.ExpiresAt}, nil
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
