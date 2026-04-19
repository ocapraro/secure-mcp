package types

type HealthResponse struct {
	Status string `json:"status"`
}

type ModelsResponse struct {
	Models []string `json:"models"`
}
