package workflow

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CircuitState 熔断器状态
type CircuitState int

const (
	// CircuitClosed 正常状态，允许请求通过
	CircuitClosed CircuitState = iota
	// CircuitOpen 熔断状态，拒绝所有请求
	CircuitOpen
	// CircuitHalfOpen 半开状态，允许探测请求
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	// FailureThreshold 连续失败次数阈值，达到后触发熔断
	FailureThreshold int `json:"failure_threshold"`
	// RecoveryTimeout 熔断后等待恢复的时间
	RecoveryTimeout time.Duration `json:"recovery_timeout"`
	// HalfOpenMaxProbes 半开状态允许的探测请求数
	HalfOpenMaxProbes int `json:"half_open_max_probes"`
	// SuccessThresholdInHalfOpen 半开状态下连续成功多少次后恢复
	SuccessThresholdInHalfOpen int `json:"success_threshold_in_half_open"`
}

// DefaultCircuitBreakerConfig 默认熔断器配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:           5,
		RecoveryTimeout:            30 * time.Second,
		HalfOpenMaxProbes:          3,
		SuccessThresholdInHalfOpen: 2,
	}
}

// CircuitBreakerEvent 熔断器状态变更事件
type CircuitBreakerEvent struct {
	NodeID    string       `json:"node_id"`
	OldState  CircuitState `json:"old_state"`
	NewState  CircuitState `json:"new_state"`
	Timestamp time.Time    `json:"timestamp"`
	Reason    string       `json:"reason"`
	Failures  int          `json:"failures"`
}

// CircuitBreakerEventHandler 事件处理器接口
type CircuitBreakerEventHandler interface {
	OnStateChange(event CircuitBreakerEvent)
}

// CircuitBreaker 熔断器实现
type CircuitBreaker struct {
	nodeID          string
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int       // 连续失败次数
	successes       int       // 半开状态下连续成功次数
	lastFailureTime time.Time // 最后一次失败时间
	probeCount      int       // 半开状态下已探测次数
	eventHandler    CircuitBreakerEventHandler
	logger          *zap.Logger
	mu              sync.RWMutex
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(
	nodeID string,
	config CircuitBreakerConfig,
	eventHandler CircuitBreakerEventHandler,
	logger *zap.Logger,
) *CircuitBreaker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CircuitBreaker{
		nodeID:       nodeID,
		config:       config,
		state:        CircuitClosed,
		eventHandler: eventHandler,
		logger:       logger.With(zap.String("node_id", nodeID)),
	}
}

// AllowRequest 检查是否允许请求通过
func (cb *CircuitBreaker) AllowRequest() (bool, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true, nil

	case CircuitOpen:
		// 检查是否到了恢复时间
		if time.Since(cb.lastFailureTime) >= cb.config.RecoveryTimeout {
			cb.transitionTo(CircuitHalfOpen, "recovery timeout elapsed")
			cb.probeCount = 0
			cb.successes = 0
			return true, nil
		}
		return false, fmt.Errorf("circuit breaker open for node %s: %d consecutive failures, retry after %v",
			cb.nodeID, cb.failures, cb.config.RecoveryTimeout-time.Since(cb.lastFailureTime))

	case CircuitHalfOpen:
		if cb.probeCount < cb.config.HalfOpenMaxProbes {
			cb.probeCount++
			return true, nil
		}
		return false, fmt.Errorf("circuit breaker half-open for node %s: max probes (%d) reached",
			cb.nodeID, cb.config.HalfOpenMaxProbes)

	default:
		return false, fmt.Errorf("unknown circuit breaker state: %d", cb.state)
	}
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failures = 0 // 重置失败计数

	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThresholdInHalfOpen {
			cb.transitionTo(CircuitClosed, fmt.Sprintf("%d consecutive successes in half-open", cb.successes))
			cb.failures = 0
			cb.successes = 0
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen, fmt.Sprintf("%d consecutive failures", cb.failures))
		}

	case CircuitHalfOpen:
		// 半开状态下任何失败都重新熔断
		cb.successes = 0
		cb.transitionTo(CircuitOpen, "failure in half-open state")
	}
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailures 获取当前失败次数
func (cb *CircuitBreaker) GetFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Reset 重置熔断器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	oldState := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.probeCount = 0
	if oldState != CircuitClosed {
		cb.emitEvent(oldState, CircuitClosed, "manual reset")
	}
}

// transitionTo 状态转换（必须在锁内调用）
func (cb *CircuitBreaker) transitionTo(newState CircuitState, reason string) {
	oldState := cb.state
	cb.state = newState

	cb.logger.Info("circuit breaker state change",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()),
		zap.String("reason", reason),
		zap.Int("failures", cb.failures))

	cb.emitEvent(oldState, newState, reason)
}

// emitEvent 发送事件（必须在锁内调用）
func (cb *CircuitBreaker) emitEvent(oldState, newState CircuitState, reason string) {
	if cb.eventHandler != nil {
		event := CircuitBreakerEvent{
			NodeID:    cb.nodeID,
			OldState:  oldState,
			NewState:  newState,
			Timestamp: time.Now(),
			Reason:    reason,
			Failures:  cb.failures,
		}
		// 异步发送避免死锁
		go cb.eventHandler.OnStateChange(event)
	}
}

// CircuitBreakerRegistry 熔断器注册表，管理所有节点的熔断器
type CircuitBreakerRegistry struct {
	breakers     map[string]*CircuitBreaker
	config       CircuitBreakerConfig
	eventHandler CircuitBreakerEventHandler
	logger       *zap.Logger
	mu           sync.RWMutex
}

// NewCircuitBreakerRegistry 创建熔断器注册表
func NewCircuitBreakerRegistry(
	config CircuitBreakerConfig,
	eventHandler CircuitBreakerEventHandler,
	logger *zap.Logger,
) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers:     make(map[string]*CircuitBreaker),
		config:       config,
		eventHandler: eventHandler,
		logger:       logger,
	}
}

// GetOrCreate 获取或创建节点的熔断器
func (r *CircuitBreakerRegistry) GetOrCreate(nodeID string) *CircuitBreaker {
	r.mu.RLock()
	if cb, ok := r.breakers[nodeID]; ok {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// 双重检查
	if cb, ok := r.breakers[nodeID]; ok {
		return cb
	}

	cb := NewCircuitBreaker(nodeID, r.config, r.eventHandler, r.logger)
	r.breakers[nodeID] = cb
	return cb
}

// GetAllStates 获取所有熔断器状态
func (r *CircuitBreakerRegistry) GetAllStates() map[string]CircuitState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	states := make(map[string]CircuitState, len(r.breakers))
	for id, cb := range r.breakers {
		states[id] = cb.GetState()
	}
	return states
}

// ResetAll 重置所有熔断器
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}
