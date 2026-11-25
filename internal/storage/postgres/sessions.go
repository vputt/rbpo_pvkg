package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

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

// IdentityByToken берёт роль из таблицы пользователей тем же запросом: по D-01 роль
// читается при каждом запросе, а не запоминается в момент входа. Истёкшие сессии
// отсекаются условием по времени сервера БД.
func (s *SessionStore) IdentityByToken(ctx context.Context, tokenHash []byte) (domain.Identity, error) {
	var identity domain.Identity
	err := s.db.QueryRow(ctx, `
		SELECT sessions.user_id, users.role
		FROM sessions
		JOIN users ON users.id = sessions.user_id
		WHERE sessions.token_hash = $1
		  AND sessions.expires_at > CURRENT_TIMESTAMP
	`, tokenHash).Scan(&identity.UserID, &identity.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Identity{}, domain.ErrSessionNotFound
	}
	if err != nil {
		return domain.Identity{}, fmt.Errorf("чтение сессии: %w", err)
	}
	return identity, nil
}
