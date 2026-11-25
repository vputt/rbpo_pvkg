package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vputt/rbpo_pvkg/internal/domain"
	"github.com/vputt/rbpo_pvkg/internal/service"
)

const unauthorizedBody = `{"error":{"code":"unauthorized","message":"требуется действующий токен сессии"}}`

type identifierStub struct {
	identity domain.Identity
	err      error
	calls    int
	gotToken string
}

func (i *identifierStub) Identify(ctx context.Context, token string) (domain.Identity, error) {
	i.calls++
	i.gotToken = token
	return i.identity, i.err
}

type identityProbe struct {
	calls    int
	identity domain.Identity
	found    bool
}

func (p *identityProbe) handle(w http.ResponseWriter, r *http.Request) {
	p.calls++
	p.identity, p.found = IdentityFrom(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"user_id": p.identity.UserID, "role": p.identity.Role})
}

func protectedRequest(t *testing.T, identifier Identifier, probe *identityProbe, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	router := NewRouter(Dependencies{
		Items:    &ItemHandlers{List: probe.handle},
		Identify: identifier,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestAuthenticatePutsServerIdentityIntoContext(t *testing.T) {
	identifier := &identifierStub{identity: domain.Identity{UserID: 7, Role: domain.RoleStaff}}
	probe := &identityProbe{}

	response := protectedRequest(t, identifier, probe, "Bearer IQ8Wd-token")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if identifier.gotToken != "IQ8Wd-token" {
		t.Fatalf("token passed to the session check = %q, want the one from the header", identifier.gotToken)
	}
	if !probe.found || probe.identity.UserID != 7 || probe.identity.Role != domain.RoleStaff {
		t.Fatalf("identity in the handler = %+v (found %v), want user 7 with role staff", probe.identity, probe.found)
	}
	want := `{"role":"staff","user_id":7}`
	if body := response.Body.String(); body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
}

func TestAuthenticateAcceptsCaseInsensitiveScheme(t *testing.T) {
	identifier := &identifierStub{identity: domain.Identity{UserID: 1, Role: domain.RoleUser}}
	probe := &identityProbe{}

	if response := protectedRequest(t, identifier, probe, "bearer IQ8Wd-token"); response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: the scheme is case-insensitive", response.Code, http.StatusOK)
	}
}

func TestAuthenticateRejectsUnusableHeader(t *testing.T) {
	headers := map[string]string{
		"missing":         "",
		"another scheme":  "Basic ZGVtbzpkZW1v",
		"scheme only":     "Bearer",
		"empty token":     "Bearer    ",
		"no scheme":       "IQ8Wd-token",
		"wrong separator": "Bearer=IQ8Wd-token",
	}

	for name, header := range headers {
		identifier := &identifierStub{identity: domain.Identity{UserID: 1, Role: domain.RoleUser}}
		probe := &identityProbe{}
		response := protectedRequest(t, identifier, probe, header)

		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want %d", name, response.Code, http.StatusUnauthorized)
		}
		if body := response.Body.String(); body != unauthorizedBody {
			t.Errorf("%s: body = %q, want %q", name, body, unauthorizedBody)
		}
		if identifier.calls != 0 {
			t.Errorf("%s: an unusable header must not reach the session check", name)
		}
		if probe.calls != 0 {
			t.Errorf("%s: the protected handler must not be called", name)
		}
	}
}

func TestAuthenticateRejectsUnknownAndExpiredSession(t *testing.T) {
	for name, err := range map[string]error{
		"unparsable token": service.ErrInvalidToken,
		"unknown session":  domain.ErrSessionNotFound,
	} {
		identifier := &identifierStub{err: err}
		probe := &identityProbe{}
		response := protectedRequest(t, identifier, probe, "Bearer IQ8Wd-token")

		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want %d", name, response.Code, http.StatusUnauthorized)
		}
		if body := response.Body.String(); body != unauthorizedBody {
			t.Errorf("%s: body = %q, want %q", name, body, unauthorizedBody)
		}
		if probe.calls != 0 {
			t.Errorf("%s: the protected handler must not be called", name)
		}
	}
}

func TestAuthenticateHidesInternalError(t *testing.T) {
	identifier := &identifierStub{err: errors.New(`ERROR: relation "sessions" does not exist (SQLSTATE 42P01)`)}
	probe := &identityProbe{}

	response := protectedRequest(t, identifier, probe, "Bearer IQ8Wd-token")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	body := response.Body.String()
	if strings.Contains(body, "sessions") || strings.Contains(body, "SQLSTATE") {
		t.Fatalf("body = %q, want no database details", body)
	}
	if probe.calls != 0 {
		t.Fatal("the protected handler must not be called")
	}
}

// Защищённый маршрут без проверки сессии не регистрируется: забытая проверка
// не должна превращать маршрут в открытый.
func TestProtectedRouteIsAbsentWithoutSessionCheck(t *testing.T) {
	probe := &identityProbe{}
	router := NewRouter(Dependencies{Items: &ItemHandlers{List: probe.handle}})

	request := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if probe.calls != 0 {
		t.Fatal("the protected handler must not be reachable without a session check")
	}
}

func TestOpenRoutesNeedNoToken(t *testing.T) {
	identifier := &identifierStub{}
	router := NewRouter(Dependencies{
		Auth:      NewAuthHandlers(&authStub{err: service.ErrInvalidCredentials}),
		Items:     &ItemHandlers{List: (&identityProbe{}).handle},
		Identify:  identifier,
		Readiness: pingFunc(func(context.Context) error { return nil }),
	})

	cases := []struct {
		method string
		path   string
		body   string
		token  bool
		want   int
	}{
		{http.MethodGet, "/health", "", false, http.StatusOK},
		{http.MethodGet, "/ready", "", false, http.StatusOK},
		{http.MethodPost, "/api/auth/login", `{"login":"demo-user","password":"wrong-password-0001"}`, false, http.StatusUnauthorized},
		{http.MethodGet, "/api/items", "", false, http.StatusUnauthorized},
		{http.MethodGet, "/api/items", "", true, http.StatusOK},
	}
	for _, testCase := range cases {
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		if testCase.token {
			request.Header.Set("Authorization", "Bearer IQ8Wd-token")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != testCase.want {
			t.Errorf("%s %s (token: %v): status = %d, want %d", testCase.method, testCase.path, testCase.token, response.Code, testCase.want)
		}
	}
	// Открытые маршруты сессию не проверяют: проверка сработала ровно один раз -
	// на защищённом запросе с токеном.
	if identifier.calls != 1 {
		t.Fatalf("session check calls = %d, want 1", identifier.calls)
	}
}
