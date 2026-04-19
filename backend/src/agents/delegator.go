package agents

import (
	"fmt"
	"smcp/ollama"
	"strings"
)

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
		"You are the Delegator, people come to you with a task and a list of specialists, and you figure out who needs to work on what. If the task is generic enough, or if there's no good specialist that it maps to, just assign it to the Generalist. You will be given taks in the format: %sYou must evaluate the task, and decide on the correct specialist for the job, or assign it to the Generalist. Your response MUST follow the format: `{\"reasoning\":string,\"task\":string, \"asignee\":string}`. Here are your specialists:%s",
		taskFormat,
		specialistsSection.String(),
	)

	return &Agent{
		Messages: []ollama.OllamaMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		},
		Model: HEAVY_MODEL,
	}
}
