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

// MistralProvider 实现 Mistral AI LLM 提供者.
// Mistral AI 使用 OpenAI 兼容的 API 格式.
type MistralProvider struct {
	*openai.OpenAIProvider
	cfg providers.MistralConfig
}

// NewMistralProvider 创建新的 Mistral 提供者实例.
func NewMistralProvider(cfg providers.MistralConfig, logger *zap.Logger) *MistralProvider {
	// 如果未提供则设置默认 BaseURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.mistral.ai"
	}

	// 转换为 OpenAI 配置
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
	// 重新使用 OpenAI 健康检查逻辑
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

// Completion 覆盖 OpenAI 的补全以修正提供者字段.
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

// Stream 覆盖 OpenAI 的 Stream 以修正每个块上的提供者字段.
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
