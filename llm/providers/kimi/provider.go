package kimi

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"go.uber.org/zap"
)

// Kimi Provider 执行月球射击 Kimi Profer.
// Kimi使用OpenAI相容的API格式.
type KimiProvider struct {
	*openai.OpenAIProvider
	cfg providers.KimiConfig
}

// NewKimi Provider创建了一个新的 Kimi 提供者实例 。
func NewKimiProvider(cfg providers.KimiConfig, logger *zap.Logger) *KimiProvider {
	// 如果未提供则设置默认 BaseURL
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.moonshot.cn"
	}

	// 转换为 OpenAI 配置
	openaiCfg := providers.OpenAIConfig{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Timeout: cfg.Timeout,
	}

	return &KimiProvider{
		OpenAIProvider: openai.NewOpenAIProvider(openaiCfg, logger),
		cfg:            cfg,
	}
}

func (p *KimiProvider) Name() string { return "kimi" }

func (p *KimiProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	// 重新使用 OpenAI 健康检查逻辑
	status, err := p.OpenAIProvider.HealthCheck(ctx)
	if err != nil {
		return &llm.HealthStatus{
			Healthy: false,
			Latency: time.Since(start),
		}, fmt.Errorf("kimi health check failed: %w", err)
	}
	return status, nil
}

func (p *KimiProvider) SupportsNativeFunctionCalling() bool { return true }

// 完成超过 OpenAI 的补全来修正提供方字段 。
func (p *KimiProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
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

// Cream 覆盖 OpenAI 的 Stream 来修正每个块上的提供方字段 。
func (p *KimiProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
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
