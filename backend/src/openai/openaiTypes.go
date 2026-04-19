package openai

import "net/http"

// Shared message type — mirrors ollama.OllamaMessage so agents don't change.
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaChatRequest mirrors the ollama package's type so call sites compile unchanged.
type OllamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream,omitempty"`
}

// OllamaStreamLine is the shape the frontend expects (Ollama NDJSON format).
type OllamaStreamLine struct {
	Message OllamaMessage `json:"message"`
	Done    bool          `json:"done"`
}

// --- OpenAI wire types ---

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// Non-streaming response
type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
}

// Streaming SSE chunk
type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// OpenAI models list response
type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// OllamaModelsResponse mirrors the ollama package's type for server.go compatibility.
type OllamaModelsResponse struct {
	Models []OllamaModel `json:"models"`
}

type OllamaModel struct {
	Name string `json:"name"`
}

type OpenAIService struct {
	apiKey string
	client *http.Client
}
