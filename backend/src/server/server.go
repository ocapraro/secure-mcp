package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"smcp/agents"
	"smcp/database"
	"smcp/ollama"
	"smcp/types"
	"strconv"
	"strings"
	"time"
)

func InitServer(mux *http.ServeMux, dbService *database.DatabaseService) {
	// No global timeout — streaming chat responses can take arbitrarily long.
	// Request-level context (r.Context()) handles cancellation when the client disconnects.
	client := &http.Client{}
	url, ok := os.LookupEnv("OLLAMA_BASE_URL")
	if !ok || strings.TrimSpace(url) == "" {
		panic("OLLAMA_BASE_URL environment variable is not set")
	}

	ollamaService := ollama.NewOllamaService(url, client)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.HealthResponse{
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
		var models types.ModelsResponse
		for _, model := range ollamaModels.Models {
			models.Models = append(models.Models, model.Name)
		}

		_ = json.NewEncoder(w).Encode(models)
	})

	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sessions, err := dbService.GetSessions()
		if err != nil {
			http.Error(w, "failed to fetch sessions", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(sessions)
	})

	mux.HandleFunc("POST /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var session database.CreateSession
		if err := json.NewDecoder(r.Body).Decode(&session); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		id, err := dbService.CreateSession(session)
		if err != nil {
			http.Error(w, "failed to fetch sessions", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(database.PartialSession{
			CreateSession: session,
			ID:            id,
			UpdatedAt:     time.Now(),
		})
	})

	mux.HandleFunc("GET /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		idParam := r.PathValue("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}

		session, err := dbService.GetSessionByID(id)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		_ = json.NewEncoder(w).Encode(session)
	})

	mux.HandleFunc("PATCH /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		idParam := r.PathValue("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}

		var update database.UpdateSession
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		session, err := dbService.UpdateSessionByID(id, update)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		_ = json.NewEncoder(w).Encode(session)
	})

	mux.HandleFunc("DELETE /api/sessions/{id}", func(w http.ResponseWriter, r *http.Request) {
		idParam := r.PathValue("id")
		id, err := strconv.ParseInt(idParam, 10, 64)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}

		err = dbService.DeleteSessionByID(id)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var chatReq ollama.OllamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(chatReq.Model) == "" {
			http.Error(w, "model is required", http.StatusBadRequest)
			return
		}
		if len(chatReq.Messages) == 0 {
			http.Error(w, "messages are required", http.StatusBadRequest)
			return
		}

		resp, err := ollamaService.SendChat(r.Context(), chatReq)
		if err != nil {
			http.Error(w, "failed to send chat to Ollama", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, "chat request failed", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)

		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 2048)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					return
				}
				// Headers/body may have already started; avoid calling http.Error after WriteHeader.
				if flusher != nil {
					flusher.Flush()
				}
				return
			}
		}
	})

	mux.HandleFunc("GET /api/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		res, err := agents.CallPlanner().Chat("how many red solo cups do americans eat a year?", ollamaService, r.Context())
		if err != nil {
			http.Error(w, "failed", http.StatusInternalServerError)
		}

		_ = json.NewEncoder(w).Encode(res)
	})
}
