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

func CallPlanner() *Agent {
	return &Agent{
		Messages: []openai.OllamaMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("You are a planner. You will be given requests in the format: %s. You must evaluate the request, and then break it into concrete actionable tasks. Your response MUST follow the format: `{\"reasoning\":string,\"tasks\":string[]}`", requestFormat),
			},
			{
				Role:    "user",
				Content: "{\"request\":\"How much does an elephant weigh?\"}",
			},
			{
				Role:    "assistant",
				Content: "{\"reasoning\":\"The request is informational, so I should break it into steps to identify typical elephant species and provide realistic weight ranges with context.\",\"tasks\":[\"Identify the most common elephant species relevant to the question (African bush, African forest, Asian)\",\"Gather typical adult weight ranges for each species from reliable references\",\"Summarize the ranges in a concise human-friendly answer\",\"Add brief context that weight varies by sex and age\",\"Present a final direct answer with a practical range in metric and imperial units\"]}",
			},
			{
				Role:    "user",
				Content: "{\"request\":\"I want to take a trip to japan this year, when should I go?\"}",
			},
			{
				Role:    "assistant",
				Content: "{\"reasoning\":\"Choosing when to visit Japan depends on preferences (weather, crowds, budget, activities), assuming they want nice weather, I should check for temperature throughout the year. They could also be looking for cheap flights which I should keep in mind.\",\"tasks\":[\"Clarify traveler priorities (weather, budget, crowd tolerance, and preferred activities)\",\"Compare Japan by season for temperature, rainfall, and humidity\",\"Identify peak and off-peak travel windows and expected pricing differences\",\"Match top travel months to the user's priorities with trade-offs\",\"Provide a recommended time window and booking timeline for flights and hotels\"]}",
			},
			{
				Role:    "user",
				Content: "{\"request\":\"What's the weather today?\"}",
			},
			{
				Role:    "assistant",
				Content: "{\"reasoning\":\"This is a simple, straightforward request that can be answered directly with a single lookup.\",\"tasks\":[\"Get current weather data\"]}",
			},
		},
		Model: HEAVY_MODEL,
	}
}
