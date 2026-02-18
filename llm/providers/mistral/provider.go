package mistral

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

// MistralProvider implements Mistral AI Provider.
// Mistral AI uses OpenAI-compatible API format.
type MistralProvider struct {
	*openai.OpenAIProvider
	cfg providers.MistralConfig
}

// NewMistralProvider creates a new Mistral provider instance.
func NewMistralProvider(cfg providers.MistralConfig, logger *zap.Logger) *MistralProvider {
	// Set default BaseURL if not provided
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.mistral.ai"
	}

	// Convert to OpenAI config
	openaiCfg := providers.OpenAIConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	return &MistralProvider{
		OpenAIProvider: openai.NewOpenAIProvider(openaiCfg, logger),
		cfg:            cfg,
	}
}

func (p *MistralProvider) Name() string { return "mistral" }

func (p *MistralProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	// Reuse OpenAI health check logic
	status, err := p.OpenAIProvider.HealthCheck(ctx)
	if err != nil {
		return &llm.HealthStatus{
			Healthy: false,
			Latency: time.Since(start),
		}, fmt.Errorf("mistral health check failed: %w", err)
	}
	return status, nil
}

func (p *MistralProvider) SupportsNativeFunctionCalling() bool { return true }

// Completion overrides OpenAI's Completion to fix the Provider field.
func (p *MistralProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	resp, err := p.OpenAIProvider.Completion(ctx, req)
	if err != nil {
		if llmErr, ok := err.(*llm.Error); ok {
			llmErr.Provider = p.Name()
			return nil, llmErr
		}
		return nil, err
	}
	resp.Provider = p.Name()
	return resp, nil
}

// Stream overrides OpenAI's Stream to fix the Provider field on each chunk.
func (p *MistralProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch, err := p.OpenAIProvider.Stream(ctx, req)
	if err != nil {
		if llmErr, ok := err.(*llm.Error); ok {
			llmErr.Provider = p.Name()
			return nil, llmErr
		}
		return nil, err
	}

	wrappedCh := make(chan llm.StreamChunk)
	go func() {
		defer close(wrappedCh)
		for chunk := range ch {
			chunk.Provider = p.Name()
			if chunk.Err != nil {
				chunk.Err.Provider = p.Name()
			}
			wrappedCh <- chunk
		}
	}()
	return wrappedCh, nil
}
