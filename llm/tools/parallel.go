// Package tools provides tool execution capabilities for LLM agents.
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

// ParallelConfig defines configuration for parallel tool execution.
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

// DefaultParallelConfig returns sensible defaults for parallel execution.
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

// ParallelExecutor executes multiple tool calls concurrently with advanced features.
type ParallelExecutor struct {
	registry ToolRegistry
	config   ParallelConfig
	logger   *zap.Logger

	// Metrics
	totalExecutions   int64
	successExecutions int64
	failedExecutions  int64
	totalDuration     int64 // nanoseconds
}

// NewParallelExecutor creates a new parallel tool executor.
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

// ParallelResult contains results from parallel tool execution.
type ParallelResult struct {
	Results       []ToolResult  `json:"results"`
	TotalDuration time.Duration `json:"total_duration"`
	Completed     int           `json:"completed"`
	Failed        int           `json:"failed"`
	Cancelled     int           `json:"cancelled"`
	PartialResult bool          `json:"partial_result"`
}

// Execute runs multiple tool calls in parallel with concurrency control.
func (p *ParallelExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) *ParallelResult {
	start := time.Now()
	result := &ParallelResult{
		Results: make([]ToolResult, len(calls)),
	}

	if len(calls) == 0 {
		result.TotalDuration = time.Since(start)
		return result
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, p.config.ExecutionTimeout)
	defer cancel()

	// Semaphore for concurrency control
	sem := make(chan struct{}, p.config.MaxConcurrency)

	// Channel for fail-fast cancellation
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

			// Acquire semaphore
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

			// Execute with retry logic
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

	// Calculate statistics
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

// executeWithRetry executes a single tool call with retry logic.
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

		// Don't retry on certain errors
		if !p.isRetryableError(lastResult.Error) {
			break
		}
	}

	return lastResult
}

// executeSingle executes a single tool call.
func (p *ParallelExecutor) executeSingle(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	start := time.Now()
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	// Check context before execution
	select {
	case <-ctx.Done():
		result.Error = "context cancelled"
		result.Duration = time.Since(start)
		return result
	default:
	}

	// Get tool function
	fn, meta, err := p.registry.Get(call.Name)
	if err != nil {
		result.Error = fmt.Sprintf("tool not found: %s", err.Error())
		result.Duration = time.Since(start)
		return result
	}

	// Check rate limit
	if reg, ok := p.registry.(*DefaultRegistry); ok {
		if err := reg.checkRateLimit(call.Name); err != nil {
			result.Error = fmt.Sprintf("rate limit exceeded: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}

	// Validate arguments
	if len(call.Arguments) > 0 {
		var tmp interface{}
		if err := json.Unmarshal(call.Arguments, &tmp); err != nil {
			result.Error = fmt.Sprintf("invalid arguments: %s", err.Error())
			result.Duration = time.Since(start)
			return result
		}
	}

	// Execute with timeout
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

// isRetryableError determines if an error should trigger a retry.
func (p *ParallelExecutor) isRetryableError(errMsg string) bool {
	// Don't retry validation errors or not found errors
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

// Stats returns execution statistics.
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

// ExecuteWithDependencies executes tools respecting dependency order.
// Dependencies are specified as tool call IDs that must complete before this call.
type ToolCallWithDeps struct {
	Call         llmpkg.ToolCall                                     `json:"call"`
	DependsOn    []string                                            `json:"depends_on,omitempty"` // IDs of tool calls that must complete first
	ResultMapper func(results map[string]ToolResult) json.RawMessage `json:"-"`                    // Optional: modify args based on deps
}

// ExecuteWithDependencies executes tool calls respecting dependency order.
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

	// Build dependency graph
	callIndex := make(map[string]int)
	for i, c := range calls {
		callIndex[c.Call.ID] = i
	}

	// Track completed results
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

			// Wait for dependencies
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

			// Apply result mapper if provided
			call := cwd.Call
			if cwd.ResultMapper != nil {
				mu.Lock()
				newArgs := cwd.ResultMapper(completedResults)
				mu.Unlock()
				if newArgs != nil {
					call.Arguments = newArgs
				}
			}

			// Acquire semaphore
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

			// Execute
			toolResult := p.executeWithRetry(execCtx, call)
			result.Results[idx] = toolResult

			// Mark as completed
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

// BatchExecutor provides batch execution with automatic batching of similar tool calls.
type BatchExecutor struct {
	parallel *ParallelExecutor
	logger   *zap.Logger
}

// NewBatchExecutor creates a batch executor.
func NewBatchExecutor(parallel *ParallelExecutor, logger *zap.Logger) *BatchExecutor {
	return &BatchExecutor{
		parallel: parallel,
		logger:   logger,
	}
}

// ExecuteBatched groups similar tool calls and executes them efficiently.
func (b *BatchExecutor) ExecuteBatched(ctx context.Context, calls []llmpkg.ToolCall) *ParallelResult {
	// Group calls by tool name for potential batching optimization
	groups := make(map[string][]int)
	for i, call := range calls {
		groups[call.Name] = append(groups[call.Name], i)
	}

	b.logger.Debug("batched execution",
		zap.Int("total_calls", len(calls)),
		zap.Int("unique_tools", len(groups)))

	// For now, delegate to parallel executor
	// Future: implement actual batching for tools that support it
	return b.parallel.Execute(ctx, calls)
}
