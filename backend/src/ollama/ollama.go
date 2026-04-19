package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"smcp/types"
	"strings"
)

func NewOllamaService(baseURL string, client *http.Client) *OllamaService {
	return &OllamaService{
		baseURL: baseURL,
		client:  client,
	}
}

// get performs a get request to the desired endpoint
func (s *OllamaService) get(endpoint string, ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req)
}

func (s *OllamaService) GetHealth(ctx context.Context) types.HealthResponse {
	resp, err := s.get("", ctx)
	if err != nil || resp.StatusCode != 200 {
		return types.HealthResponse{
			Status: "not ok",
		}
	}
	defer resp.Body.Close()

	return types.HealthResponse{
		Status: "ok",
	}
}

func (s *OllamaService) GetModels(ctx context.Context) (OllamaModelsResponse, error) {
	resp, err := s.get("/api/tags", ctx)
	if err != nil {
		return OllamaModelsResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return OllamaModelsResponse{}, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	var result OllamaModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return OllamaModelsResponse{}, err
	}

	return result, nil
}

func (s *OllamaService) SendChat(ctx context.Context, req OllamaChatRequest) (*http.Response, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Map to Ollama-compatible messages (role + content only)
	ollamaMsgs := make([]OllamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		ollamaMsgs[i] = OllamaMessage{Role: string(m.Role), Content: m.Content}
	}
	payload := map[string]any{
		"model":    req.Model,
		"messages": ollamaMsgs,
		"stream":   true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	return s.client.Do(httpReq)
}

// SendChatPatiently sends a chat request and returns the complete response string
func (s *OllamaService) SendChatPatiently(ctx context.Context, req OllamaChatRequest) (string, error) {
	resp, err := s.SendChat(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to send chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	// Parse NDJSON stream response
	var fullResponse strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		var line OllamaStreamLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // Skip malformed lines
		}
		if line.Message.Content != "" {
			fullResponse.WriteString(line.Message.Content)
		}
		if line.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading response: %w", err)
	}

	return fullResponse.String(), nil
}
