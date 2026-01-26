package llama

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/providers"
	"github.com/BaSui01/agentflow/providers/openai"
	"go.uber.org/zap"
)

// LlamaProvider implements Meta Llama Provider via third-party OpenAI-compatible APIs.
// Supports Together AI, Replicate, and OpenRouter.
type LlamaProvider struct {
	*openai.OpenAIProvider
	cfg providers.LlamaConfig
}

// NewLlamaProvider creates a new Llama provider instance.
func NewLlamaProvider(cfg providers.LlamaConfig, logger *zap.Logger) *LlamaProvider {
	// Set default provider and BaseURL if not provided
	if cfg.Provider == "" {
		cfg.Provider = "together" // Default to Together AI
	}

	if cfg.BaseURL == "" {
		switch cfg.Provider {
		case "together":
			cfg.BaseURL = "https://api.together.xyz"
		case "replicate":
			cfg.BaseURL = "https://api.replicate.com"
		case "openrouter":
			cfg.BaseURL = "https://openrouter.ai/api"
		default:
			cfg.BaseURL = "https://api.together.xyz"
		}
	}

	// Convert to OpenAI config
	openaiCfg := providers.OpenAIConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	return &LlamaProvider{
		OpenAIProvider: openai.NewOpenAIProvider(openaiCfg, logger),
		cfg:            cfg,
	}
}

func (p *LlamaProvider) Name() string {
	return fmt.Sprintf("llama-%s", p.cfg.Provider)
}

func (p *LlamaProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	// Reuse OpenAI health check logic
	status, err := p.OpenAIProvider.HealthCheck(ctx)
	if err != nil {
		return &llm.HealthStatus{
			Healthy: false,
			Latency: time.Since(start),
		}, fmt.Errorf("llama health check failed: %w", err)
	}
	return status, nil
}

func (p *LlamaProvider) SupportsNativeFunctionCalling() bool { return true }
