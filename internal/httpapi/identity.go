package httpapi

import (
	"context"

	"github.com/vputt/rbpo_pvkg/internal/domain"
)

type contextKey struct{}

var identityKey contextKey

// IdentityFrom возвращает личность, положенную в контекст проверкой сессии.
// Обработчики берут личность только отсюда: `user_id` и `role` из тела и параметров
// запроса не читаются нигде (SR-07).
func IdentityFrom(ctx context.Context) (domain.Identity, bool) {
	identity, ok := ctx.Value(identityKey).(domain.Identity)
	return identity, ok
}

func withIdentity(ctx context.Context, identity domain.Identity) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}
