package agents

import (
	"fmt"
	"smcp/openai"
	"strings"
)

func CallGeneralist() *Agent {
	return &Agent{
		Messages: []openai.OllamaMessage{
			{
				Role:    "system",
				Content: "You are a helpful generalist assistant. Answer the user's task as concisely and accurately as possible.",
			},
		},
		Model: MEDIUM_MODEL,
	}
}

func CallDelegator() *Agent {
	specialists := ListSpecialists()
	var specialistsSection strings.Builder
	for _, specialist := range specialists {
		specialistsSection.WriteString(
			fmt.Sprintf(
				"\n- name: %s\n  resume: %s\n  example_request: %s",
				specialist.Name,
				specialist.Resume,
				specialist.ExampleRequest,
			),
		)
	}

	systemPrompt := fmt.Sprintf(
		"You are the Delegator, people come to you with a set of tasks and a list of specialists, and you figure out who needs to work on what. If the task is generic enough, or if there's no good specialist that it maps to, just assign it to the Generalist. You must evaluate each task, and decide on the correct specialist for the job, or assign it to the Generalist. You can assign multiple tasks to the same specialist. Your response MUST follow the format: `{\"reasoning\":string,assignments:{\"task\":string, \"asignee\":string}[]}`. Here are your specialists:%s",
		specialistsSection.String(),
	)

	return &Agent{
		Messages: []openai.OllamaMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		},
		Model: HEAVY_MODEL,
	}
}
