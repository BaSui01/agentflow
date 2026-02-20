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

// 匈奴提供商执行"特能匈奴提供商".
// Hunyuan使用OpenAI相容的API格式.
type HunyuanProvider struct {
	*openai.OpenAIProvider
	cfg providers.HunyuanConfig
}

// NewHunyuan Provider创建了新的Hunyuan供应商实例.
func NewHunyuanProvider(cfg providers.HunyuanConfig, logger *zap.Logger) *HunyuanProvider {
	// 如果未提供则设置默认 BaseURL
	// Hunyuan OpenAI相容的API:https://api.hunyuan.cloud.tencent.com/v1 互联网档案馆的存檔,存档日期2013-12-11.
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.hunyuan.cloud.tencent.com/v1"
	}

	// 转换为 OpenAI 配置
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
	// 重新使用 OpenAI 健康检查逻辑
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

// 完成超过 OpenAI 的补全来修正提供方字段 。
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

// Cream 覆盖 OpenAI 的 Stream 来修正每个块上的提供方字段 。
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
