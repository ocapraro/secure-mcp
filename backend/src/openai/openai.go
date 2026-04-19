package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"smcp/types"
	"strings"
)

const baseURL = "https://api.openai.com/v1"

func NewOpenAIService(apiKey string, client *http.Client) *OpenAIService {
	return &OpenAIService{
		apiKey: apiKey,
		client: client,
	}
}

func (s *OpenAIService) authHeader(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
}

func (s *OpenAIService) GetHealth(ctx context.Context) types.HealthResponse {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return types.HealthResponse{Status: "not ok"}
	}
	s.authHeader(req)
	resp, err := s.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return types.HealthResponse{Status: "not ok"}
	}
	defer resp.Body.Close()
	return types.HealthResponse{Status: "ok"}
}

func (s *OpenAIService) GetModels(ctx context.Context) (OllamaModelsResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return OllamaModelsResponse{}, err
	}
	s.authHeader(req)

	resp, err := s.client.Do(req)
	if err != nil {
		return OllamaModelsResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return OllamaModelsResponse{}, fmt.Errorf("openai returned status %d", resp.StatusCode)
	}

	var raw openAIModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return OllamaModelsResponse{}, err
	}

	var result OllamaModelsResponse
	for _, m := range raw.Data {
		// Only expose GPT chat models to keep the list manageable
		if strings.HasPrefix(m.ID, "gpt-") {
			result.Models = append(result.Models, OllamaModel{Name: m.ID})
		}
	}
	return result, nil
}

// SendChat calls OpenAI with streaming enabled and returns an *http.Response whose body
// is translated from OpenAI SSE to Ollama-compatible NDJSON so the frontend needs no changes.
func (s *OpenAIService) SendChat(ctx context.Context, req OllamaChatRequest) (*http.Response, error) {
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	msgs := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openAIMessage{Role: m.Role, Content: m.Content}
	}

	payload := openAIChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	s.authHeader(httpReq)

	upstream, err := s.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if upstream.StatusCode != http.StatusOK {
		return upstream, nil
	}

	// Translate OpenAI SSE → Ollama NDJSON in a pipe so the caller can stream bytes
	pr, pw := io.Pipe()
	go func() {
		defer upstream.Body.Close()
		defer pw.Close()

		scanner := bufio.NewScanner(upstream.Body)
		enc := json.NewEncoder(pw)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				// Emit final done token
				_ = enc.Encode(OllamaStreamLine{Done: true})
				return
			}
			var chunk openAIStreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			content := chunk.Choices[0].Delta.Content
			_ = enc.Encode(OllamaStreamLine{
				Message: OllamaMessage{Role: "assistant", Content: content},
				Done:    false,
			})
		}
	}()

	// Return a synthetic *http.Response wrapping the translated pipe
	translated := &http.Response{
		StatusCode: http.StatusOK,
		Body:       pr,
		Header:     make(http.Header),
	}
	translated.Header.Set("Content-Type", "application/x-ndjson")
	return translated, nil
}

// SendChatPatiently calls OpenAI without streaming and returns the complete response string.
func (s *OpenAIService) SendChatPatiently(ctx context.Context, req OllamaChatRequest) (string, error) {
	if req.Model == "" {
		return "", fmt.Errorf("model is required")
	}

	msgs := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openAIMessage{Role: m.Role, Content: m.Content}
	}

	payload := openAIChatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	s.authHeader(httpReq)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai returned status %d: %s", resp.StatusCode, string(b))
	}

	var result openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("openai returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}
