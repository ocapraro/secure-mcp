package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"smcp/openai"
	"strings"
	"time"
)

type SpecialistScriptPlan struct {
	Reasoning string   `json:"reasoning"`
	Scripts   []string `json:"scripts"` // e.g. ["get-weather Boston", "fetch-forecast Boston"]
}

type ScriptExecutionResult struct {
	Script string `json:"script"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type SpecialistExecutionEnvelope struct {
	Specialist string                  `json:"specialist"`
	Task       string                  `json:"task"`
	Results    []ScriptExecutionResult `json:"results"`
}

type StagedScript struct {
	ScriptCall string `json:"scriptCall"`
	StagedFile string `json:"stagedFile"`
}

// CallSpecialist builds an Agent primed as the given specialist.
func CallSpecialist(s Specialist) *Agent {
	var pluginDocs strings.Builder
	for _, p := range s.Plugins {
		pluginDocs.WriteString(fmt.Sprintf(
			"\n- script: %s\n  description: %s\n  usage: %s\n  example: %s",
			p.Path, p.Description, p.Usage, p.Example,
		))
	}

	pluginSection := "You have no plugins available."
	if pluginDocs.Len() > 0 {
		pluginSection = fmt.Sprintf(
			"You have the following plugins available. Your job is ONLY to choose which scripts should run for the task. Do not answer the task yourself. Respond with JSON in this exact shape: {\"reasoning\":string,\"scripts\":string[]}, where each scripts item is a script name (without path) followed by args, e.g. \"get-weather Boston\". If plugins are available, scripts must contain at least one entry.%s",
			pluginDocs.String(),
		)
	}

	systemPrompt := fmt.Sprintf(
		"You are %s. %s\n\n%s\n\nYou must return valid JSON only. Never include markdown fences.",
		s.Name, s.Resume, pluginSection,
	)

	return &Agent{
		Messages: []openai.OllamaMessage{
			{Role: "system", Content: systemPrompt},
		},
		Model: MEDIUM_MODEL,
	}
}

func normalizeScriptName(scriptPath string) string {
	base := filepath.Base(strings.TrimSpace(scriptPath))
	return strings.TrimSuffix(base, ".py")
}

func parseScriptPlan(raw string) (SpecialistScriptPlan, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.Trim(raw, "`")
		raw = strings.TrimPrefix(raw, "json")
		raw = strings.TrimSpace(raw)
	}

	var plan SpecialistScriptPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return SpecialistScriptPlan{}, err
	}
	return plan, nil
}

func inferLocationFromTask(task string) string {
	lower := strings.ToLower(task)
	if strings.Contains(lower, "japan") {
		return "Japan"
	}
	if strings.Contains(lower, "boston") {
		return "Boston"
	}
	if strings.Contains(lower, "tokyo") {
		return "Tokyo"
	}
	return "New York"
}

func fallbackScriptCall(allowed map[string]string, task string) string {
	location := inferLocationFromTask(task)
	if _, ok := allowed["fetch-forecast"]; ok {
		return fmt.Sprintf("fetch-forecast %s", location)
	}
	if _, ok := allowed["get-weather"]; ok {
		return fmt.Sprintf("get-weather %s", location)
	}
	for name := range allowed {
		return fmt.Sprintf("%s %s", name, location)
	}
	return ""
}

func buildScriptArgInjector(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("import sys\nif len(sys.argv) == 1:\n    sys.argv = [__file__] + %s\n\n", string(b)), nil
}

func withInjectedArgs(source string, args []string) (string, error) {
	injector, err := buildScriptArgInjector(args)
	if err != nil {
		return "", err
	}
	if injector == "" {
		return source, nil
	}

	if strings.HasPrefix(source, "#!") {
		if i := strings.IndexByte(source, '\n'); i >= 0 {
			return source[:i+1] + injector + source[i+1:], nil
		}
		return source + "\n" + injector, nil
	}

	return injector + source, nil
}

func specialistDirName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, " ", "-"))
}

// SelectSpecialistScripts asks the specialist model which scripts should run for this task.
func SelectSpecialistScripts(s Specialist, task string, openaiService *openai.OpenAIService, ctx context.Context, emit func(string)) (SpecialistScriptPlan, error) {
	agent := CallSpecialist(s)

	message := fmt.Sprintf("{\"task\":\"%s\"}", task)
	raw, err := agent.Chat(message, openaiService, ctx)
	if err != nil {
		return SpecialistScriptPlan{}, fmt.Errorf("specialist %s error: %w", s.Name, err)
	}
	if emit != nil {
		emit(fmt.Sprintf("#### %s response\n```json\n%s\n```\n\n", s.Name, strings.TrimSpace(raw)))
	}

	plan, err := parseScriptPlan(raw)
	if err != nil {
		return SpecialistScriptPlan{}, fmt.Errorf("specialist %s returned invalid script plan", s.Name)
	}

	allowed := make(map[string]string, len(s.Plugins))
	for _, p := range s.Plugins {
		allowed[normalizeScriptName(p.Path)] = p.Path
	}
	if len(allowed) == 0 {
		return SpecialistScriptPlan{}, fmt.Errorf("specialist %s has no configured scripts", s.Name)
	}

	if len(plan.Scripts) == 0 {
		retryMessage := fmt.Sprintf("{\"task\":\"%s\",\"instruction\":\"Return at least one script call from your available plugins.\"}", task)
		retryRaw, retryErr := agent.Chat(retryMessage, openaiService, ctx)
		if retryErr != nil {
		} else {
			if emit != nil {
				emit(fmt.Sprintf("#### %s retry response\n```json\n%s\n```\n\n", s.Name, strings.TrimSpace(retryRaw)))
			}
			retryPlan, parseErr := parseScriptPlan(retryRaw)
			if parseErr == nil {
				plan = retryPlan
			}
		}
	}

	if len(plan.Scripts) == 0 {
		fallback := fallbackScriptCall(allowed, task)
		if fallback != "" {
			plan.Scripts = []string{fallback}
			if emit != nil {
				emit(fmt.Sprintf("Fallback script selected: `%s`\n\n", fallback))
			}
		}
	}

	if emit != nil && len(plan.Scripts) > 0 {
		emit("#### Scripts selected\n")
		for _, call := range plan.Scripts {
			emit(fmt.Sprintf("- `%s`\n", strings.TrimSpace(call)))
		}
		emit("\n")
	}

	validated := make([]string, 0, len(plan.Scripts))
	for _, call := range plan.Scripts {
		fields := strings.Fields(strings.TrimSpace(call))
		if len(fields) == 0 {
			continue
		}
		scriptName := normalizeScriptName(fields[0])
		if _, ok := allowed[scriptName]; !ok {
			if emit != nil {
				emit(fmt.Sprintf("Script rejected: `%s`\n\n", call))
			}
			continue
		}
		validated = append(validated, call)
	}

	if len(validated) == 0 {
		return SpecialistScriptPlan{}, fmt.Errorf("specialist %s selected no valid scripts", s.Name)
	}
	plan.Scripts = validated
	return plan, nil
}

// StageSpecialistScripts copies selected specialist scripts into sharedDir so sandbox can run them.
func StageSpecialistScripts(s Specialist, plan SpecialistScriptPlan, sharedDir string, emit func(string)) ([]StagedScript, error) {
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create shared scripts dir: %w", err)
	}

	batchID := time.Now().UnixNano()

	pluginPaths := make(map[string]string, len(s.Plugins))
	for _, p := range s.Plugins {
		pluginPaths[normalizeScriptName(p.Path)] = p.Path
	}

	specialistDir := filepath.Join(specialistsDir, specialistDirName(s.Name))
	staged := make([]StagedScript, 0, len(plan.Scripts))

	for i, call := range plan.Scripts {
		fields := strings.Fields(strings.TrimSpace(call))
		if len(fields) == 0 {
			continue
		}

		scriptName := normalizeScriptName(fields[0])
		relPath, ok := pluginPaths[scriptName]
		if !ok {
			return nil, fmt.Errorf("script %q is not configured for specialist %q", scriptName, s.Name)
		}

		sourcePath := filepath.Join(specialistDir, relPath)
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read source script %s: %w", sourcePath, err)
		}

		content, err := withInjectedArgs(string(source), fields[1:])
		if err != nil {
			return nil, fmt.Errorf("prepare script %q: %w", call, err)
		}

		stagedFile := fmt.Sprintf("%s-%s-%d-%d.py", specialistDirName(s.Name), scriptName, batchID, i+1)
		destPath := filepath.Join(sharedDir, stagedFile)
		if err := os.WriteFile(destPath, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write staged script %s: %w", destPath, err)
		}

		staged = append(staged, StagedScript{
			ScriptCall: call,
			StagedFile: stagedFile,
		})

		if emit != nil {
			emit(fmt.Sprintf("Staged `%s` as `%s`\n\n", call, stagedFile))
		}
	}

	if len(staged) == 0 {
		return nil, fmt.Errorf("no scripts staged for specialist %q", s.Name)
	}

	return staged, nil
}
