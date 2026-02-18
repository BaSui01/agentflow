package providers

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// RetryConfig holds retry configuration for a provider wrapper.
type RetryConfig struct {
	MaxRetries    int           `json:"max_retries"`    // Maximum retry attempts, default 3
	InitialDelay  time.Duration `json:"initial_delay"`  // Initial backoff delay, default 1s
	MaxDelay      time.Duration `json:"max_delay"`      // Maximum backoff delay, default 30s
	BackoffFactor float64       `json:"backoff_factor"` // Exponential backoff factor, default 2.0
	RetryableOnly bool          `json:"retryable_only"` // Only retry errors marked Retryable
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		RetryableOnly: true,
	}
}

// RetryableProvider wraps an llm.Provider with exponential-backoff retry logic.
type RetryableProvider struct {
	inner  llm.Provider
	config RetryConfig
	logger *zap.Logger
}

// NewRetryableProvider creates a retrying wrapper around the given provider.
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

// Compile-time interface check.
var _ llm.Provider = (*RetryableProvider)(nil)

func (p *RetryableProvider) Name() string                        { return p.inner.Name() }
func (p *RetryableProvider) SupportsNativeFunctionCalling() bool { return p.inner.SupportsNativeFunctionCalling() }
func (p *RetryableProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return p.inner.HealthCheck(ctx)
}

// Completion performs a chat completion with retry on transient errors.
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

		// Non-retryable errors are returned immediately.
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

// Stream performs a streaming chat request with retry on connection errors.
// Only the connection-establishment phase is retried; mid-stream errors are not.
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
