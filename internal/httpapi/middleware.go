package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/service"
)

type Identifier interface {
	Identify(ctx context.Context, token string) (domain.Identity, error)
}

// Authenticate проверяет токен сессии и кладёт личность в контекст запроса.
// Отсутствующий, неразбираемый, неизвестный и истёкший токен получают один и тот же
// ответ `401`, и обработчик при этом не вызывается.
func Authenticate(identifier Identifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				writeError(w, http.StatusUnauthorized, codeUnauthorized, messageUnauthorized)
				return
			}

			identity, err := identifier.Identify(r.Context(), token)
			switch {
			case errors.Is(err, service.ErrInvalidToken), errors.Is(err, domain.ErrSessionNotFound):
				writeError(w, http.StatusUnauthorized, codeUnauthorized, messageUnauthorized)
				return
			case err != nil:
				log.Printf("проверка сессии: %v", err)
				writeError(w, http.StatusInternalServerError, codeInternalError, messageInternalError)
				return
			}

			next.ServeHTTP(w, r.WithContext(withIdentity(r.Context(), identity)))
		})
	}
}

func bearerToken(header string) (string, bool) {
	scheme, token, found := strings.Cut(header, " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}
