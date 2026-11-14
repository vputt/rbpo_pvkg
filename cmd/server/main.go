package main

import (
	"log"
	"net/http"

	"github.com/vputt/rbpo_pvkg/internal/config"
	"github.com/vputt/rbpo_pvkg/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("настройки приложения: %v", err)
	}
	router := httpapi.NewRouter(nil)
	log.Printf("HTTP API: %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, router); err != nil {
		log.Fatal(err)
	}
}
