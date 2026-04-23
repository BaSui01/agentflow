package bootstrap

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/config"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmcompose "github.com/BaSui01/agentflow/llm/runtime/compose"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// LLMHandlerRuntime groups LLM-related runtime dependencies used by API handlers.
type LLMHandlerRuntime = llmcompose.Runtime

// BuildLLMHandlerRuntime creates the LLM runtime required by handler layer.
// The main provider entry is selected by cfg.LLM.MainProviderMode.
func BuildLLMHandlerRuntime(cfg *config.Config, db *gorm.DB, logger *zap.Logger) (*LLMHandlerRuntime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required for llm handler runtime")
	}

	baseProvider, err := BuildMainProvider(context.Background(), cfg, db, logger)
	if err != nil {
		return nil, err
	}

	return BuildLLMHandlerRuntimeFromProvider(cfg, baseProvider, logger)
}

// BuildLLMHandlerRuntimeFromProvider assembles the handler-facing runtime around
// an already constructed main chat provider.
func BuildLLMHandlerRuntimeFromProvider(cfg *config.Config, mainProvider llm.Provider, logger *zap.Logger) (*LLMHandlerRuntime, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required for llm handler runtime")
	}
	return llmcompose.Build(buildComposeConfig(cfg), mainProvider, logger)
}

func buildComposeConfig(cfg *config.Config) llmcompose.Config {
	return llmcompose.Config{
		Timeout:    cfg.LLM.Timeout,
		MaxRetries: cfg.LLM.MaxRetries,
		Budget: llmcompose.BudgetConfig{
			Enabled:             cfg.Budget.Enabled,
			MaxTokensPerRequest: cfg.Budget.MaxTokensPerRequest,
			MaxTokensPerMinute:  cfg.Budget.MaxTokensPerMinute,
			MaxTokensPerHour:    cfg.Budget.MaxTokensPerHour,
			MaxTokensPerDay:     cfg.Budget.MaxTokensPerDay,
			MaxCostPerRequest:   cfg.Budget.MaxCostPerRequest,
			MaxCostPerDay:       cfg.Budget.MaxCostPerDay,
			AlertThreshold:      cfg.Budget.AlertThreshold,
			AutoThrottle:        cfg.Budget.AutoThrottle,
			ThrottleDelay:       cfg.Budget.ThrottleDelay,
		},
		Cache: llmcompose.CacheConfig{
			Enabled:      cfg.Cache.Enabled,
			LocalMaxSize: cfg.Cache.LocalMaxSize,
			LocalTTL:     cfg.Cache.LocalTTL,
			EnableRedis:  cfg.Cache.EnableRedis,
			RedisTTL:     cfg.Cache.RedisTTL,
			KeyStrategy:  cfg.Cache.KeyStrategy,
		},
		Tool: llmcompose.ToolProviderConfig{
			Provider:        cfg.LLM.ToolProvider,
			DefaultProvider: cfg.LLM.DefaultProvider,
			APIKey:          cfg.LLM.ToolAPIKey,
			DefaultAPIKey:   cfg.LLM.APIKey,
			BaseURL:         cfg.LLM.ToolBaseURL,
			DefaultBaseURL:  cfg.LLM.BaseURL,
			Timeout:         cfg.LLM.ToolTimeout,
			MaxRetries:      cfg.LLM.ToolMaxRetries,
		},
	}
}
