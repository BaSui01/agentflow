package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// State 熔断器状态
type State int

const (
	// StateClosed 关闭状态（正常工作）
	StateClosed State = iota
	// StateOpen 打开状态（熔断中）
	StateOpen
	// StateHalfOpen 半开状态（试探性恢复）
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "Closed"
	case StateOpen:
		return "Open"
	case StateHalfOpen:
		return "HalfOpen"
	default:
		return "Unknown"
	}
}

// Config 熔断器配置
type Config struct {
	// Threshold 连续失败次数阈值（触发熔断）
	Threshold int

	// Timeout 单次调用超时时间
	Timeout time.Duration

	// ResetTimeout 熔断恢复等待时间（从 Open -> HalfOpen）
	ResetTimeout time.Duration

	// HalfOpenMaxCalls 半开状态下允许的最大请求数
	HalfOpenMaxCalls int

	// OnStateChange 状态变更回调
	OnStateChange func(from State, to State)
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		Threshold:        5,
		Timeout:          30 * time.Second,
		ResetTimeout:     60 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

// CircuitBreaker 熔断器接口
type CircuitBreaker interface {
	// Call 执行调用，如果熔断器打开则返回错误
	Call(ctx context.Context, fn func() error) error

	// CallWithResult 执行调用并返回结果
	CallWithResult(ctx context.Context, fn func() (any, error)) (any, error)

	// State 获取当前状态
	State() State

	// Reset 重置熔断器（手动恢复）
	Reset()
}

// breaker 熔断器实现
type breaker struct {
	config *Config
	logger *zap.Logger

	mu                sync.RWMutex
	state             State
	failureCount      int       // 连续失败次数
	lastFailureTime   time.Time // 最后失败时间
	halfOpenCallCount int       // 半开状态下的调用次数
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(config *Config, logger *zap.Logger) CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}

	// 参数校验
	if config.Threshold <= 0 {
		config.Threshold = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 60 * time.Second
	}
	if config.HalfOpenMaxCalls <= 0 {
		config.HalfOpenMaxCalls = 3
	}

	return &breaker{
		config: config,
		logger: logger,
		state:  StateClosed,
	}
}

// Call 实现 CircuitBreaker.Call
func (b *breaker) Call(ctx context.Context, fn func() error) error {
	_, err := b.CallWithResult(ctx, func() (any, error) {
		return nil, fn()
	})
	return err
}

// CallWithResult 实现 CircuitBreaker.CallWithResult
// 核心逻辑：状态机转换 + 失败计数 + 超时控制
func (b *breaker) CallWithResult(ctx context.Context, fn func() (any, error)) (any, error) {
	// 检查熔断器状态
	if err := b.beforeCall(); err != nil {
		return nil, err
	}

	// 创建超时 context
	callCtx, cancel := context.WithTimeout(ctx, b.config.Timeout)
	defer cancel()

	// 执行调用
	resultCh := make(chan callResult, 1)
	go func() {
		result, err := fn()
		resultCh <- callResult{result: result, err: err}
	}()

	// 等待结果或超时
	select {
	case <-callCtx.Done():
		// 超时
		err := fmt.Errorf("调用超时: %w", callCtx.Err())
		b.afterCall(false)
		return nil, err

	case res := <-resultCh:
		// 调用完成
		// 客户端错误（如无效请求）不应计入熔断失败
		success := res.err == nil || isClientError(res.err)
		b.afterCall(success)

		if !success {
			return nil, res.err
		}

		return res.result, nil
	}
}

type callResult struct {
	result any
	err    error
}

// isClientError 判断错误是否为客户端错误（不应计入熔断失败）。
func isClientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, code := range []string{
		"INVALID_REQUEST", "AUTHENTICATION", "UNAUTHORIZED",
		"FORBIDDEN", "QUOTA_EXCEEDED", "CONTENT_FILTERED",
		"TOOL_VALIDATION", "CONTEXT_TOO_LONG",
	} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return false
}

// beforeCall 调用前检查
func (b *breaker) beforeCall() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		// 关闭状态，允许调用
		return nil

	case StateOpen:
		// 检查是否可以进入半开状态
		if time.Since(b.lastFailureTime) > b.config.ResetTimeout {
			b.setState(StateHalfOpen)
			b.halfOpenCallCount = 0
			b.logger.Info("熔断器进入半开状态")
			return nil
		}

		// 仍在熔断中
		return ErrCircuitOpen

	case StateHalfOpen:
		// 半开状态，限制调用次数
		if b.halfOpenCallCount >= b.config.HalfOpenMaxCalls {
			return ErrTooManyCallsInHalfOpen
		}
		b.halfOpenCallCount++
		return nil

	default:
		return fmt.Errorf("未知的熔断器状态: %v", b.state)
	}
}

// afterCall 调用后处理
func (b *breaker) afterCall(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if success {
		// 调用成功
		b.onSuccess()
	} else {
		// 调用失败
		b.onFailure()
	}
}

// onSuccess 处理成功调用
func (b *breaker) onSuccess() {
	switch b.state {
	case StateClosed:
		// 关闭状态，重置失败计数
		b.failureCount = 0

	case StateHalfOpen:
		// 半开状态，成功后恢复到关闭状态
		b.logger.Info("熔断器恢复正常",
			zap.Int("half_open_calls", b.halfOpenCallCount),
		)
		b.setState(StateClosed)
		b.failureCount = 0
		b.halfOpenCallCount = 0

	case StateOpen:
		// 打开状态不应该有调用
		b.logger.Warn("熔断器打开状态收到成功响应")
	}
}

// onFailure 处理失败调用
func (b *breaker) onFailure() {
	b.failureCount++
	b.lastFailureTime = time.Now()

	switch b.state {
	case StateClosed:
		// 关闭状态，检查是否达到阈值
		if b.failureCount >= b.config.Threshold {
			b.logger.Warn("熔断器打开",
				zap.Int("failure_count", b.failureCount),
				zap.Int("threshold", b.config.Threshold),
			)
			b.setState(StateOpen)
		}

	case StateHalfOpen:
		// 半开状态，失败后重新打开
		b.logger.Warn("熔断器半开状态失败，重新打开",
			zap.Int("half_open_calls", b.halfOpenCallCount),
		)
		b.setState(StateOpen)
		b.halfOpenCallCount = 0

	case StateOpen:
		// 打开状态不应该有调用
		b.logger.Warn("熔断器打开状态收到失败响应")
	}
}

// setState 设置状态并触发回调
func (b *breaker) setState(newState State) {
	oldState := b.state
	b.state = newState

	if b.config.OnStateChange != nil {
		go b.config.OnStateChange(oldState, newState)
	}
}

// State 实现 CircuitBreaker.State
func (b *breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// Reset 实现 CircuitBreaker.Reset
func (b *breaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	oldState := b.state
	b.state = StateClosed
	b.failureCount = 0
	b.halfOpenCallCount = 0

	b.logger.Info("熔断器已重置",
		zap.String("from_state", oldState.String()),
	)

	if b.config.OnStateChange != nil {
		go b.config.OnStateChange(oldState, StateClosed)
	}
}

// 错误定义
var (
	ErrCircuitOpen            = errors.New("熔断器已打开")
	ErrTooManyCallsInHalfOpen = errors.New("半开状态下调用次数过多")
)
