package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

type SessionStore struct {
	db DB
}

var _ domain.SessionRepository = (*SessionStore)(nil)

func NewSessionStore(db DB) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) Create(ctx context.Context, tokenHash []byte, userID int64, ttl time.Duration) (domain.Session, error) {
	session := domain.Session{TokenHash: tokenHash, UserID: userID}
	err := s.db.QueryRow(ctx, `
		INSERT INTO sessions (token_hash, user_id, expires_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP + make_interval(secs => $3))
		RETURNING expires_at
	`, tokenHash, userID, ttl.Seconds()).Scan(&session.ExpiresAt)
	if err != nil {
		return domain.Session{}, fmt.Errorf("создание сессии: %w", err)
	}
	return session, nil
}
