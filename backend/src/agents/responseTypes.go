package agents

type PlannerResponse struct {
	Reasoning string   `json:"reasoning"`
	Tasks     []string `json:"tasks"`
}
