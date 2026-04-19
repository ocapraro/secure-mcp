package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type OllamaService struct {
	baseURL string
	client  *http.Client
}

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

func (s *OllamaService) GetHealth(ctx context.Context) HealthResponse {
	resp, err := s.get("", ctx)
	if err != nil || resp.StatusCode != 200 {
		return HealthResponse{
			Status: "not ok",
		}
	}
	defer resp.Body.Close()

	return HealthResponse{
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
