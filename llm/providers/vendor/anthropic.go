package vendor

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	claude "github.com/BaSui01/agentflow/llm/providers/anthropic"
	"go.uber.org/zap"
)

type AnthropicConfig struct {
	APIKey    string
	BaseURL   string
	ChatModel string
	Timeout   time.Duration
}

func NewAnthropicProfile(cfg AnthropicConfig, logger *zap.Logger) *Profile {
	chat := claude.NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.ChatModel,
			Timeout: cfg.Timeout,
		},
	}, logger)

	return &Profile{
		Name: "anthropic",
		Chat: chat,
		LanguageModels: map[string]string{
			"default": cfg.ChatModel,
			"zh":      cfg.ChatModel,
			"en":      cfg.ChatModel,
		},
	}
}
