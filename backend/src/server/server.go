package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"smcp/agents"
	"smcp/database"
	"smcp/openai"
	"smcp/types"
	"strconv"
	"strings"
	"time"
)

type taskResult struct {
	Task     string `json:"task"`
	Assignee string `json:"assignee"`
	Answer   string `json:"answer"`
	Error    string `json:"error,omitempty"`
}

type synthesisTaskResult struct {
	Task              string                              `json:"task"`
	Assignee          string                              `json:"assignee"`
	GeneralistAnswer  string                              `json:"generalistAnswer,omitempty"`
	SpecialistResults *agents.SpecialistExecutionEnvelope `json:"specialistResults,omitempty"`
	RawAnswer         string                              `json:"rawAnswer,omitempty"`
	Error             string                              `json:"error,omitempty"`
}

type ndjsonStreamer struct {
	enc     *json.Encoder
	flusher http.Flusher
}

func latestUserMessage(messages []openai.OllamaMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func stripMarkdownJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.Trim(raw, "`")
		raw = strings.TrimPrefix(raw, "json")
		raw = strings.TrimSpace(raw)
	}
	return raw
}

func buildSpecialistMap() map[string]agents.Specialist {
	specialistList := agents.ListSpecialists()
	specialistMap := make(map[string]agents.Specialist, len(specialistList))
	for _, s := range specialistList {
		specialistMap[strings.ToLower(s.Name)] = s
	}
	return specialistMap
}

func newNDJSONStreamer(w http.ResponseWriter) *ndjsonStreamer {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	return &ndjsonStreamer{enc: json.NewEncoder(w), flusher: flusher}
}

func (s *ndjsonStreamer) Send(content string) {
	if content == "" {
		return
	}
	_ = s.enc.Encode(openai.OllamaStreamLine{
		Message: openai.OllamaMessage{Role: "assistant", Content: content},
		Done:    false,
	})
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func (s *ndjsonStreamer) Done() {
	_ = s.enc.Encode(openai.OllamaStreamLine{Done: true})
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func markdownList(items []string) string {
	if len(items) == 0 {
		return "- None\n"
	}
	var b strings.Builder
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteString("\n")
	}
	return b.String()
}

func runDelegatedTasks(initialMessage string, specialistMap map[string]agents.Specialist, openaiService *openai.OpenAIService, chatModel string, ctx *http.Request, emit func(string)) ([]taskResult, error) {
	if emit != nil {
		emit("## Planning\n")
		emit(fmt.Sprintf("Original request: %s\n\n", initialMessage))
	}
	plannedRaw, err := agents.CallPlanner().Chat(initialMessage, openaiService, ctx.Context())
	if err != nil {
		return nil, fmt.Errorf("planner failed: %w", err)
	}

	var planned agents.PlannerResponse
	if err := json.Unmarshal([]byte(plannedRaw), &planned); err != nil {
		return nil, fmt.Errorf("failed to parse planner response: %w", err)
	}
	if emit != nil {
		emit("### Planner output\n")
		emit(fmt.Sprintf("Reasoning: %s\n\n", planned.Reasoning))
		emit("Tasks:\n")
		emit(markdownList(planned.Tasks))
		emit("\n")
	}

	delegRaw, err := agents.CallDelegator().Chat(fmt.Sprintf("Here are your tasks:%v", planned.Tasks), openaiService, ctx.Context())
	if err != nil {
		return nil, fmt.Errorf("delegator failed: %w", err)
	}

	var delegated agents.DelegatorResponse
	if err := json.Unmarshal([]byte(stripMarkdownJSON(delegRaw)), &delegated); err != nil {
		return nil, fmt.Errorf("failed to parse delegator response: %w", err)
	}
	if emit != nil {
		emit("### Delegation\n")
		emit(fmt.Sprintf("Reasoning: %s\n\n", delegated.Reasoning))
		for _, assignment := range delegated.Assignments {
			emit(fmt.Sprintf("- `%s` -> **%s**\n", assignment.Task, assignment.Assignee))
		}
		emit("\n")
	}

	results := make([]taskResult, 0, len(delegated.Assignments))
	for index, assignment := range delegated.Assignments {
		tr := taskResult{Task: assignment.Task, Assignee: assignment.Assignee}
		assigneeLower := strings.ToLower(assignment.Assignee)
		if emit != nil {
			emit(fmt.Sprintf("### Task %d\nTask: %s\nAssigned to: %s\n\n", index+1, assignment.Task, assignment.Assignee))
		}

		if assigneeLower == "generalist" {
			answer, err := openaiService.SendChatPatiently(ctx.Context(), openai.OllamaChatRequest{
				Model: chatModel,
				Messages: []openai.OllamaMessage{
					{Role: "system", Content: "You are a helpful general assistant. Answer the task accurately and concisely."},
					{Role: "user", Content: assignment.Task},
				},
			})
			if err != nil {
				tr.Error = err.Error()
				if emit != nil {
					emit(fmt.Sprintf("Generalist error: %s\n\n", tr.Error))
				}
			} else {
				tr.Answer = answer
				if emit != nil {
					emit(fmt.Sprintf("Generalist result\n\n%s\n\n", answer))
				}
			}
		} else if s, ok := specialistMap[assigneeLower]; ok {
			answer, err := agents.RunSpecialistTask(s, assignment.Task, openaiService, ctx.Context(), emit)
			if err != nil {
				tr.Error = err.Error()
				if emit != nil {
					emit(fmt.Sprintf("Specialist error: %s\n\n", tr.Error))
				}
			} else {
				tr.Answer = answer
			}
		} else {
			answer, err := openaiService.SendChatPatiently(ctx.Context(), openai.OllamaChatRequest{
				Model: chatModel,
				Messages: []openai.OllamaMessage{
					{Role: "system", Content: "You are a helpful general assistant. Answer the task accurately and concisely."},
					{Role: "user", Content: assignment.Task},
				},
			})
			if err != nil {
				tr.Error = err.Error()
				if emit != nil {
					emit(fmt.Sprintf("Fallback generalist error: %s\n\n", tr.Error))
				}
			} else {
				tr.Answer = answer
				if emit != nil {
					emit(fmt.Sprintf("Fallback generalist result\n\n%s\n\n", answer))
				}
			}
		}

		results = append(results, tr)
	}

	return results, nil
}

func buildSynthesisSummary(results []taskResult) []synthesisTaskResult {
	summary := make([]synthesisTaskResult, 0, len(results))
	for _, tr := range results {
		sr := synthesisTaskResult{
			Task:     tr.Task,
			Assignee: tr.Assignee,
			Error:    tr.Error,
		}

		if tr.Error != "" {
			summary = append(summary, sr)
			continue
		}

		if strings.EqualFold(tr.Assignee, "Generalist") {
			sr.GeneralistAnswer = tr.Answer
			summary = append(summary, sr)
			continue
		}

		var specialistOut agents.SpecialistExecutionEnvelope
		if err := json.Unmarshal([]byte(tr.Answer), &specialistOut); err == nil {
			sr.SpecialistResults = &specialistOut
		} else {
			sr.RawAnswer = tr.Answer
		}
		summary = append(summary, sr)
	}
	return summary
}

func writeNDJSONResponse(w http.ResponseWriter, content string) {
	streamer := newNDJSONStreamer(w)
	streamer.Send(content)
	streamer.Done()
}

func InitServer(mux *http.ServeMux, dbService *database.DatabaseService) {
	// No global timeout — streaming chat responses can take arbitrarily long.
	// Request-level context (r.Context()) handles cancellation when the client disconnects.
	client := &http.Client{}
	apiKey, ok := os.LookupEnv("OPENAI_API_KEY")
	if !ok || strings.TrimSpace(apiKey) == "" {
		panic("OPENAI_API_KEY environment variable is not set")
	}

	ollamaService := openai.NewOpenAIService(apiKey, client)

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
		var chatReq openai.OllamaChatRequest
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

		initialMessage := latestUserMessage(chatReq.Messages)
		if initialMessage == "" {
			http.Error(w, "a user message is required", http.StatusBadRequest)
			return
		}

		streamer := newNDJSONStreamer(w)
		streamer.Send("## Working\n\nStarting orchestration...\n\n")

		specialistMap := buildSpecialistMap()
		results, err := runDelegatedTasks(initialMessage, specialistMap, ollamaService, chatReq.Model, r, streamer.Send)
		if err != nil {
			streamer.Send(fmt.Sprintf("## Error\n\n%s\n", err.Error()))
			streamer.Done()
			return
		}

		summary := buildSynthesisSummary(results)
		summaryJSON, err := json.Marshal(summary)
		if err != nil {
			streamer.Send("## Error\n\nFailed to prepare synthesis payload.\n")
			streamer.Done()
			return
		}
		streamer.Send("## Synthesis\n\nCombining task results into the final response...\n\n")

		synthesisPrompt := fmt.Sprintf(
			"You are given the user's original request and delegated task outputs. Use ONLY these results to produce a final helpful response. If data is missing or a task failed, say so clearly.\n\nOriginal request: %s\n\nTask outputs: %s",
			initialMessage,
			string(summaryJSON),
		)

		finalResponse, err := ollamaService.SendChatPatiently(r.Context(), openai.OllamaChatRequest{
			Model: chatReq.Model,
			Messages: []openai.OllamaMessage{
				{Role: "system", Content: "You are a helpful assistant. Answer using the delegated results provided."},
				{Role: "user", Content: synthesisPrompt},
			},
		})
		if err != nil {
			streamer.Send("## Error\n\nFailed to synthesize final response.\n")
			streamer.Done()
			return
		}

		streamer.Send("## Final Response\n\n")
		streamer.Send(finalResponse)
		streamer.Done()
	})
}
