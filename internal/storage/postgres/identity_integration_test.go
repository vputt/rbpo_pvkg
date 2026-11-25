//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/httpapi"
	"github.com/vputt/rbpo_pvkg/internal/service"
	"github.com/vputt/rbpo_pvkg/internal/storage/postgres"
)

// TestRequestIdentityComesFromTheServer закрывает критерий приёмки SR-07 целиком:
// запрос идёт через настоящий роутер с проверкой сессии и настоящую базу, а защищённый
// обработчик отдаёт личность, которую положила в контекст проверка сессии.
func TestRequestIdentityComesFromTheServer(t *testing.T) {
	pool := prepareDatabase(t)
	ctx := context.Background()
	user := createTrainingUser(t, pool)

	login, err := newAuthService(t, pool).Login(ctx, "demo-user", trainingPassword)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	var probeCalls int
	router := httpapi.NewRouter(httpapi.Dependencies{
		Items: &httpapi.ItemHandlers{List: func(w http.ResponseWriter, r *http.Request) {
			probeCalls++
			identity, found := httpapi.IdentityFrom(r.Context())
			if !found {
				t.Error("protected handler was reached without an identity in the context")
			}
			w.Header().Set("Content-Type", "application/json")
			response := identityResponse{UserID: identity.UserID, Role: identity.Role}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("encode identity: %v", err)
			}
		}},
		Identify: service.NewSessions(postgres.NewSessionStore(pool)),
	})

	t.Run("no token", func(t *testing.T) {
		response := protectedCall(router, "", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
	})

	t.Run("arbitrary string as token", func(t *testing.T) {
		response := protectedCall(router, "Bearer not-a-real-token", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
	})

	t.Run("valid token", func(t *testing.T) {
		identity := identityOf(t, protectedCall(router, "Bearer "+login.Token, ""))
		if identity.UserID != user.ID || identity.Role != domain.RoleUser {
			t.Fatalf("identity = %+v, want user %d with role %s", identity, user.ID, domain.RoleUser)
		}
	})

	t.Run("identity from the request is ignored", func(t *testing.T) {
		body := `{"user_id":999,"role":"staff"}`
		identity := identityOf(t, protectedCall(router, "Bearer "+login.Token, body))
		if identity.UserID != user.ID || identity.Role != domain.RoleUser {
			t.Fatalf("identity = %+v, want the one from the session, not from the request", identity)
		}
	})

	t.Run("role change is seen by the next request", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE users SET role = $2 WHERE id = $1`, user.ID, domain.RoleStaff); err != nil {
			t.Fatalf("change the role: %v", err)
		}
		identity := identityOf(t, protectedCall(router, "Bearer "+login.Token, ""))
		if identity.Role != domain.RoleStaff {
			t.Fatalf("role = %s, want %s with the same token right after the change", identity.Role, domain.RoleStaff)
		}
	})

	t.Run("expired session", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE sessions SET expires_at = CURRENT_TIMESTAMP - interval '1 second'`); err != nil {
			t.Fatalf("expire the session: %v", err)
		}
		response := protectedCall(router, "Bearer "+login.Token, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
	})

	if probeCalls != 3 {
		t.Fatalf("protected handler calls = %d, want 3: only the three authorized requests may reach it", probeCalls)
	}
}

// Поля ответа - snake_case, как во всём API; в domain json-тегов нет по соглашению.
type identityResponse struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

func protectedCall(router http.Handler, authorization, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "/api/items?user_id=999&role=staff", strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func identityOf(t *testing.T, response *httptest.ResponseRecorder) domain.Identity {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", response.Code, http.StatusOK, response.Body.String())
	}
	var decoded identityResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode identity: %v", err)
	}
	return domain.Identity{UserID: decoded.UserID, Role: decoded.Role}
}
