package main

import (
	"encoding/json"
	"log"
	"net/http"
	"smcp/server"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(server.HealthResponse{
			Status: "ok",
		})
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Println("listening on :8080")
	log.Fatal(server.ListenAndServe())
}
