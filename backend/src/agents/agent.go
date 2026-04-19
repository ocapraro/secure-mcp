package agents

import (
	"context"
	"fmt"
	"smcp/openai"
)

type Agent struct {
	Messages []openai.OllamaMessage
	Model    string
}

func (a *Agent) Chat(message string, openaiService *openai.OpenAIService, ctx context.Context) (string, error) {
	messages := append(a.Messages,
		openai.OllamaMessage{
			Role:    "user",
			Content: fmt.Sprintf("{request:\"%s\"}", message),
		},
	)

	payload := openai.OllamaChatRequest{
		Model:    a.Model,
		Messages: messages,
		Stream:   false,
	}

	return openaiService.SendChatPatiently(ctx, payload)
}
