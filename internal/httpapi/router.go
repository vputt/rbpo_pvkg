package httpapi

import (
	"context"
	"io"
	"net/http"
	"time"
)

const readinessTimeout = 2 * time.Second

type ReadinessChecker interface {
	Ping(context.Context) error
}

type ItemHandlers struct {
	Create http.HandlerFunc
	List   http.HandlerFunc
}

// Dependencies - то, что роутер раздаёт по маршрутам. Защищённые маршруты оборачиваются
// проверкой сессии здесь, а не у обработчиков: иначе её легко забыть у нового маршрута.
// Без Identify защищённые маршруты не регистрируются вовсе - открытыми они не становятся.
type Dependencies struct {
	Auth      *AuthHandlers
	Items     *ItemHandlers
	Identify  Identifier
	Readiness ReadinessChecker
}

func NewRouter(deps Dependencies) *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{\"status\":\"ok\"}")
	})
	router.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if deps.Readiness == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "{\"status\":\"unavailable\"}")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		if err := deps.Readiness.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "{\"status\":\"unavailable\"}")
			return
		}

		io.WriteString(w, "{\"status\":\"ready\"}")
	})
	if deps.Auth != nil && deps.Auth.Login != nil {
		router.HandleFunc("POST /api/auth/login", deps.Auth.Login)
	}
	if deps.Items != nil && deps.Identify != nil {
		authenticated := Authenticate(deps.Identify)
		if deps.Items.Create != nil {
			router.Handle("POST /api/items", authenticated(deps.Items.Create))
		}
		if deps.Items.List != nil {
			router.Handle("GET /api/items", authenticated(deps.Items.List))
		}
	}
	return router
}
