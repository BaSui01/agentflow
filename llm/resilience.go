package llm

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// 重试政策定义了重试行为 。
type RetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	Multiplier     float64       `json:"multiplier"`
}

// 默认重试政策返回合理默认 。
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:     3,
		InitialBackoff: 1 * time.Second,
		MaxBackoff:     30 * time.Second,
		Multiplier:     2.0,
	}
}

// 电路状态代表断路器状态.
type CircuitState int32

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreakerConfig配置断路器.
type CircuitBreakerConfig struct {
	FailureThreshold int           `json:"failure_threshold"`
	SuccessThreshold int           `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
}

// 默认 CircuitBreakerConfig 返回合理的默认值 。
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	}
}

// CircuitBreaker执行断路器模式.
type CircuitBreaker struct {
	config          *CircuitBreakerConfig
	state           atomic.Int32
	failures        atomic.Int32
	successes       atomic.Int32
	lastFailureTime atomic.Int64
	mu              sync.RWMutex
	logger          *zap.Logger
}

// 打开电路时返回 Err Circuit Open 。
var ErrCircuitOpen = errors.New("circuit breaker is open")

// 新CircuitBreaker创建了新的断路器.
func NewCircuitBreaker(config *CircuitBreakerConfig, logger *zap.Logger) *CircuitBreaker {
	if config == nil {
		config = DefaultCircuitBreakerConfig()
	}
	return &CircuitBreaker{
		config: config,
		logger: logger,
	}
}

// 状态返回当前电路状态 。
func (cb *CircuitBreaker) State() CircuitState {
	return CircuitState(cb.state.Load())
}

// 调用以断路器保护功能执行 。
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	state := cb.State()

	if state == CircuitOpen {
		if time.Now().UnixNano()-cb.lastFailureTime.Load() > cb.config.Timeout.Nanoseconds() {
			cb.state.Store(int32(CircuitHalfOpen))
			cb.successes.Store(0)
		} else {
			return ErrCircuitOpen
		}
	}

	err := fn()

	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

func (cb *CircuitBreaker) recordFailure() {
	failures := cb.failures.Add(1)
	cb.lastFailureTime.Store(time.Now().UnixNano())

	if failures >= int32(cb.config.FailureThreshold) {
		cb.state.Store(int32(CircuitOpen))
		cb.logger.Warn("circuit breaker opened", zap.Int32("failures", failures))
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	state := cb.State()
	if state == CircuitHalfOpen {
		successes := cb.successes.Add(1)
		if successes >= int32(cb.config.SuccessThreshold) {
			cb.state.Store(int32(CircuitClosed))
			cb.failures.Store(0)
			cb.logger.Info("circuit breaker closed")
		}
	} else {
		cb.failures.Store(0)
	}
}

// 耐活性 Provider用重试,断路器和一能来包裹一个提供者.
type ResilientProvider struct {
	provider       Provider
	retryPolicy    *RetryPolicy
	circuitBreaker *CircuitBreaker
	idempotencyTTL time.Duration
	idempotencyMap sync.Map
	logger         *zap.Logger
}

// 具有弹性的Config配置有弹性的提供者.
type ResilientConfig struct {
	RetryPolicy       *RetryPolicy
	CircuitBreaker    *CircuitBreakerConfig
	EnableIdempotency bool
	IdempotencyTTL    time.Duration
}

// NewResilientProviderSimple 使用默认配置创建弹性 Provider.
// 这是简单用例的便捷函数。
func NewResilientProviderSimple(provider Provider, _ interface{}, logger *zap.Logger) *ResilientProvider {
	return NewResilientProvider(provider, nil, logger)
}

// NewResylient Provider创建了具有弹性的提供者包装.
func NewResilientProvider(provider Provider, config *ResilientConfig, logger *zap.Logger) *ResilientProvider {
	if config == nil {
		config = &ResilientConfig{
			RetryPolicy:       DefaultRetryPolicy(),
			CircuitBreaker:    DefaultCircuitBreakerConfig(),
			EnableIdempotency: true,
			IdempotencyTTL:    1 * time.Hour,
		}
	}

	return &ResilientProvider{
		provider:       provider,
		retryPolicy:    config.RetryPolicy,
		circuitBreaker: NewCircuitBreaker(config.CircuitBreaker, logger),
		idempotencyTTL: config.IdempotencyTTL,
		logger:         logger,
	}
}

// 完成器具有弹性。
func (rp *ResilientProvider) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	key := rp.generateIdempotencyKey(req)
	if cached, ok := rp.idempotencyMap.Load(key); ok {
		if entry, ok := cached.(*idempotencyEntry); ok {
			if time.Now().Before(entry.expiresAt) {
				return entry.response, nil
			}
			rp.idempotencyMap.Delete(key)
		}
	}

	var resp *ChatResponse
	var lastErr error

	err := rp.circuitBreaker.Call(ctx, func() error {
		backoff := rp.retryPolicy.InitialBackoff

		for i := 0; i <= rp.retryPolicy.MaxRetries; i++ {
			var err error
			resp, err = rp.provider.Completion(ctx, req)
			if err == nil {
				return nil
			}

			lastErr = err
			if !IsRetryable(err) {
				return err
			}

			if i < rp.retryPolicy.MaxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				backoff = time.Duration(float64(backoff) * rp.retryPolicy.Multiplier)
				if backoff > rp.retryPolicy.MaxBackoff {
					backoff = rp.retryPolicy.MaxBackoff
				}
			}
		}
		return lastErr
	})

	if err != nil {
		return nil, err
	}

	rp.idempotencyMap.Store(key, &idempotencyEntry{
		response:  resp,
		expiresAt: time.Now().Add(rp.idempotencyTTL),
	})

	return resp, nil
}

// Stream 执行器 提供器( 不重试进行 streaming) 。
func (rp *ResilientProvider) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	if rp.circuitBreaker.State() == CircuitOpen {
		return nil, ErrCircuitOpen
	}
	return rp.provider.Stream(ctx, req)
}

// 健康检查工具 提供者。
func (rp *ResilientProvider) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return rp.provider.HealthCheck(ctx)
}

// 名称执行提供方.
func (rp *ResilientProvider) Name() string {
	return rp.provider.Name()
}

// 支持 NativeFunctionCalling 执行提供者.
func (rp *ResilientProvider) SupportsNativeFunctionCalling() bool {
	return rp.provider.SupportsNativeFunctionCalling()
}

// ListModels 执行提供者 。
func (rp *ResilientProvider) ListModels(ctx context.Context) ([]Model, error) {
	return rp.provider.ListModels(ctx)
}

func (rp *ResilientProvider) generateIdempotencyKey(req *ChatRequest) string {
	data, _ := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{
		Model:    req.Model,
		Messages: req.Messages,
	})
	return string(data)
}

type idempotencyEntry struct {
	response  *ChatResponse
	expiresAt time.Time
}
