package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type pingFunc func(context.Context) error

func (f pingFunc) Ping(ctx context.Context) error {
	return f(ctx)
}

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		checker    ReadinessChecker
		wantStatus int
		wantBody   string
	}{
		{
			name: "database is available",
			checker: pingFunc(func(context.Context) error {
				return nil
			}),
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ready"}`,
		},
		{
			name: "database is unavailable",
			checker: pingFunc(func(context.Context) error {
				return errors.New("postgres://user:secret@db/internal_table")
			}),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable"}`,
		},
		{
			name:       "checker is missing",
			checker:    nil,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/ready", nil)
			response := httptest.NewRecorder()

			NewRouter(nil, nil, tt.checker).ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if body := response.Body.String(); body != tt.wantBody {
				t.Fatalf("body = %q, want %q", body, tt.wantBody)
			}
			if strings.Contains(response.Body.String(), "secret") {
				t.Fatal("response contains database error details")
			}
		})
	}
}

func TestHealthDoesNotPingDatabase(t *testing.T) {
	pings := 0
	checker := pingFunc(func(context.Context) error {
		pings++
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	NewRouter(nil, nil, checker).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if pings != 0 {
		t.Fatalf("database ping count = %d, want 0", pings)
	}
}
