package moderation

import "time"

// OpenAIConfig configures the OpenAI moderation provider.
type OpenAIConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultOpenAIConfig returns default OpenAI moderation config.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL: "https://api.openai.com/v1",
		Model:   "omni-moderation-latest",
		Timeout: 30 * time.Second,
	}
}
