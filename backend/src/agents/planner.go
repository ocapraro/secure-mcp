package agents

import (
	"fmt"
	"smcp/ollama"
)

func CallPlanner() *Agent {
	return &Agent{
		Messages: []ollama.OllamaMessage{
			{
				Role:    "system",
				Content: fmt.Sprintf("You are a planner. You will be given requests in the format: %s. You must evaluate the request, and then break it into concrete actionable tasks. Your response MUST follow the format: \n```json\n{\"reasoning\":string,\"tasks\":string[]}\n```\n", requestFormat),
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
		},
		Model: HEAVY_MODEL,
	}
}
