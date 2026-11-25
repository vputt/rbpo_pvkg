package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vputt/rbpo_pvkg/internal/service"
)

const invalidCredentialsBody = `{"error":{"code":"invalid_credentials","message":"неверный логин или пароль"}}`

type authStub struct {
	result      service.LoginResult
	err         error
	calls       int
	gotLogin    string
	gotPassword string
}

func (a *authStub) Login(ctx context.Context, login, password string) (service.LoginResult, error) {
	a.calls++
	a.gotLogin, a.gotPassword = login, password
	return a.result, a.err
}

func login(t *testing.T, auth Authenticator, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	response := httptest.NewRecorder()
	NewRouter(Dependencies{Auth: NewAuthHandlers(auth)}).ServeHTTP(response, request)
	return response
}

func TestLoginReturnsToken(t *testing.T) {
	expiresAt := time.Date(2026, 11, 25, 6, 0, 0, 0, time.UTC)
	auth := &authStub{result: service.LoginResult{Token: "VG9rZW4", ExpiresAt: expiresAt}}

	response := login(t, auth, `{"login":"demo-user","password":"training-password-0001"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	want := `{"token":"VG9rZW4","expires_at":"2026-11-25T06:00:00Z"}`
	if body := response.Body.String(); body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if auth.gotLogin != "demo-user" || auth.gotPassword != "training-password-0001" {
		t.Fatalf("service got login %q and a password of length %d", auth.gotLogin, len(auth.gotPassword))
	}
}

func TestLoginRejectsUnknownField(t *testing.T) {
	auth := &authStub{result: service.LoginResult{Token: "VG9rZW4"}}

	response := login(t, auth, `{"login":"demo-user","password":"training-password-0001","role":"staff"}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("body = %q, want code invalid_request", response.Body.String())
	}
	if auth.calls != 0 {
		t.Fatal("request with an unknown field must not reach the service")
	}
}

func TestLoginRejectsMalformedBody(t *testing.T) {
	cases := map[string]string{
		"truncated json": `{"login":"demo-user"`,
		"not an object":  `"demo-user"`,
		"wrong type":     `{"login":"demo-user","password":42}`,
		"empty body":     ``,
	}

	for name, body := range cases {
		auth := &authStub{}
		response := login(t, auth, body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", name, response.Code, http.StatusBadRequest)
		}
		if auth.calls != 0 {
			t.Errorf("%s: malformed body must not reach the service", name)
		}
	}
}

func TestLoginAnswersTheSameWayOnInvalidCredentials(t *testing.T) {
	auth := &authStub{err: service.ErrInvalidCredentials}

	unknownLogin := login(t, auth, `{"login":"nobody","password":"training-password-0001"}`)
	wrongPassword := login(t, auth, `{"login":"demo-user","password":"wrong-password-0001"}`)

	for _, response := range []*httptest.ResponseRecorder{unknownLogin, wrongPassword} {
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
		}
		if body := response.Body.String(); body != invalidCredentialsBody {
			t.Fatalf("body = %q, want %q", body, invalidCredentialsBody)
		}
	}
	if unknownLogin.Body.String() != wrongPassword.Body.String() {
		t.Fatal("unknown login and wrong password must give the same body")
	}
}

func TestLoginAnswersTooManyRequestsWhileLocked(t *testing.T) {
	auth := &authStub{err: service.ErrLoginLocked}

	response := login(t, auth, `{"login":"demo-user","password":"training-password-0001"}`)

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
	want := `{"error":{"code":"too_many_attempts","message":"слишком много неудачных попыток входа, попробуйте позже"}}`
	if body := response.Body.String(); body != want {
		t.Fatalf("body = %q, want %q", body, want)
	}
	if strings.Contains(response.Body.String(), "token") {
		t.Fatal("a locked account must not receive a token")
	}
}

func TestLoginHidesInternalError(t *testing.T) {
	auth := &authStub{err: errors.New(`ERROR: relation "sessions" does not exist (SQLSTATE 42P01)`)}

	response := login(t, auth, `{"login":"demo-user","password":"training-password-0001"}`)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	body := response.Body.String()
	if strings.Contains(body, "sessions") || strings.Contains(body, "SQLSTATE") {
		t.Fatalf("body = %q, want no database details", body)
	}
	if !strings.Contains(body, `"code":"internal_error"`) {
		t.Fatalf("body = %q, want code internal_error", body)
	}
}

func TestLoginRouteIsAbsentWithoutHandlers(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	NewRouter(Dependencies{}).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
