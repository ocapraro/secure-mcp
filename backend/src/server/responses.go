package server

import "smcp/database"

type HealthResponse struct {
	Status string `json:"status"`
}

type ModelsResponse struct {
	Models []string `json:"models"`
}

type ChatRequest struct {
	Model    string             `json:"model"`
	Messages []database.Message `json:"messages"`
	Stream   bool               `json:"stream,omitempty"`
}

// ========= Ollama Responses ===========
type OllamaModelsResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Name       string             `json:"name"`
	Model      string             `json:"model"`
	ModifiedAt string             `json:"modified_at"`
	Size       int64              `json:"size"`
	Digest     string             `json:"digest"`
	Details    OllamaModelDetails `json:"details"`
}

type OllamaModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
