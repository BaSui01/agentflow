package llm

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yourusername/agentflow/llm/circuitbreaker"
	"github.com/yourusername/agentflow/llm/idempotency"
	"github.com/yourusername/agentflow/llm/retry"
	"go.uber.org/zap"
)

// ResilientProvider 具有弹性能力的 Provider 包装器
// 提供重试、幂等和熔断功能
// 遵循装饰器模式：增强原有 Provider 而不修改其代码
type ResilientProvider struct {
	provider          Provider                      // 底层 Provider
	retryer           retry.Retryer                 // 重试器
	idempotency       idempotency.Manager           // 幂等性管理器
	circuitBreaker    circuitbreaker.CircuitBreaker // 熔断器
	logger            *zap.Logger
	enableIdempotency bool          // 是否启用幂等性
	idempotencyTTL    time.Duration // 幂等键缓存时间
}

// ResilientProviderConfig 弹性 Provider 配置
type ResilientProviderConfig struct {
	// EnableRetry 是否启用重试
	EnableRetry bool
	// RetryPolicy 重试策略
	RetryPolicy *retry.RetryPolicy

	// EnableIdempotency 是否启用幂等性
	EnableIdempotency bool
	// IdempotencyTTL 幂等键缓存时间
	IdempotencyTTL time.Duration

	// EnableCircuitBreaker 是否启用熔断器
	EnableCircuitBreaker bool
	// CircuitBreakerConfig 熔断器配置
	CircuitBreakerConfig *circuitbreaker.Config
}

// DefaultResilientProviderConfig 返回默认配置
func DefaultResilientProviderConfig() *ResilientProviderConfig {
	return &ResilientProviderConfig{
		EnableRetry:          true,
		RetryPolicy:          retry.DefaultRetryPolicy(),
		EnableIdempotency:    true,
		IdempotencyTTL:       1 * time.Hour,
		EnableCircuitBreaker: true,
		CircuitBreakerConfig: circuitbreaker.DefaultConfig(),
	}
}

// NewResilientProvider 创建具有弹性能力的 Provider
func NewResilientProvider(
	provider Provider,
	retryer retry.Retryer,
	idempotencyMgr idempotency.Manager,
	breaker circuitbreaker.CircuitBreaker,
	config *ResilientProviderConfig,
	logger *zap.Logger,
) *ResilientProvider {
	if config == nil {
		config = DefaultResilientProviderConfig()
	}

	return &ResilientProvider{
		provider:          provider,
		retryer:           retryer,
		idempotency:       idempotencyMgr,
		circuitBreaker:    breaker,
		logger:            logger,
		enableIdempotency: config.EnableIdempotency,
		idempotencyTTL:    config.IdempotencyTTL,
	}
}

// Completion 实现 Provider.Completion
// 集成重试、幂等和熔断能力
func (rp *ResilientProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 生成幂等键
	idempotencyKey := ""
	if rp.enableIdempotency && rp.idempotency != nil {
		key, err := rp.generateIdempotencyKey(req)
		if err != nil {
			rp.logger.Warn("生成幂等键失败，跳过幂等性检查",
				zap.Error(err),
			)
		} else {
			idempotencyKey = key

			// 检查是否有缓存结果
			if cached, found, err := rp.idempotency.Get(ctx, idempotencyKey); err == nil && found {
				rp.logger.Debug("幂等键命中，返回缓存结果",
					zap.String("key", idempotencyKey),
				)

				var resp ChatResponse
				if err := json.Unmarshal(cached, &resp); err == nil {
					return &resp, nil
				}
			}
		}
	}

	// 执行调用（带重试和熔断）
	var resp *ChatResponse
	var err error

	// 熔断器包装
	callFn := func() error {
		resp, err = rp.provider.Completion(ctx, req)
		return err
	}

	if rp.circuitBreaker != nil {
		err = rp.circuitBreaker.Call(ctx, callFn)
	} else if rp.retryer != nil {
		err = rp.retryer.Do(ctx, callFn)
	} else {
		err = callFn()
	}

	if err != nil {
		return nil, err
	}

	// 缓存结果（幂等性）
	if rp.enableIdempotency && idempotencyKey != "" && rp.idempotency != nil {
		if cacheErr := rp.idempotency.Set(ctx, idempotencyKey, resp, rp.idempotencyTTL); cacheErr != nil {
			rp.logger.Warn("缓存幂等结果失败",
				zap.String("key", idempotencyKey),
				zap.Error(cacheErr),
			)
		}
	}

	return resp, nil
}

// Stream 实现 Provider.Stream
// 注意：流式调用不启用幂等性（因为无法缓存 SSE 流）
func (rp *ResilientProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	// 流式调用不启用重试和幂等性
	// 仅使用熔断器保护
	if rp.circuitBreaker != nil {
		// 检查熔断器状态
		if rp.circuitBreaker.State() == circuitbreaker.StateOpen {
			return nil, circuitbreaker.ErrCircuitOpen
		}
	}

	// 直接调用底层 Provider
	return rp.provider.Stream(ctx, req)
}

func (rp *ResilientProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return rp.provider.HealthCheck(ctx)
}

// Name 实现 Provider.Name
func (rp *ResilientProvider) Name() string {
	return rp.provider.Name()
}

// SupportsNativeFunctionCalling 实现 Provider.SupportsNativeFunctionCalling
// 委托给底层 Provider
func (rp *ResilientProvider) SupportsNativeFunctionCalling() bool {
	return rp.provider.SupportsNativeFunctionCalling()
}

// generateIdempotencyKey 生成幂等键
// 基于请求的核心参数（排除非确定性参数如 temperature、top_p）
func (rp *ResilientProvider) generateIdempotencyKey(req *ChatRequest) (string, error) {
	// 提取确定性参数
	deterministicReq := struct {
		Model    string       `json:"model"`
		Messages []Message    `json:"messages"`
		Tools    []ToolSchema `json:"tools,omitempty"`
	}{
		Model:    req.Model,
		Messages: req.Messages,
		Tools:    req.Tools,
	}

	return rp.idempotency.GenerateKey(deterministicReq)
}

// WrapProviderWithResilience 便捷函数：为 Provider 添加弹性能力
// 使用默认配置创建 ResilientProvider
func WrapProviderWithResilience(
	provider Provider,
	retryer retry.Retryer,
	idempotencyMgr idempotency.Manager,
	breaker circuitbreaker.CircuitBreaker,
	logger *zap.Logger,
) Provider {
	return NewResilientProvider(
		provider,
		retryer,
		idempotencyMgr,
		breaker,
		DefaultResilientProviderConfig(),
		logger,
	)
}

// NewResilientProviderSimple 简化版构造函数
// 自动创建重试器、幂等性管理器和熔断器
func NewResilientProviderSimple(
	provider Provider,
	idempotencyMgr idempotency.Manager,
	logger *zap.Logger,
) Provider {
	config := DefaultResilientProviderConfig()

	// 创建重试器
	retryer := retry.NewBackoffRetryer(config.RetryPolicy, logger)

	// 创建熔断器
	breaker := circuitbreaker.NewCircuitBreaker(config.CircuitBreakerConfig, logger)

	return NewResilientProvider(
		provider,
		retryer,
		idempotencyMgr,
		breaker,
		config,
		logger,
	)
}
