package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"smcp/openai"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
		argDocs := "none"
		if len(p.Arguments) > 0 {
			argParts := make([]string, 0, len(p.Arguments))
			for _, a := range p.Arguments {
				name := strings.TrimSpace(a.Name)
				if name == "" {
					continue
				}
				required := strings.EqualFold(strings.TrimSpace(a.Required), "true")
				desc := strings.TrimSpace(a.Value)
				if desc == "" {
					desc = "no description provided"
				}
				argParts = append(argParts, fmt.Sprintf("%s(required=%t): %s", name, required, desc))
			}
			if len(argParts) > 0 {
				argDocs = strings.Join(argParts, "; ")
			}
		}

		pluginDocs.WriteString(fmt.Sprintf(
			"\n- script: %s\n  description: %s\n  usage: %s\n  arguments: %s\n  example: %s",
			p.Path, p.Description, p.Usage, argDocs, p.Example,
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
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "`")
	raw = strings.TrimSpace(raw)

	var plan SpecialistScriptPlan
	if err := json.Unmarshal([]byte(raw), &plan); err == nil {
		return plan, nil
	}

	if candidate, ok := extractFirstJSONObject(raw); ok {
		if err := json.Unmarshal([]byte(candidate), &plan); err == nil {
			return plan, nil
		}
	}

	// Tolerate common LLM formatting issue: one or more extra trailing '}' characters.
	fixed := strings.TrimSpace(raw)
	for strings.HasSuffix(fixed, "}") {
		openCount := strings.Count(fixed, "{")
		closeCount := strings.Count(fixed, "}")
		if closeCount <= openCount {
			break
		}
		fixed = strings.TrimSpace(fixed[:len(fixed)-1])
		if err := json.Unmarshal([]byte(fixed), &plan); err == nil {
			return plan, nil
		}
	}

	return SpecialistScriptPlan{}, fmt.Errorf("invalid specialist script plan JSON")
}

func extractFirstJSONObject(raw string) (string, bool) {
	start := strings.IndexByte(raw, '{')
	if start < 0 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(raw); i++ {
		c := raw[i]

		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}

		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(raw[start : i+1]), true
			}
		}
	}

	return "", false
}

func fallbackScriptCall(allowed map[string]string) string {
	if _, ok := allowed["fetch-forecast"]; ok {
		return "fetch-forecast <location>"
	}
	if _, ok := allowed["get-weather"]; ok {
		return "get-weather <location>"
	}
	for name := range allowed {
		return name
	}
	return ""
}

var tokenLiteralPattern = regexp.MustCompile(`"__TOKEN_([A-Za-z_][A-Za-z0-9_]*)(?::(string|int|float|bool))?__"`)
var safeStringPattern = regexp.MustCompile(`^[\p{L}\p{N} .,'_\-/]{1,200}$`)

func isRequiredArg(required string) bool {
	return strings.EqualFold(strings.TrimSpace(required), "true")
}

func sanitizeTypedArg(raw string, argType string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("invalid UTF-8")
	}

	switch argType {
	case "", "string":
		if raw == "" {
			return "", fmt.Errorf("must not be empty")
		}
		if !safeStringPattern.MatchString(raw) {
			return "", fmt.Errorf("contains unsupported characters")
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case "int":
		v, err := strconv.Atoi(raw)
		if err != nil {
			return "", fmt.Errorf("must be an integer")
		}
		return strconv.Itoa(v), nil
	case "float":
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "", fmt.Errorf("must be a number")
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case "bool":
		v, err := strconv.ParseBool(strings.ToLower(raw))
		if err != nil {
			return "", fmt.Errorf("must be true or false")
		}
		if v {
			return "True", nil
		}
		return "False", nil
	default:
		return "", fmt.Errorf("unsupported token type %q", argType)
	}
}

func mapScriptArgs(plugin Plugin, rawArgs []string) (map[string]string, error) {
	values := make(map[string]string)
	defs := plugin.Arguments

	if len(defs) == 0 {
		if len(rawArgs) > 0 {
			return nil, fmt.Errorf("script does not declare arguments, but values were provided")
		}
		return values, nil
	}

	for i, def := range defs {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			return nil, fmt.Errorf("plugin argument with empty name")
		}

		var v string
		if i == len(defs)-1 {
			if len(rawArgs) > i {
				v = strings.Join(rawArgs[i:], " ")
			}
		} else if len(rawArgs) > i {
			v = rawArgs[i]
		}

		v = strings.TrimSpace(v)

		if strings.TrimSpace(v) == "" {
			if isRequiredArg(def.Required) {
				return nil, fmt.Errorf("missing required argument %q", name)
			}
			continue
		}
		values[name] = v
	}

	return values, nil
}

func replaceTokensWithSanitizedLiterals(source string, argValues map[string]string) (string, error) {
	var replaceErr error

	replaced := tokenLiteralPattern.ReplaceAllStringFunc(source, func(m string) string {
		if replaceErr != nil {
			return m
		}
		parts := tokenLiteralPattern.FindStringSubmatch(m)
		if len(parts) < 3 {
			replaceErr = fmt.Errorf("invalid token format %q", m)
			return m
		}
		name := parts[1]
		argType := parts[2]

		raw, ok := argValues[name]
		if !ok {
			replaceErr = fmt.Errorf("missing value for token %q", name)
			return m
		}

		lit, err := sanitizeTypedArg(raw, argType)
		if err != nil {
			replaceErr = fmt.Errorf("invalid value for %q: %w", name, err)
			return m
		}
		return lit
	})

	if replaceErr != nil {
		return "", replaceErr
	}

	if tokenLiteralPattern.MatchString(replaced) {
		return "", fmt.Errorf("unresolved token remains after conversion")
	}

	return replaced, nil
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
		fallback := fallbackScriptCall(allowed)
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
func StageSpecialistScripts(s Specialist, task string, plan SpecialistScriptPlan, sharedDir string, emit func(string)) ([]StagedScript, error) {
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		return nil, fmt.Errorf("create shared scripts dir: %w", err)
	}

	batchID := time.Now().UnixNano()

	pluginByName := make(map[string]Plugin, len(s.Plugins))
	for _, p := range s.Plugins {
		pluginByName[normalizeScriptName(p.Path)] = p
	}

	specialistDir := filepath.Join(specialistsDir, specialistDirName(s.Name))
	staged := make([]StagedScript, 0, len(plan.Scripts))

	for i, call := range plan.Scripts {
		fields := strings.Fields(strings.TrimSpace(call))
		if len(fields) == 0 {
			continue
		}

		scriptName := normalizeScriptName(fields[0])
		plugin, ok := pluginByName[scriptName]
		if !ok {
			return nil, fmt.Errorf("script %q is not configured for specialist %q", scriptName, s.Name)
		}
		relPath := plugin.Path

		sourcePath := filepath.Join(specialistDir, relPath)
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read source script %s: %w", sourcePath, err)
		}

		argValues, err := mapScriptArgs(plugin, fields[1:])
		if err != nil {
			return nil, fmt.Errorf("parse args for %q: %w", call, err)
		}
		argValues["task_input"] = task

		content, err := replaceTokensWithSanitizedLiterals(string(source), argValues)
		if err != nil {
			return nil, fmt.Errorf("token conversion failed for %q: %w", call, err)
		}

		stagedFile := fmt.Sprintf("%s-%s-%d-%d.py", specialistDirName(s.Name), scriptName, batchID, i+1)

		// Inject SCRIPT_ID so each script can include its own filename in emitted JSON,
		// allowing the orchestrator to match sandbox output back to the staged script.
		content = strings.ReplaceAll(content, `"__SCRIPT_ID__"`, fmt.Sprintf(`"%s"`, stagedFile))
		content = strings.ReplaceAll(content, `'__SCRIPT_ID__'`, fmt.Sprintf(`'%s'`, stagedFile))
		content = strings.ReplaceAll(content, `__SCRIPT_ID__`, stagedFile)

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
