package llama

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

// LlamaProvider 通过第三方 OpenAI 兼容 API 实现 Meta Llama 提供者.
// 支持 Together AI、Replicate 和 OpenRouter.
type LlamaProvider struct {
	*openai.OpenAIProvider
	cfg providers.LlamaConfig
}

// NewLlamaProvider 创建新的 Llama 提供者实例.
func NewLlamaProvider(cfg providers.LlamaConfig, logger *zap.Logger) *LlamaProvider {
	// 如果未提供则设置默认提供者和 BaseURL
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

	// 转换为 OpenAI 配置
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
	// 重新使用 OpenAI 健康检查逻辑
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

// Completion 覆盖 OpenAI 的补全以修正提供者字段.
func (p *LlamaProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
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

// Stream 覆盖 OpenAI 的 Stream 以修正每个块上的提供者字段.
func (p *LlamaProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
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
