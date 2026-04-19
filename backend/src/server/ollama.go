package server

import (
	"context"
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

func (s *OllamaService) GetHealth(ctx context.Context) HealthResponse {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL, nil)
	if err != nil {
		return HealthResponse{
			Status: "not ok",
		}
	}
	resp, err := s.client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return HealthResponse{
			Status: "not ok",
		}
	}
	defer resp.Body.Close()

	// var result HealthResponse
	// if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
	// 	panic(err)
	// }

	return HealthResponse{
		Status: "ok",
	}
}
