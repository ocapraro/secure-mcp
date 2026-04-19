package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"smcp/openai"
	"strings"
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

func fallbackScriptCall(allowed map[string]struct{}, task string) string {
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

// runDockerScript executes a specialist's plugin via its Dockerfile image.
// scriptCall is the script filename (no path) plus arguments, e.g. "get-weather Boston MA".
func runDockerScript(specialistDir, scriptCall string) (string, error) {
	parts := strings.SplitN(scriptCall, " ", 2)
	scriptName := strings.TrimSuffix(parts[0], ".py")
	location := ""
	if len(parts) > 1 {
		location = parts[1]
	}

	dirParts := strings.Split(specialistDir, "/")
	imageName := strings.ToLower(dirParts[len(dirParts)-1])

	buildCmd := exec.Command("docker", "build", "-t", imageName, specialistDir)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("docker build failed: %v\n%s", err, out)
	}

	runArgs := []string{"run", "--rm", "-e", fmt.Sprintf("SCRIPT=%s", scriptName)}
	if location != "" {
		runArgs = append(runArgs, "-e", fmt.Sprintf("LOCATION=%s", location))
	}
	runArgs = append(runArgs, imageName)

	var stdout, stderr bytes.Buffer
	runCmd := exec.Command("docker", runArgs...)
	runCmd.Stdout = &stdout
	runCmd.Stderr = &stderr
	if err := runCmd.Run(); err != nil {
		return "", fmt.Errorf("docker run failed: %v\n%s", err, stderr.String())
	}
	return stdout.String(), nil
}

// RunSpecialistTask runs the specialist agent for a single task, executing
// selected scripts via Docker, and returns raw script outputs.
func RunSpecialistTask(s Specialist, task string, openaiService *openai.OpenAIService, ctx context.Context) (string, error) {
	agent := CallSpecialist(s)

	specialistDirName := strings.ToLower(strings.ReplaceAll(s.Name, " ", "-"))
	specialistDir := fmt.Sprintf("../specialists/%s", specialistDirName)

	message := fmt.Sprintf("{\"task\":\"%s\"}", task)
	raw, err := agent.Chat(message, openaiService, ctx)
	if err != nil {
		return "", fmt.Errorf("specialist %s error: %w", s.Name, err)
	}
	log.Printf("[specialist-response][%s] %s", s.Name, strings.TrimSpace(raw))

	plan, err := parseScriptPlan(raw)
	if err != nil {
		return "", fmt.Errorf("specialist %s returned invalid script plan", s.Name)
	}

	allowed := make(map[string]struct{}, len(s.Plugins))
	for _, p := range s.Plugins {
		allowed[normalizeScriptName(p.Path)] = struct{}{}
	}
	if len(allowed) == 0 {
		return "", fmt.Errorf("specialist %s has no configured scripts", s.Name)
	}

	if len(plan.Scripts) == 0 {
		retryMessage := fmt.Sprintf("{\"task\":\"%s\",\"instruction\":\"Return at least one script call from your available plugins.\"}", task)
		retryRaw, retryErr := agent.Chat(retryMessage, openaiService, ctx)
		if retryErr != nil {
		} else {
			log.Printf("[specialist-response][%s] %s", s.Name, strings.TrimSpace(retryRaw))
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
		}
	}

	results := make([]ScriptExecutionResult, 0, len(plan.Scripts))
	for _, call := range plan.Scripts {
		call = strings.TrimSpace(call)
		if call == "" {
			continue
		}
		first := strings.Fields(call)
		if len(first) == 0 {
			continue
		}
		scriptName := normalizeScriptName(first[0])
		if _, ok := allowed[scriptName]; !ok {
			errMsg := fmt.Sprintf("script %q is not allowed for specialist %q", scriptName, s.Name)
			results = append(results, ScriptExecutionResult{Script: call, Error: errMsg})
			continue
		}

		output, scriptErr := runDockerScript(specialistDir, call)
		if scriptErr != nil {
			results = append(results, ScriptExecutionResult{Script: call, Error: scriptErr.Error()})
			continue
		}
		results = append(results, ScriptExecutionResult{Script: call, Output: output})
	}

	env := SpecialistExecutionEnvelope{
		Specialist: s.Name,
		Task:       task,
		Results:    results,
	}
	encoded, err := json.Marshal(env)
	if err != nil {
		return "", fmt.Errorf("failed to encode specialist execution output: %w", err)
	}

	return string(encoded), nil
}
