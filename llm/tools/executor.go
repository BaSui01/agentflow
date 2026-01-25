package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ToolFunc 定义工具函数的签名。
// 输入为 JSON 参数，输出为 JSON 结果或错误。
type ToolFunc func(ctx context.Context, args json.RawMessage) (json.RawMessage, error)

// ToolMetadata 描述工具的元数据。
type ToolMetadata struct {
	Schema      llmpkg.ToolSchema // 工具的 JSON Schema
	Permission  string            // 所需权限（可选，用于鉴权）
	RateLimit   *RateLimitConfig  // 速率限制配置（可选）
	Timeout     time.Duration     // 执行超时（默认 30s）
	Description string            // 详细描述
}

// RateLimitConfig 定义速率限制配置
type RateLimitConfig struct {
	MaxCalls int           // 最大调用次数
	Window   time.Duration // 时间窗口
}

// ToolResult 表示工具执行的结果。
type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`    // 对应的 ToolCall ID
	Name       string          `json:"name"`            // 工具名称
	Result     json.RawMessage `json:"result"`          // 执行结果（JSON）
	Error      string          `json:"error,omitempty"` // 错误信息
	Duration   time.Duration   `json:"duration"`        // 执行耗时
}

// ToolRegistry 定义工具注册中心接口。
type ToolRegistry interface {
	// Register 注册一个工具
	Register(name string, fn ToolFunc, metadata ToolMetadata) error

	// Unregister 注销一个工具
	Unregister(name string) error

	// Get 获取工具函数和元数据
	Get(name string) (ToolFunc, ToolMetadata, error)

	// List 列出所有已注册的工具
	List() []llmpkg.ToolSchema

	// Has 检查工具是否已注册
	Has(name string) bool
}

// ToolExecutor 定义工具执行器接口。
type ToolExecutor interface {
	// Execute 执行一组 ToolCalls，返回对应的 ToolResults
	Execute(ctx context.Context, calls []llmpkg.ToolCall) []ToolResult

	// ExecuteOne 执行单个 ToolCall
	ExecuteOne(ctx context.Context, call llmpkg.ToolCall) ToolResult
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
		r.rateLimits[name] = &rateLimiter{
			maxCalls: metadata.RateLimit.MaxCalls,
			window:   metadata.RateLimit.Window,
			calls:    make([]time.Time, 0),
		}
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

func (r *DefaultRegistry) List() []llmpkg.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]llmpkg.ToolSchema, 0, len(r.metadata))
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
}

// NewDefaultExecutor 创建默认的工具执行器。
func NewDefaultExecutor(registry ToolRegistry, logger *zap.Logger) *DefaultExecutor {
	return &DefaultExecutor{
		registry: registry,
		logger:   logger,
	}
}

func (e *DefaultExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))

	// 并发执行所有工具调用
	var wg sync.WaitGroup
	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c llmpkg.ToolCall) {
			defer wg.Done()
			results[idx] = e.ExecuteOne(ctx, c)
		}(i, call)
	}
	wg.Wait()

	return results
}

func (e *DefaultExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) ToolResult {
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
		var tmp interface{}
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

	resChan := make(chan json.RawMessage, 1)
	errChan := make(chan error, 1)

	go func() {
		res, err := fn(execCtx, call.Arguments)
		if err != nil {
			errChan <- err
		} else {
			resChan <- res
		}
	}()

	select {
	case res := <-resChan:
		result.Result = res
		result.Duration = time.Since(start)
		e.logger.Info("tool executed successfully",
			zap.String("name", call.Name),
			zap.Duration("duration", result.Duration))

	case err := <-errChan:
		result.Error = err.Error()
		result.Duration = time.Since(start)
		e.logger.Error("tool execution failed",
			zap.String("name", call.Name),
			zap.Error(err),
			zap.Duration("duration", result.Duration))

	case <-execCtx.Done():
		result.Error = fmt.Sprintf("execution timeout after %s", meta.Timeout)
		result.Duration = time.Since(start)
		e.logger.Error("tool execution timeout",
			zap.String("name", call.Name),
			zap.Duration("timeout", meta.Timeout))
	}

	return result
}

// ====== 速率限制器 ======

type rateLimiter struct {
	mu       sync.Mutex
	maxCalls int
	window   time.Duration
	calls    []time.Time
}

func (rl *rateLimiter) Allow() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// 清理过期记录
	validCalls := make([]time.Time, 0)
	for _, t := range rl.calls {
		if t.After(cutoff) {
			validCalls = append(validCalls, t)
		}
	}
	rl.calls = validCalls

	// 检查是否超限
	if len(rl.calls) >= rl.maxCalls {
		return fmt.Errorf("rate limit exceeded: %d calls in %s", rl.maxCalls, rl.window)
	}

	// 记录本次调用
	rl.calls = append(rl.calls, now)
	return nil
}
