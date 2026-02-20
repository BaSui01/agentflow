package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// 并行Config定义了并行工具执行的配置.
type ParallelConfig struct {
	MaxConcurrency   int           // Maximum concurrent tool executions (0 = unlimited)
	ExecutionTimeout time.Duration // Global timeout for all parallel executions
	FailFast         bool          // Stop all executions on first error
	RetryOnError     bool          // Retry failed tool calls
	MaxRetries       int           // Maximum retry attempts per tool
	RetryDelay       time.Duration // Delay between retries
	CollectPartial   bool          // Return partial results on timeout/cancel
	DependencyGraph  bool          // Enable dependency-aware execution order
}

// 默认ParallelConfig 返回并行执行的合理默认值 。
func DefaultParallelConfig() ParallelConfig {
	return ParallelConfig{
		MaxConcurrency:   10,
		ExecutionTimeout: 60 * time.Second,
		FailFast:         false,
		RetryOnError:     false,
		MaxRetries:       2,
		RetryDelay:       500 * time.Millisecond,
		CollectPartial:   true,
		DependencyGraph:  false,
	}
}

// 并行执行器同时执行多个工具调用和高级功能.
type ParallelExecutor struct {
	registry ToolRegistry
	config   ParallelConfig
	logger   *zap.Logger

	// 计量
	totalExecutions   int64
	successExecutions int64
	failedExecutions  int64
	totalDuration     int64 // nanoseconds
}

// NewParallelExecutor创建了一个新的并行工具执行器.
func NewParallelExecutor(registry ToolRegistry, config ParallelConfig, logger *zap.Logger) *ParallelExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.MaxConcurrency <= 0 {
		config.MaxConcurrency = 10
	}
	if config.ExecutionTimeout <= 0 {
		config.ExecutionTimeout = 60 * time.Second
	}
	return &ParallelExecutor{
		registry: registry,
		config:   config,
		logger:   logger,
	}
}

// 并行结果包含并行工具执行的结果.
type ParallelResult struct {
	Results       []ToolResult  `json:"results"`
	TotalDuration time.Duration `json:"total_duration"`
	Completed     int           `json:"completed"`
	Failed        int           `json:"failed"`
	Cancelled     int           `json:"cancelled"`
	PartialResult bool          `json:"partial_result"`
}

// Execute运行多个工具调用与货币控制并行.
func (p *ParallelExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) *ParallelResult {
	start := time.Now()
	result := &ParallelResult{
		Results: make([]ToolResult, len(calls)),
	}

	if len(calls) == 0 {
		result.TotalDuration = time.Since(start)
		return result
	}

	// 创建超时的执行上下文
	execCtx, cancel := context.WithTimeout(ctx, p.config.ExecutionTimeout)
	defer cancel()

	// 用于货币控制的Semaphore
	sem := make(chan struct{}, p.config.MaxConcurrency)

	// 故障快速取消的通道
	var failFastCancel context.CancelFunc
	if p.config.FailFast {
		execCtx, failFastCancel = context.WithCancel(execCtx)
		defer failFastCancel()
	}

	var wg sync.WaitGroup
	var firstError atomic.Value

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, c llmpkg.ToolCall) {
			defer wg.Done()

			// 获取分母
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-execCtx.Done():
				result.Results[idx] = ToolResult{
					ToolCallID: c.ID,
					Name:       c.Name,
					Error:      "execution cancelled before start",
				}
				atomic.AddInt64(&p.failedExecutions, 1)
				return
			}

			// 用重试逻辑执行
			toolResult := p.executeWithRetry(execCtx, c)
			result.Results[idx] = toolResult

			if toolResult.Error != "" {
				atomic.AddInt64(&p.failedExecutions, 1)
				if p.config.FailFast && firstError.CompareAndSwap(nil, toolResult.Error) {
					p.logger.Warn("fail-fast triggered", zap.String("tool", c.Name), zap.String("error", toolResult.Error))
					if failFastCancel != nil {
						failFastCancel()
					}
				}
			} else {
				atomic.AddInt64(&p.successExecutions, 1)
			}
		}(i, call)
	}

	wg.Wait()

	// 计算统计
	result.TotalDuration = time.Since(start)
	atomic.AddInt64(&p.totalExecutions, int64(len(calls)))
	atomic.AddInt64(&p.totalDuration, int64(result.TotalDuration))

	for _, r := range result.Results {
		if r.Error == "" {
			result.Completed++
		} else if r.Error == "execution cancelled before start" || r.Error == "context cancelled" {
			result.Cancelled++
		} else {
			result.Failed++
		}
	}

	result.PartialResult = result.Cancelled > 0 || result.Failed > 0

	p.logger.Info("parallel execution completed",
		zap.Int("total", len(calls)),
		zap.Int("completed", result.Completed),
		zap.Int("failed", result.Failed),
		zap.Int("cancelled", result.Cancelled),
		zap.Duration("duration", result.TotalDuration))

	return result
}

// 执行 With Retry 执行带有重试逻辑的单一工具调用 。
func (p *ParallelExecutor) executeWithRetry(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	var lastResult ToolResult
	maxAttempts := 1
	if p.config.RetryOnError {
		maxAttempts = p.config.MaxRetries + 1
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ToolResult{
					ToolCallID: call.ID,
					Name:       call.Name,
					Error:      "context cancelled during retry",
				}
			case <-time.After(p.config.RetryDelay):
			}
			p.logger.Debug("retrying tool execution",
				zap.String("tool", call.Name),
				zap.Int("attempt", attempt+1))
		}

		lastResult = p.executeSingle(ctx, call)
		if lastResult.Error == "" {
			return lastResult
		}

		// 不要重试某些错误
		if !p.isRetryableError(lastResult.Error) {
			break
		}
	}

	return lastResult
}

// 执行 Single 执行单个工具调用 。
func (p *ParallelExecutor) executeSingle(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	start := time.Now()
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	// 执行前检查上下文
	select {
	case <-ctx.Done():
		result.Error = "context cancelled"
		result.Duration = time.Since(start)
		return result
	default:
	}

	// 获取工具函数
	fn, meta, err := p.registry.Get(call.Name)
	if err != nil {
		result.Error = fmt.Sprintf("tool not found: %s", err.Error())
		result.Duration = time.Since(start)
		return result
	}

	// 检查率限制
	if reg, ok := p.registry.(*DefaultRegistry); ok {
		if err := reg.checkRateLimit(call.Name); err != nil {
			result.Error = fmt.Sprintf("rate limit exceeded: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}

	// 验证参数
	if len(call.Arguments) > 0 {
		var tmp any
		if err := json.Unmarshal(call.Arguments, &tmp); err != nil {
			result.Error = fmt.Sprintf("invalid arguments: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}

	// 以超时执行
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
	case err := <-errChan:
		result.Error = err.Error()
		result.Duration = time.Since(start)
	case <-execCtx.Done():
		result.Error = fmt.Sprintf("execution timeout after %s", meta.Timeout)
		result.Duration = time.Since(start)
	}

	return result
}

// 可重试错误决定是否触发重试 。
func (p *ParallelExecutor) isRetryableError(errMsg string) bool {
	// 不要重试验证错误或发现错误
	nonRetryable := []string{
		"tool not found",
		"invalid arguments",
		"rate limit exceeded",
	}
	for _, s := range nonRetryable {
		if len(errMsg) >= len(s) && errMsg[:len(s)] == s {
			return false
		}
	}
	return true
}

// Stats 返回执行统计 。
func (p *ParallelExecutor) Stats() (total, success, failed int64, avgDuration time.Duration) {
	total = atomic.LoadInt64(&p.totalExecutions)
	success = atomic.LoadInt64(&p.successExecutions)
	failed = atomic.LoadInt64(&p.failedExecutions)
	totalDur := atomic.LoadInt64(&p.totalDuration)
	if total > 0 {
		avgDuration = time.Duration(totalDur / total)
	}
	return
}

// 执行与依赖关系执行工具 。
// 依赖性被指定为工具调用ID,在调用之前必须完成.
type ToolCallWithDeps struct {
	Call         llmpkg.ToolCall                                     `json:"call"`
	DependsOn    []string                                            `json:"depends_on,omitempty"` // IDs of tool calls that must complete first
	ResultMapper func(results map[string]ToolResult) json.RawMessage `json:"-"`                    // Optional: modify args based on deps
}

// 执行与相互依存执行工具调用尊重依赖命令 。
func (p *ParallelExecutor) ExecuteWithDependencies(ctx context.Context, calls []ToolCallWithDeps) *ParallelResult {
	start := time.Now()
	result := &ParallelResult{
		Results: make([]ToolResult, len(calls)),
	}

	if len(calls) == 0 {
		result.TotalDuration = time.Since(start)
		return result
	}

	execCtx, cancel := context.WithTimeout(ctx, p.config.ExecutionTimeout)
	defer cancel()

	// 构建依赖图
	callIndex := make(map[string]int)
	for i, c := range calls {
		callIndex[c.Call.ID] = i
	}

	// 跟踪完成的成果
	var mu sync.Mutex
	completedResults := make(map[string]ToolResult)
	completed := make(map[string]chan struct{})
	for _, c := range calls {
		completed[c.Call.ID] = make(chan struct{})
	}

	sem := make(chan struct{}, p.config.MaxConcurrency)
	var wg sync.WaitGroup

	for i, callWithDeps := range calls {
		wg.Add(1)
		go func(idx int, cwd ToolCallWithDeps) {
			defer wg.Done()

			// 等待依赖关系
			for _, depID := range cwd.DependsOn {
				if ch, ok := completed[depID]; ok {
					select {
					case <-ch:
					case <-execCtx.Done():
						result.Results[idx] = ToolResult{
							ToolCallID: cwd.Call.ID,
							Name:       cwd.Call.Name,
							Error:      "context cancelled waiting for dependencies",
						}
						return
					}
				}
			}

			// 如果提供了结果映射器, 则应用
			call := cwd.Call
			if cwd.ResultMapper != nil {
				mu.Lock()
				newArgs := cwd.ResultMapper(completedResults)
				mu.Unlock()
				if newArgs != nil {
					call.Arguments = newArgs
				}
			}

			// 获取分母
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-execCtx.Done():
				result.Results[idx] = ToolResult{
					ToolCallID: call.ID,
					Name:       call.Name,
					Error:      "context cancelled before execution",
				}
				return
			}

			// 执行
			toolResult := p.executeWithRetry(execCtx, call)
			result.Results[idx] = toolResult

			// 标为已完成
			mu.Lock()
			completedResults[call.ID] = toolResult
			mu.Unlock()
			close(completed[call.ID])

		}(i, callWithDeps)
	}

	wg.Wait()

	result.TotalDuration = time.Since(start)
	for _, r := range result.Results {
		if r.Error == "" {
			result.Completed++
		} else {
			result.Failed++
		}
	}
	result.PartialResult = result.Failed > 0

	return result
}

// 批量执行器提供批量执行,并自动分批处理类似工具调用.
type BatchExecutor struct {
	parallel *ParallelExecutor
	logger   *zap.Logger
}

// NewBatch 执行器创建批次执行器 。
func NewBatchExecutor(parallel *ParallelExecutor, logger *zap.Logger) *BatchExecutor {
	return &BatchExecutor{
		parallel: parallel,
		logger:   logger,
	}
}

// 执行Batched类类似工具调用并高效地执行.
func (b *BatchExecutor) ExecuteBatched(ctx context.Context, calls []llmpkg.ToolCall) *ParallelResult {
	// 按工具名分组调用,以便进行可能的分批优化
	groups := make(map[string][]int)
	for i, call := range calls {
		groups[call.Name] = append(groups[call.Name], i)
	}

	b.logger.Debug("batched execution",
		zap.Int("total_calls", len(calls)),
		zap.Int("unique_tools", len(groups)))

	// 现在,代表 平行执行者
	// 未来:对辅助工具进行实际分批
	return b.parallel.Execute(ctx, calls)
}
