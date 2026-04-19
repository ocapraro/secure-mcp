package agents

import (
	"context"
	"fmt"
	"smcp/ollama"
)

type Agent struct {
	Messages []ollama.OllamaMessage
	Model    string
}

func (a *Agent) Chat(message string, ollamaService *ollama.OllamaService, ctx context.Context) (string, error) {
	messages := append(a.Messages,
		ollama.OllamaMessage{
			Role:    "user",
			Content: fmt.Sprintf("{request:\"%s\"}", message),
		},
	)

	payload := ollama.OllamaChatRequest{
		Model:    a.Model,
		Messages: messages,
		Stream:   false,
	}

	return ollamaService.SendChatPatiently(ctx, payload)
}
