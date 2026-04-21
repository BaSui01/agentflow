package usecase

type UpsertToolProviderInput struct {
	APIKey         string
	BaseURL        string
	TimeoutSeconds int
	Priority       int
	Enabled        *bool
}

type ToolProviderView struct {
	ID             uint   `json:"id"`
	Provider       string `json:"provider"`
	BaseURL        string `json:"base_url,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Priority       int    `json:"priority"`
	Enabled        bool   `json:"enabled"`
	HasAPIKey      bool   `json:"has_api_key"`
}
