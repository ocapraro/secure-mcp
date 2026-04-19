package agents

type PlannerResponse struct {
	Reasoning string   `json:"reasoning"`
	Tasks     []string `json:"tasks"`
}

type Assignment struct {
	Task     string `json:"task"`
	Assignee string `json:"asignee"`
}

type DelegatorResponse struct {
	Reasoning   string       `json:"reasoning"`
	Assignments []Assignment `json:"assignments"`
}
