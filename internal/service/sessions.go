package service

import (
	"context"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

type Sessions struct {
	sessions domain.SessionRepository
}

func NewSessions(sessions domain.SessionRepository) *Sessions {
	return &Sessions{sessions: sessions}
}

// Identify проверяет токен запроса и возвращает личность с актуальной ролью из БД.
// Нераспознанный токен до базы не доходит.
func (s *Sessions) Identify(ctx context.Context, token string) (domain.Identity, error) {
	tokenHash, err := SessionTokenHash(token)
	if err != nil {
		return domain.Identity{}, err
	}
	return s.sessions.IdentityByToken(ctx, tokenHash)
}
