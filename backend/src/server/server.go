package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"smcp/agents"
	"smcp/database"
	"smcp/openai"
	"smcp/sandbox"
	"smcp/types"
	"sort"
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

type secretRequestInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Specialists []string `json:"specialists"`
	Scripts     []string `json:"scripts"`
}

type secretResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	HasValue  bool      `json:"has_value"`
	UpdatedAt time.Time `json:"updated_at"`
}

var secretNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

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

func buildSecretRequests(specialists []agents.Specialist) []secretRequestInfo {
	byName := map[string]*secretRequestInfo{}
	seenSpecialist := map[string]map[string]bool{}
	seenScript := map[string]map[string]bool{}

	for _, s := range specialists {
		for _, p := range s.Plugins {
			scriptName := filepath.Base(strings.TrimSpace(p.Path))
			for _, sec := range p.Secrets {
				name := strings.TrimSpace(sec.Name)
				if name == "" {
					continue
				}
				if !secretNamePattern.MatchString(name) {
					continue
				}

				item, ok := byName[name]
				if !ok {
					item = &secretRequestInfo{
						Name:        name,
						Description: strings.TrimSpace(sec.Value),
						Required:    strings.EqualFold(strings.TrimSpace(sec.Required), "true"),
						Specialists: []string{},
						Scripts:     []string{},
					}
					byName[name] = item
					seenSpecialist[name] = map[string]bool{}
					seenScript[name] = map[string]bool{}
				}

				if item.Description == "" {
					item.Description = strings.TrimSpace(sec.Value)
				}
				item.Required = item.Required || strings.EqualFold(strings.TrimSpace(sec.Required), "true")

				if !seenSpecialist[name][s.Name] {
					item.Specialists = append(item.Specialists, s.Name)
					seenSpecialist[name][s.Name] = true
				}
				if scriptName != "" && !seenScript[name][scriptName] {
					item.Scripts = append(item.Scripts, scriptName)
					seenScript[name][scriptName] = true
				}
			}
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]secretRequestInfo, 0, len(names))
	for _, name := range names {
		item := byName[name]
		sort.Strings(item.Specialists)
		sort.Strings(item.Scripts)
		out = append(out, *item)
	}

	return out
}

func secretNameSetFromRequests(requests []secretRequestInfo) map[string]bool {
	allowed := make(map[string]bool, len(requests)*2)
	for _, req := range requests {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			continue
		}
		allowed[name] = true
		allowed[strings.ToLower(name)] = true
	}
	return allowed
}

func redactSecrets(secrets []database.Secret) []secretResponse {
	out := make([]secretResponse, 0, len(secrets))
	for _, s := range secrets {
		out = append(out, secretResponse{
			ID:        s.ID,
			Name:      s.Name,
			HasValue:  strings.TrimSpace(s.Value) != "",
			UpdatedAt: s.UpdatedAt,
		})
	}
	return out
}

func hashSpecialistSource(s agents.Specialist) (string, error) {
	directory := strings.TrimSpace(s.Directory)
	if directory == "" {
		return "", fmt.Errorf("specialist %q has no directory", s.Name)
	}

	root := filepath.Join("../specialists", directory)
	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		rel = filepath.ToSlash(rel)
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(content)
		_, _ = h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func syncSpecialistsIntegrity(dbService *database.DatabaseService, specialists []agents.Specialist) map[string]database.SpecialistIntegrity {
	out := make(map[string]database.SpecialistIntegrity, len(specialists))
	for _, s := range specialists {
		hash, err := hashSpecialistSource(s)
		if err != nil {
			continue
		}
		record, err := dbService.UpsertAndCompareSpecialistIntegrity(database.UpsertSpecialistIntegrity{
			SpecialistName: s.Name,
			Directory:      s.Directory,
			Version:        strings.TrimSpace(s.Version),
			CurrentHash:    hash,
		})
		if err != nil {
			continue
		}
		out[s.Directory] = record
	}
	return out
}

func runDelegatedTasks(initialMessage string, specialistMap map[string]agents.Specialist, openaiService *openai.OpenAIService, chatModel string, secretValues map[string]string, ctx *http.Request, emit func(string)) ([]taskResult, error) {
	if err := sandbox.ClearSharedScriptsDir(); err != nil {
		return nil, fmt.Errorf("failed to prepare shared scripts dir: %w", err)
	}

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
	type pendingSpecialistTask struct {
		ResultIndex int
		Specialist  agents.Specialist
		Task        string
		Staged      []agents.StagedScript
	}
	pending := make([]pendingSpecialistTask, 0)

	runGeneralistTask := func(task string) (string, error) {
		answer, err := openaiService.SendChatPatiently(ctx.Context(), openai.OllamaChatRequest{
			Model: chatModel,
			Messages: []openai.OllamaMessage{
				{Role: "system", Content: "You are a helpful general assistant. Answer the task accurately and concisely."},
				{Role: "user", Content: task},
			},
		})
		if err != nil {
			return "", err
		}
		return answer, nil
	}

	for index, assignment := range delegated.Assignments {
		tr := taskResult{Task: assignment.Task, Assignee: assignment.Assignee}
		assigneeLower := strings.ToLower(assignment.Assignee)
		specialistTaskPrompt := assignment.Task
		if strings.TrimSpace(initialMessage) != "" {
			specialistTaskPrompt = fmt.Sprintf("%s\n\nUser request context: %s", assignment.Task, initialMessage)
		}
		if emit != nil {
			emit(fmt.Sprintf("### Task %d\nTask: %s\nAssigned to: %s\n\n", index+1, assignment.Task, assignment.Assignee))
		}

		if assigneeLower == "generalist" {
			answer, err := runGeneralistTask(assignment.Task)
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
			plan, err := agents.SelectSpecialistScripts(s, specialistTaskPrompt, openaiService, ctx.Context(), emit)
			if err != nil {
				tr.Error = err.Error()
				if emit != nil {
					emit(fmt.Sprintf("Specialist error: %s\n\n", err.Error()))
				}
			} else {
				staged, stageErr := agents.StageSpecialistScripts(s, specialistTaskPrompt, plan, sandbox.SharedScriptsDir(), secretValues, emit)
				if stageErr != nil {
					tr.Error = stageErr.Error()
					if emit != nil {
						emit(fmt.Sprintf("Staging error: %s\n\n", stageErr.Error()))
					}
				} else {
					pending = append(pending, pendingSpecialistTask{
						ResultIndex: len(results),
						Specialist:  s,
						Task:        assignment.Task,
						Staged:      staged,
					})
				}
			}
		} else {
			tr.Error = fmt.Sprintf("unknown assignee: %s", assignment.Assignee)
			if emit != nil {
				emit(fmt.Sprintf("Assignment error: %s\n\n", tr.Error))
			}
		}

		results = append(results, tr)
	}

	if len(pending) == 0 {
		return results, nil
	}

	if emit != nil {
		emit("### Sandbox\n")
		emit(fmt.Sprintf("Running sandbox for %d staged specialist script(s)...\n\n", len(pending)))
	}

	vmResults, err := sandbox.RunSandbox(ctx.Context())
	if err != nil {
		if emit != nil {
			emit(fmt.Sprintf("Sandbox error: `%v`\n\n", err))
		}
		for _, p := range pending {
			results[p.ResultIndex].Error = fmt.Sprintf("sandbox run failed: %v", err)
		}
		return results, nil
	}

	if emit != nil {
		emit(fmt.Sprintf("Sandbox returned %d VM message(s).\n\n", len(vmResults)))
	}

	resultByScript := make(map[string]sandbox.VMMessage, len(vmResults))
	for _, vm := range vmResults {
		if vm.Type != "result" {
			continue
		}
		if vm.Script == "" {
			continue
		}
		resultByScript[filepath.Base(vm.Script)] = vm
	}

	if emit != nil {
		emit(fmt.Sprintf("Sandbox reported %d script result(s).\n\n", len(resultByScript)))
	}

	for _, p := range pending {
		env := agents.SpecialistExecutionEnvelope{
			Specialist: p.Specialist.Name,
			Task:       p.Task,
			Results:    make([]agents.ScriptExecutionResult, 0, len(p.Staged)),
		}

		for _, staged := range p.Staged {
			entry := agents.ScriptExecutionResult{Script: staged.ScriptCall}
			if vm, ok := resultByScript[staged.StagedFile]; ok {
				if vm.OK {
					entry.Output = vm.Output
				} else {
					entry.Error = fmt.Sprintf("exit code %d: %s", vm.ExitCode, vm.Output)
					if emit != nil {
						emit(fmt.Sprintf("Sandbox script failed `%s`: `%s`\n\n", staged.StagedFile, entry.Error))
					}
				}
			} else {
				entry.Error = "no sandbox result returned for staged script"
				if emit != nil {
					emit(fmt.Sprintf("Sandbox missing result for staged script `%s`.\n\n", staged.StagedFile))
				}
			}
			env.Results = append(env.Results, entry)
		}

		encoded, marshalErr := json.Marshal(env)
		if marshalErr != nil {
			results[p.ResultIndex].Error = fmt.Sprintf("failed to encode specialist result: %v", marshalErr)
			continue
		}
		results[p.ResultIndex].Answer = string(encoded)
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

		secretValues, err := dbService.GetSecretValues()
		if err != nil {
			streamer.Send("## Error\n\nFailed to load secrets for specialist execution.\n")
			streamer.Done()
			return
		}

		specialistMap := buildSpecialistMap()
		results, err := runDelegatedTasks(initialMessage, specialistMap, ollamaService, chatReq.Model, secretValues, r, streamer.Send)
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

		// Persist specialist script execution logs.
		for _, tr := range results {
			if strings.EqualFold(tr.Assignee, "Generalist") || tr.Answer == "" {
				continue
			}
			var envelope agents.SpecialistExecutionEnvelope
			if err := json.Unmarshal([]byte(tr.Answer), &envelope); err != nil {
				continue
			}
			for _, entry := range envelope.Results {
				_ = dbService.InsertSpecialistLog(database.CreateSpecialistLog{
					Specialist: envelope.Specialist,
					Task:       envelope.Task,
					Script:     entry.Script,
					OK:         entry.Error == "",
					Output:     entry.Output,
					Error:      entry.Error,
				})
			}
		}
	})

	mux.HandleFunc("GET /api/specialists", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		type specialistInfo struct {
			Name                string `json:"name"`
			Version             string `json:"version"`
			Resume              string `json:"resume"`
			PluginCount         int    `json:"plugin_count"`
			IntegrityChanged    bool   `json:"integrity_changed"`
			ExpectedSourceHash  string `json:"expected_source_hash"`
			CurrentSourceHash   string `json:"current_source_hash"`
			SpecialistDirectory string `json:"specialist_directory"`
		}
		list := agents.ListSpecialists()
		integrityByDir := syncSpecialistsIntegrity(dbService, list)
		out := make([]specialistInfo, 0, len(list))
		for _, s := range list {
			integrity := integrityByDir[s.Directory]
			out = append(out, specialistInfo{
				Name:                s.Name,
				Version:             strings.TrimSpace(s.Version),
				Resume:              s.Resume,
				PluginCount:         len(s.Plugins),
				IntegrityChanged:    integrity.Changed,
				ExpectedSourceHash:  integrity.ExpectedHash,
				CurrentSourceHash:   integrity.CurrentHash,
				SpecialistDirectory: s.Directory,
			})
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("GET /api/specialist-logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		specialist := strings.TrimSpace(r.URL.Query().Get("specialist"))
		logs, err := dbService.GetSpecialistLogs(specialist)
		if err != nil {
			http.Error(w, "failed to query logs", http.StatusInternalServerError)
			return
		}
		if logs == nil {
			logs = []database.SpecialistLog{}
		}
		_ = json.NewEncoder(w).Encode(logs)
	})

	mux.HandleFunc("GET /api/secret-requests", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requests := buildSecretRequests(agents.ListSpecialists())
		_ = json.NewEncoder(w).Encode(requests)
	})

	mux.HandleFunc("GET /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		secrets, err := dbService.GetSecrets()
		if err != nil {
			http.Error(w, "failed to fetch secrets", http.StatusInternalServerError)
			return
		}
		if secrets == nil {
			secrets = []database.Secret{}
		}
		_ = json.NewEncoder(w).Encode(redactSecrets(secrets))
	})

	mux.HandleFunc("POST /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var payload database.CreateSecret
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		payload.Name = strings.TrimSpace(payload.Name)
		payload.Value = strings.TrimSpace(payload.Value)

		if !secretNamePattern.MatchString(payload.Name) {
			http.Error(w, "secret name must match [A-Za-z_][A-Za-z0-9_]{0,63}", http.StatusBadRequest)
			return
		}

		allowed := secretNameSetFromRequests(buildSecretRequests(agents.ListSpecialists()))
		if !allowed[payload.Name] && !allowed[strings.ToLower(payload.Name)] {
			http.Error(w, "secret name is not requested by any plugin", http.StatusBadRequest)
			return
		}

		if payload.Value == "" {
			http.Error(w, "secret value is required", http.StatusBadRequest)
			return
		}

		secret, err := dbService.UpsertSecret(payload)
		if err != nil {
			http.Error(w, "failed to save secret", http.StatusInternalServerError)
			return
		}

		_ = json.NewEncoder(w).Encode(secretResponse{
			ID:        secret.ID,
			Name:      secret.Name,
			HasValue:  strings.TrimSpace(secret.Value) != "",
			UpdatedAt: secret.UpdatedAt,
		})
	})

	mux.HandleFunc("DELETE /api/secrets/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.PathValue("name"))
		if !secretNamePattern.MatchString(name) {
			http.Error(w, "invalid secret name", http.StatusBadRequest)
			return
		}

		err := dbService.DeleteSecretByName(name)
		if err != nil {
			http.Error(w, "secret not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	})
}
