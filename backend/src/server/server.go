package server

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

func InitServer(mux *http.ServeMux) {
	client := &http.Client{Timeout: 5 * time.Second}
	url, ok := os.LookupEnv("OLLAMA_BASE_URL")
	if !ok || strings.TrimSpace(url) == "" {
		panic("OLLAMA_BASE_URL environment variable is not set")
	}

	ollamaService := NewOllamaService(url, client)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(HealthResponse{
			Status: "ok",
		})
	})

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		health := ollamaService.GetHealth(r.Context())
		_ = json.NewEncoder(w).Encode(health)
	})

	mux.HandleFunc("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ollamaModels, err := ollamaService.GetModels(r.Context())
		if err != nil {
			http.Error(w, "failed to fetch models from Ollama", http.StatusBadGateway)
			return
		}
		var models ModelsResponse
		for _, model := range ollamaModels.Models {
			models.Models = append(models.Models, model.Name)
		}

		_ = json.NewEncoder(w).Encode(models)
	})
}
