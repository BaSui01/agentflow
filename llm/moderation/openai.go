package moderation

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAIProvider 使用 OpenAI API 执行内容审核.
type OpenAIProvider struct {
	*providers.BaseCapabilityProvider
}

// NewOpenAIProvider 创建新的 OpenAI 内容审核提供者.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "omni-moderation-latest"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &OpenAIProvider{
		BaseCapabilityProvider: providers.NewBaseCapabilityProvider(providers.CapabilityConfig{
			Name:    "openai-moderation",
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			Timeout: timeout,
		}),
	}
}

func (p *OpenAIProvider) Name() string { return p.ProviderName }

type openAIModerationRequest struct {
	Model string `json:"model,omitempty"`
	Input any    `json:"input"`
}

type openAIModerationResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Flagged        bool               `json:"flagged"`
		Categories     map[string]bool    `json:"categories"`
		CategoryScores map[string]float64 `json:"category_scores"`
	} `json:"results"`
}
