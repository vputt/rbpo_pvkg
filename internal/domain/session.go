package domain

import (
	"context"
	"errors"
	"time"
)

var ErrSessionNotFound = errors.New("действующая сессия не найдена")

type Session struct {
	TokenHash []byte
	UserID    int64
	ExpiresAt time.Time
}

// Identity - личность запроса: кто и с какой ролью, по данным сервера. Роль читается
// вместе с сессией при каждом запросе, поэтому её изменение в БД действует сразу (SR-07).
type Identity struct {
	UserID int64
	Role   string
}

// SessionRepository хранит сессии. Срок действия считает само хранилище: по D-01 он
// отсчитывается от момента входа, и источник времени тот же, что у блокировки входа.
type SessionRepository interface {
	Create(ctx context.Context, tokenHash []byte, userID int64, ttl time.Duration) (Session, error)
	// IdentityByToken возвращает личность по хешу токена и только для неистёкшей сессии.
	IdentityByToken(ctx context.Context, tokenHash []byte) (Identity, error)
}
