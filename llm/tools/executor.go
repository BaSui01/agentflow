package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ToolFunc 定义工具函数签名.
type ToolFunc func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// ToolMetadata 描述工具元数据.
type ToolMetadata struct {
	Schema      llm.ToolSchema   // Tool JSON Schema
	Permission  string           // Required permission (optional)
	RateLimit   *RateLimitConfig // Rate limit config (optional)
	Timeout     time.Duration    // Execution timeout (default 30s)
	Description string           // Detailed description
}

// RateLimitConfig 定义速率限制配置.
type RateLimitConfig struct {
	MaxCalls int           // Maximum calls
	Window   time.Duration // Time window
}

// ToolResult 表示工具执行结果.
type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration"`
	FromCache  bool            `json:"from_cache,omitempty"`
}

// ToolRegistry 定义工具注册接口.
type ToolRegistry interface {
	Register(name string, fn ToolFunc, metadata ToolMetadata) error
	Unregister(name string) error
	Get(name string) (ToolFunc, ToolMetadata, error)
	List() []llm.ToolSchema
	Has(name string) bool
}

// ToolExecutor 定义工具执行器接口.
type ToolExecutor interface {
	Execute(ctx context.Context, calls []llm.ToolCall) []ToolResult
	ExecuteOne(ctx context.Context, call llm.ToolCall) ToolResult
}

// ToolStreamEventType 定义流式工具执行事件类型.
type ToolStreamEventType string

const (
	// ToolStreamProgress 表示工具执行进度事件.
	ToolStreamProgress ToolStreamEventType = "progress"
	// ToolStreamOutput 表示工具执行输出事件.
	ToolStreamOutput ToolStreamEventType = "output"
	// ToolStreamComplete 表示工具执行完成事件.
	ToolStreamComplete ToolStreamEventType = "complete"
	// ToolStreamError 表示工具执行错误事件.
	ToolStreamError ToolStreamEventType = "error"
)

// ToolStreamEvent 表示流式工具执行中的单个事件.
type ToolStreamEvent struct {
	Type     ToolStreamEventType `json:"type"`
	ToolName string              `json:"tool_name"`
	Data     any                 `json:"data,omitempty"`
	Error    error               `json:"-"`
}

// StreamableToolExecutor 是 ToolExecutor 的可选扩展接口（Optional Interface pattern, §23），
// 支持流式工具执行以报告长时间运行工具的进度.
type StreamableToolExecutor interface {
	ToolExecutor
	ExecuteOneStream(ctx context.Context, call llm.ToolCall) <-chan ToolStreamEvent
}

// ExecutorConfig 定义工具执行器的可配置参数.
type ExecutorConfig struct {
	MaxRetries   int           // 单个工具失败时的最大重试次数（0 表示不重试）
	RetryDelay   time.Duration // 首次重试前的等待时间
	RetryBackoff float64       // 重试间隔的指数退避乘数（例如 2.0 表示每次翻倍）
}

// DefaultExecutorConfig 返回默认的执行器配置（不重试）.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		MaxRetries:   0,
		RetryDelay:   100 * time.Millisecond,
		RetryBackoff: 2.0,
	}
}

// ====== 实现：DefaultRegistry ======

type DefaultRegistry struct {
	mu         sync.RWMutex
	tools      map[string]ToolFunc
	metadata   map[string]ToolMetadata
	rateLimits map[string]*rateLimiter // 工具级别的速率限制器
	logger     *zap.Logger
}

// NewDefaultRegistry 创建默认的工具注册中心。
func NewDefaultRegistry(logger *zap.Logger) *DefaultRegistry {
	return &DefaultRegistry{
		tools:      make(map[string]ToolFunc),
		metadata:   make(map[string]ToolMetadata),
		rateLimits: make(map[string]*rateLimiter),
		logger:     logger,
	}
}

func (r *DefaultRegistry) Register(name string, fn ToolFunc, metadata ToolMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %s already registered", name)
	}

	// 校验 Schema
	if metadata.Schema.Name == "" {
		metadata.Schema.Name = name
	}
	if metadata.Schema.Name != name {
		return fmt.Errorf("tool name mismatch: schema.Name=%s, register name=%s", metadata.Schema.Name, name)
	}

	// 设置默认超时
	if metadata.Timeout == 0 {
		metadata.Timeout = 30 * time.Second
	}

	r.tools[name] = fn
	r.metadata[name] = metadata

	// 初始化速率限制器
	if metadata.RateLimit != nil {
		r.rateLimits[name] = newRateLimiter(metadata.RateLimit.MaxCalls, metadata.RateLimit.Window)
	}

	r.logger.Info("tool registered", zap.String("name", name), zap.Duration("timeout", metadata.Timeout))
	return nil
}

func (r *DefaultRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool %s not found", name)
	}

	delete(r.tools, name)
	delete(r.metadata, name)
	delete(r.rateLimits, name)

	r.logger.Info("tool unregistered", zap.String("name", name))
	return nil
}

func (r *DefaultRegistry) Get(name string) (ToolFunc, ToolMetadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fn, ok := r.tools[name]
	if !ok {
		return nil, ToolMetadata{}, fmt.Errorf("tool %s not found", name)
	}

	meta := r.metadata[name]
	return fn, meta, nil
}

func (r *DefaultRegistry) List() []llm.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]llm.ToolSchema, 0, len(r.metadata))
	for _, meta := range r.metadata {
		schemas = append(schemas, meta.Schema)
	}
	return schemas
}

func (r *DefaultRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// checkRateLimit 检查是否触发速率限制
func (r *DefaultRegistry) checkRateLimit(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	limiter, ok := r.rateLimits[name]
	if !ok {
		return nil // 没有速率限制
	}

	return limiter.Allow()
}

// ====== 实现：DefaultExecutor ======

type DefaultExecutor struct {
	registry ToolRegistry
	logger   *zap.Logger
	config   ExecutorConfig
}

// NewDefaultExecutor 创建默认的工具执行器（无重试）。
func NewDefaultExecutor(registry ToolRegistry, logger *zap.Logger) *DefaultExecutor {
	return &DefaultExecutor{
		registry: registry,
		logger:   logger,
		config:   DefaultExecutorConfig(),
	}
}

// NewDefaultExecutorWithConfig 创建带自定义配置的工具执行器。
func NewDefaultExecutorWithConfig(registry ToolRegistry, logger *zap.Logger, config ExecutorConfig) *DefaultExecutor {
	if config.RetryDelay <= 0 {
		config.RetryDelay = 100 * time.Millisecond
	}
	if config.RetryBackoff <= 0 {
		config.RetryBackoff = 2.0
	}
	return &DefaultExecutor{
		registry: registry,
		logger:   logger,
		config:   config,
	}
}

func (e *DefaultExecutor) Execute(ctx context.Context, calls []llm.ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))

	// 并发执行所有工具调用，单个工具失败不阻塞其他工具
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c llm.ToolCall) {
			defer wg.Done()
			results[idx] = e.executeWithRetry(ctx, c)
		}(i, call)
	}
	wg.Wait()

	return results
}

// executeWithRetry 执行单个工具调用，失败时按配置重试.
func (e *DefaultExecutor) executeWithRetry(ctx context.Context, call llm.ToolCall) ToolResult {
	result := e.ExecuteOne(ctx, call)
	if result.Error == "" || e.config.MaxRetries <= 0 {
		return result
	}

	delay := e.config.RetryDelay
	for attempt := 1; attempt <= e.config.MaxRetries; attempt++ {
		e.logger.Warn("retrying tool execution",
			zap.String("name", call.Name),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", e.config.MaxRetries),
			zap.String("last_error", result.Error))

		select {
		case <-ctx.Done():
			result.Error = fmt.Sprintf("retry cancelled: %v", ctx.Err())
			return result
		case <-time.After(delay):
		}

		result = e.ExecuteOne(ctx, call)
		if result.Error == "" {
			return result
		}

		delay = time.Duration(float64(delay) * e.config.RetryBackoff)
	}

	return result
}

func (e *DefaultExecutor) ExecuteOne(ctx context.Context, call llm.ToolCall) ToolResult {
	start := time.Now()
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	// 1. 获取工具函数和元数据
	fn, meta, err := e.registry.Get(call.Name)
	if err != nil {
		result.Error = fmt.Sprintf("tool not found: %s", err.Error())
		result.Duration = time.Since(start)
		e.logger.Error("tool not found", zap.String("name", call.Name), zap.Error(err))
		return result
	}

	// 2. 检查速率限制（如果注册表支持）
	if reg, ok := e.registry.(*DefaultRegistry); ok {
		if err := reg.checkRateLimit(call.Name); err != nil {
			result.Error = fmt.Sprintf("rate limit exceeded: %s", err.Error())
			result.Duration = time.Since(start)
			e.logger.Warn("rate limit exceeded", zap.String("name", call.Name))
			return result
		}
	}

	// 3. 参数校验（简单校验：确保是有效 JSON）
	if len(call.Arguments) > 0 {
		var tmp any
		if err := json.Unmarshal(call.Arguments, &tmp); err != nil {
			result.Error = fmt.Sprintf("invalid arguments: %s", err.Error())
			result.Duration = time.Since(start)
			e.logger.Error("invalid tool arguments", zap.String("name", call.Name), zap.Error(err))
			return result
		}
	}

	// 4. 执行工具（带超时控制）
	execCtx, cancel := context.WithTimeout(ctx, meta.Timeout)
	defer cancel()

	// 使用带缓冲的 channel 防止 goroutine 泄漏
	// 即使超时后没人接收，goroutine 也能正常退出
	doneChan := make(chan struct {
		res json.RawMessage
		err error
	}, 1)

	go func() {
		res, err := fn(execCtx, call.Arguments)
		// 使用 select 确保即使超时也能退出
		select {
		case doneChan <- struct {
			res json.RawMessage
			err error
		}{res, err}:
		case <-execCtx.Done():
			// 上下文已取消，直接退出，不阻塞
		}
	}()

	select {
	case done := <-doneChan:
		if done.err != nil {
			result.Error = done.err.Error()
			result.Duration = time.Since(start)
			e.logger.Error("tool execution failed",
				zap.String("name", call.Name),
				zap.Error(done.err),
				zap.Duration("duration", result.Duration))
		} else {
			result.Result = done.res
			result.Duration = time.Since(start)
			e.logger.Info("tool executed successfully",
				zap.String("name", call.Name),
				zap.Duration("duration", result.Duration))
		}

	case <-execCtx.Done():
		result.Error = fmt.Sprintf("execution timeout after %s", meta.Timeout)
		result.Duration = time.Since(start)
		e.logger.Error("tool execution timeout",
			zap.String("name", call.Name),
			zap.Duration("timeout", meta.Timeout))
	}

	return result
}

// ExecuteOneStream 执行单个工具调用并通过 channel 发射流式事件.
// 支持长时间运行工具的进度回报和 context cancellation.
// channel 在 goroutine 结束时保证关闭.
func (e *DefaultExecutor) ExecuteOneStream(ctx context.Context, call llm.ToolCall) <-chan ToolStreamEvent {
	ch := make(chan ToolStreamEvent, 4)

	go func() {
		defer close(ch)

		// 发射 progress 事件：开始执行
		select {
		case ch <- ToolStreamEvent{
			Type:     ToolStreamProgress,
			ToolName: call.Name,
			Data:     "starting execution",
		}:
		case <-ctx.Done():
			ch <- ToolStreamEvent{
				Type:     ToolStreamError,
				ToolName: call.Name,
				Error:    ctx.Err(),
			}
			return
		}

		// 使用 executeWithRetry 执行（包含重试逻辑）
		result := e.executeWithRetry(ctx, call)

		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			ch <- ToolStreamEvent{
				Type:     ToolStreamError,
				ToolName: call.Name,
				Error:    ctx.Err(),
			}
			return
		default:
		}

		if result.Error != "" {
			ch <- ToolStreamEvent{
				Type:     ToolStreamError,
				ToolName: call.Name,
				Error:    fmt.Errorf("%s", result.Error),
			}
			return
		}

		// 发射 output 事件
		ch <- ToolStreamEvent{
			Type:     ToolStreamOutput,
			ToolName: call.Name,
			Data:     result.Result,
		}

		// 发射 complete 事件
		ch <- ToolStreamEvent{
			Type:     ToolStreamComplete,
			ToolName: call.Name,
			Data:     result,
		}
	}()

	return ch
}

// Compile-time check: DefaultExecutor implements StreamableToolExecutor.
var _ StreamableToolExecutor = (*DefaultExecutor)(nil)

// ====== 速率限制器 ======

// tokenBucketLimiter 实现令牌桶速率限制器，Allow() 时间复杂度为 O(1)
type tokenBucketLimiter struct {
	mu         sync.Mutex
	tokens     float64   // Current available tokens
	maxTokens  float64   // Maximum tokens (bucket capacity)
	refillRate float64   // Tokens per second
	lastRefill time.Time // Last refill timestamp
}

// newTokenBucketLimiter 创建新的令牌桶速率限制器
// maxCalls: 时间窗口内允许的最大调用次数
// window: 时间窗口持续时间
func newTokenBucketLimiter(maxCalls int, window time.Duration) *tokenBucketLimiter {
	refillRate := float64(maxCalls) / window.Seconds()
	return &tokenBucketLimiter{
		tokens:     float64(maxCalls),
		maxTokens:  float64(maxCalls),
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow 检查请求是否被允许（O(1) 时间复杂度）
func (tb *tokenBucketLimiter) Allow() error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// 根据已过去的时间补充令牌
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate

	// 令牌数上限封顶
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}

	tb.lastRefill = now

	// 检查是否有可用的令牌
	if tb.tokens < 1 {
		return fmt.Errorf("rate limit exceeded: no tokens available")
	}

	// 消耗一个令牌
	tb.tokens--
	return nil
}

// Tokens 返回当前可用的令牌数（用于监控）
func (tb *tokenBucketLimiter) Tokens() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

// Reset 将限制器重置为满容量
func (tb *tokenBucketLimiter) Reset() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = tb.maxTokens
	tb.lastRefill = time.Now()
}

// rateLimiter 为向后兼容而保留，内部使用 tokenBucketLimiter
// Deprecated: 新代码请直接使用 tokenBucketLimiter
type rateLimiter struct {
	internal *tokenBucketLimiter
}

// newRateLimiter 创建新的速率限制器（内部使用令牌桶）
func newRateLimiter(maxCalls int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		internal: newTokenBucketLimiter(maxCalls, window),
	}
}

func (rl *rateLimiter) Allow() error {
	return rl.internal.Allow()
}
