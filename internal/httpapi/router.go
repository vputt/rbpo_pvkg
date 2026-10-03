package httpapi

import (
	"io"
	"net/http"
)

type ItemHandlers struct {
	Create http.HandlerFunc
	List   http.HandlerFunc
}

func NewRouter(items *ItemHandlers) *http.ServeMux {
	router := http.NewServeMux()
	router.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{\"status\":\"ok\"}")
	})
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
