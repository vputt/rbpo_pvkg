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

func NewRouter(items *ItemHandlers, auth *AuthHandlers, readiness ReadinessChecker) *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{\"status\":\"ok\"}")
	})
	router.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if readiness == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "{\"status\":\"unavailable\"}")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
		defer cancel()
		if err := readiness.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			io.WriteString(w, "{\"status\":\"unavailable\"}")
			return
		}

		io.WriteString(w, "{\"status\":\"ready\"}")
	})
	if auth != nil && auth.Login != nil {
		router.HandleFunc("POST /api/auth/login", auth.Login)
	}
	if items != nil {
		if items.Create != nil {
			router.HandleFunc("POST /api/items", items.Create)
		}
		if items.List != nil {
			router.HandleFunc("GET /api/items", items.List)
		}
	}
	return router
}
