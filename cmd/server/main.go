package main

import (
	"log"
	"net/http"

	"github.com/vputt/rbpo_pvkg/internal/httpapi"
)

func main() {
	const address = "localhost:8080"
	router := httpapi.NewRouter(nil)
	log.Printf("HTTP API: %s", address)
	if err := http.ListenAndServe(address, router); err != nil {
		log.Fatal(err)
	}
}
