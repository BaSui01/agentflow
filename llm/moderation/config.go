package moderation

import "time"

// OpenAIConfig 配置 OpenAI 节奏提供者 。
type OpenAIConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 OpenAIConfig 返回默认 OpenAI 温和配置 。
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseURL: "https://api.openai.com/v1",
		Model:   "omni-moderation-latest",
		Timeout: 30 * time.Second,
	}
}
