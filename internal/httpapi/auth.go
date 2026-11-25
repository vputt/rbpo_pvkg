package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/vputt/rbpo_pvkg/internal/service"
)

// Тело запроса входа заведомо короткое; ограничение не даёт одному запросу
// заставить сервер читать произвольный объём данных.
const maxLoginBodyBytes = 4 << 10

type Authenticator interface {
	Login(ctx context.Context, login, password string) (service.LoginResult, error)
}

type AuthHandlers struct {
	Login http.HandlerFunc
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewAuthHandlers(auth Authenticator) *AuthHandlers {
	return &AuthHandlers{Login: loginHandler(auth)}
}

func loginHandler(auth Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxLoginBodyBytes))
		decoder.DisallowUnknownFields()

		var request loginRequest
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, codeInvalidRequest, "тело запроса не разобрано: ожидаются только поля login и password")
			return
		}

		result, err := auth.Login(r.Context(), request.Login, request.Password)
		switch {
		case errors.Is(err, service.ErrInvalidCredentials):
			writeError(w, http.StatusUnauthorized, codeInvalidCredentials, "неверный логин или пароль")
			return
		case err != nil:
			log.Printf("вход: %v", err)
			writeError(w, http.StatusInternalServerError, codeInternalError, messageInternalError)
			return
		}

		writeJSON(w, http.StatusOK, loginResponse{Token: result.Token, ExpiresAt: result.ExpiresAt})
	}
}
