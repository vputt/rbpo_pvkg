package domain

import (
	"context"
	"time"
)

type Session struct {
	TokenHash []byte
	UserID    int64
	ExpiresAt time.Time
}

// SessionRepository хранит сессии. Срок действия считает само хранилище: по D-01 он
// отсчитывается от момента входа, и источник времени тот же, что у блокировки входа.
type SessionRepository interface {
	Create(ctx context.Context, tokenHash []byte, userID int64, ttl time.Duration) (Session, error)
}
