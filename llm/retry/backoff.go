package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"go.uber.org/zap"
)

// RetryPolicy 定义重试策略配置
// 遵循 KISS 原则：简单但功能完整的重试策略
type RetryPolicy struct {
	MaxRetries      int                                               // 最大重试次数（0 表示不重试）
	InitialDelay    time.Duration                                     // 初始延迟时间
	MaxDelay        time.Duration                                     // 最大延迟时间
	Multiplier      float64                                           // 延迟时间倍增因子（指数退避）
	Jitter          bool                                              // 是否添加随机抖动（防止雪崩）
	RetryableErrors []error                                           // 可重试的错误类型（为空则重试所有错误）
	OnRetry         func(attempt int, err error, delay time.Duration) // 重试回调
}

// DefaultRetryPolicy 返回默认的重试策略
// 适用于大部分 LLM API 调用场景
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       true,
	}
}

// Retryer 重试器接口
// 提供统一的重试能力
type Retryer interface {
	// Do 执行函数，失败时根据策略重试
	Do(ctx context.Context, fn func() error) error

	// DoWithResult 执行函数并返回结果，失败时根据策略重试
	DoWithResult(ctx context.Context, fn func() (any, error)) (any, error)
}

// backoffRetryer 基于指数退避的重试器实现
type backoffRetryer struct {
	policy *RetryPolicy
	logger *zap.Logger
}

// NewBackoffRetryer 创建指数退避重试器
func NewBackoffRetryer(policy *RetryPolicy, logger *zap.Logger) Retryer {
	if policy == nil {
		policy = DefaultRetryPolicy()
	}

	// 参数校验
	if policy.MaxRetries < 0 {
		policy.MaxRetries = 0
	}
	if policy.InitialDelay <= 0 {
		policy.InitialDelay = 1 * time.Second
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 30 * time.Second
	}
	if policy.Multiplier < 1.0 {
		policy.Multiplier = 2.0
	}

	return &backoffRetryer{
		policy: policy,
		logger: logger,
	}
}

// Do 实现 Retryer.Do
func (r *backoffRetryer) Do(ctx context.Context, fn func() error) error {
	_, err := r.DoWithResult(ctx, func() (any, error) {
		return nil, fn()
	})
	return err
}

// DoWithResult 实现 Retryer.DoWithResult
// 核心重试逻辑：指数退避 + 随机抖动 + 错误过滤
func (r *backoffRetryer) DoWithResult(ctx context.Context, fn func() (any, error)) (any, error) {
	var lastErr error
	var result any

	for attempt := 0; attempt <= r.policy.MaxRetries; attempt++ {
		// 第一次执行不延迟
		if attempt > 0 {
			delay := r.calculateDelay(attempt)

			// 日志记录
			r.logger.Debug("重试中",
				zap.Int("attempt", attempt),
				zap.Int("max_retries", r.policy.MaxRetries),
				zap.Duration("delay", delay),
				zap.Error(lastErr),
			)

			// 调用回调
			if r.policy.OnRetry != nil {
				r.policy.OnRetry(attempt, lastErr, delay)
			}

			// 等待延迟，同时监听 context 取消
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("重试被取消: %w", ctx.Err())
			case <-time.After(delay):
				// 继续重试
			}
		}

		// 执行函数
		result, lastErr = fn()

		// 成功，直接返回
		if lastErr == nil {
			if attempt > 0 {
				r.logger.Info("重试成功",
					zap.Int("attempt", attempt),
				)
			}
			return result, nil
		}

		// 检查是否可重试
		if !r.isRetryable(lastErr) {
			r.logger.Debug("错误不可重试",
				zap.Error(lastErr),
			)
			return nil, lastErr
		}

		// 检查是否还有重试次数
		if attempt >= r.policy.MaxRetries {
			break
		}
	}

	// 所有重试都失败了
	r.logger.Warn("重试次数耗尽",
		zap.Int("attempts", r.policy.MaxRetries+1),
		zap.Error(lastErr),
	)

	return nil, fmt.Errorf("重试 %d 次后仍失败: %w", r.policy.MaxRetries, lastErr)
}

// calculateDelay 计算延迟时间
// 使用指数退避算法 + 可选的随机抖动
func (r *backoffRetryer) calculateDelay(attempt int) time.Duration {
	// 指数退避：delay = initial * multiplier^(attempt-1)
	delay := float64(r.policy.InitialDelay) * math.Pow(r.policy.Multiplier, float64(attempt-1))

	// 限制最大延迟
	if delay > float64(r.policy.MaxDelay) {
		delay = float64(r.policy.MaxDelay)
	}

	// 添加随机抖动（±25%）
	// 目的：防止多个客户端同时重试导致的雪崩效应
	if r.policy.Jitter {
		jitter := delay * 0.25 // 抖动范围：±25%
		delay = delay + (rand.Float64()*2-1)*jitter
	}

	// 确保延迟不小于初始延迟
	if delay < float64(r.policy.InitialDelay) {
		delay = float64(r.policy.InitialDelay)
	}

	return time.Duration(delay)
}

// isRetryable 检查错误是否可重试
func (r *backoffRetryer) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// 如果没有配置可重试错误列表，则所有错误都可重试
	if len(r.policy.RetryableErrors) == 0 {
		return true
	}

	// 检查错误是否在可重试列表中
	for _, retryableErr := range r.policy.RetryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	return false
}

// RetryableError 可重试的错误类型
// 用于标记哪些错误应该触发重试
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// IsRetryableError 检查错误是否被 WrapRetryable 包装为可重试错误。
// 注意：这与 types.IsRetryable 语义不同 —— 本函数检查 *RetryableError 包装类型，
// 而 types.IsRetryable 检查 *types.Error 的 Retryable 字段。
func IsRetryableError(err error) bool {
	var retryableErr *RetryableError
	return errors.As(err, &retryableErr)
}

// IsRetryable is an alias for IsRetryableError.
//
// Deprecated: 为避免与 types.IsRetryable 混淆，请使用 IsRetryableError。
var IsRetryable = IsRetryableError

// WrapRetryable 将错误包装为可重试错误
func WrapRetryable(err error) error {
	if err == nil {
		return nil
	}
	return &RetryableError{Err: err}
}
