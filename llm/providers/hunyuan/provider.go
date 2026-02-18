package hunyuan

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

// HunyuanProvider implements Tencent Hunyuan Provider.
// Hunyuan uses OpenAI-compatible API format.
type HunyuanProvider struct {
	*openai.OpenAIProvider
	cfg providers.HunyuanConfig
}

// NewHunyuanProvider creates a new Hunyuan provider instance.
func NewHunyuanProvider(cfg providers.HunyuanConfig, logger *zap.Logger) *HunyuanProvider {
	// Set default BaseURL if not provided
	// Hunyuan OpenAI-compatible API: https://api.hunyuan.cloud.tencent.com/v1
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.hunyuan.cloud.tencent.com/v1"
	}

	// Convert to OpenAI config
	openaiCfg := providers.OpenAIConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	return &HunyuanProvider{
		OpenAIProvider: openai.NewOpenAIProvider(openaiCfg, logger),
		cfg:            cfg,
	}
}

func (p *HunyuanProvider) Name() string { return "hunyuan" }

func (p *HunyuanProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	// Reuse OpenAI health check logic
	status, err := p.OpenAIProvider.HealthCheck(ctx)
	if err != nil {
		return &llm.HealthStatus{
			Healthy: false,
			Latency: time.Since(start),
		}, fmt.Errorf("hunyuan health check failed: %w", err)
	}
	return status, nil
}

func (p *HunyuanProvider) SupportsNativeFunctionCalling() bool { return true }

// Completion overrides OpenAI's Completion to fix the Provider field.
func (p *HunyuanProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
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
func (p *HunyuanProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
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
