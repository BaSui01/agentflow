package providers

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// RetryConfig 持有提供者包装器的再试配置 。
type RetryConfig struct {
	MaxRetries    int           `json:"max_retries"`    // Maximum retry attempts, default 3
	InitialDelay  time.Duration `json:"initial_delay"`  // Initial backoff delay, default 1s
	MaxDelay      time.Duration `json:"max_delay"`      // Maximum backoff delay, default 30s
	BackoffFactor float64       `json:"backoff_factor"` // Exponential backoff factor, default 2.0
	RetryableOnly bool          `json:"retryable_only"` // Only retry errors marked Retryable
}

// 默认 RetriConfig 返回明智的重试默认 。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		RetryableOnly: true,
	}
}

// 可重试 Provider 包了 illm 。 提供指数后退的重试逻辑.
type RetryableProvider struct {
	inner  llm.Provider
	config RetryConfig
	logger *zap.Logger
}

// New Retryable Provider在给定的提供者周围创建了重试包.
func NewRetryableProvider(inner llm.Provider, config RetryConfig, logger *zap.Logger) *RetryableProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetryableProvider{
		inner:  inner,
		config: config,
		logger: logger.With(zap.String("component", "retry_provider"), zap.String("provider", inner.Name())),
	}
}

// 编译时间界面检查.
var _ llm.Provider = (*RetryableProvider)(nil)

func (p *RetryableProvider) Name() string                        { return p.inner.Name() }
func (p *RetryableProvider) SupportsNativeFunctionCalling() bool { return p.inner.SupportsNativeFunctionCalling() }
func (p *RetryableProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return p.inner.ListModels(ctx)
}
func (p *RetryableProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return p.inner.HealthCheck(ctx)
}

// Completion 执行聊天补全，在瞬态错误上重试.
func (p *RetryableProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := p.calculateDelay(attempt)
			p.logger.Debug("retrying completion",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := p.inner.Completion(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// 不可重复的错误立即返回 。
		if p.config.RetryableOnly {
			if llmErr, ok := err.(*llm.Error); ok && !llmErr.Retryable {
				return nil, err
			}
		}

		p.logger.Warn("completion failed, will retry",
			zap.Int("attempt", attempt),
			zap.Error(err))
	}

	return nil, fmt.Errorf("completion failed after %d retries: %w", p.config.MaxRetries, lastErr)
}

// Stream 执行流式聊天请求，并重试连接错误.
// 只有连接建立阶段会被重试；流传输中的错误不会.
func (p *RetryableProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := p.calculateDelay(attempt)
			p.logger.Debug("retrying stream",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := p.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}

		lastErr = err

		if p.config.RetryableOnly {
			if llmErr, ok := err.(*llm.Error); ok && !llmErr.Retryable {
				return nil, err
			}
		}

		p.logger.Warn("stream connection failed, will retry",
			zap.Int("attempt", attempt),
			zap.Error(err))
	}

	return nil, fmt.Errorf("stream failed after %d retries: %w", p.config.MaxRetries, lastErr)
}

func (p *RetryableProvider) calculateDelay(attempt int) time.Duration {
	delay := float64(p.config.InitialDelay) * math.Pow(p.config.BackoffFactor, float64(attempt-1))
	if delay > float64(p.config.MaxDelay) {
		delay = float64(p.config.MaxDelay)
	}
	return time.Duration(delay)
}
