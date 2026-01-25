package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	llmpkg "github.com/yourusername/agentflow/llm"

	"go.uber.org/zap"
)

// FallbackStrategy 回退策略
type FallbackStrategy string

const (
	// FallbackRetry 重试当前工具
	FallbackRetry FallbackStrategy = "retry"
	// FallbackAlternate 切换到备用工具
	FallbackAlternate FallbackStrategy = "alternate"
	// FallbackSkip 跳过工具调用，让模型直接回答
	FallbackSkip FallbackStrategy = "skip"
	// FallbackError 返回错误
	FallbackError FallbackStrategy = "error"
)

// FallbackConfig 回退配置
type FallbackConfig struct {
	MaxRetries      int               `json:"max_retries"`
	RetryDelayMs    int               `json:"retry_delay_ms"`
	Alternates      map[string]string `json:"alternates"`     // 工具名 -> 备用工具名
	SkipOnErrors    []string          `json:"skip_on_errors"` // 遇到这些错误时跳过
	DefaultStrategy FallbackStrategy  `json:"default_strategy"`
}

// DefaultFallbackConfig 默认回退配置
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		MaxRetries:      2,
		RetryDelayMs:    500,
		Alternates:      make(map[string]string),
		SkipOnErrors:    []string{"tool not found", "rate limit exceeded"},
		DefaultStrategy: FallbackRetry,
	}
}

// ResilientExecutor 具有回退能力的工具执行器
type ResilientExecutor struct {
	registry ToolRegistry
	config   *FallbackConfig
	logger   *zap.Logger
}

// NewResilientExecutor 创建具有回退能力的执行器
func NewResilientExecutor(registry ToolRegistry, config *FallbackConfig, logger *zap.Logger) *ResilientExecutor {
	if config == nil {
		config = DefaultFallbackConfig()
	}
	return &ResilientExecutor{
		registry: registry,
		config:   config,
		logger:   logger,
	}
}

// Execute 执行工具调用（带回退）
func (e *ResilientExecutor) Execute(ctx context.Context, calls []llmpkg.ToolCall) []ToolResult {
	results := make([]ToolResult, len(calls))
	for i, call := range calls {
		results[i] = e.executeWithFallback(ctx, call)
	}
	return results
}

// ExecuteOne 执行单个工具调用（带回退）
func (e *ResilientExecutor) ExecuteOne(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	return e.executeWithFallback(ctx, call)
}

func (e *ResilientExecutor) executeWithFallback(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	start := time.Now()
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	// 尝试执行
	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		execResult := e.tryExecute(ctx, call)

		if execResult.Error == "" {
			// 成功
			execResult.Duration = time.Since(start)
			return execResult
		}

		// 判断回退策略
		strategy := e.determineStrategy(call.Name, execResult.Error)
		e.logger.Warn("tool execution failed",
			zap.String("tool", call.Name),
			zap.Int("attempt", attempt),
			zap.String("error", execResult.Error),
			zap.String("strategy", string(strategy)))

		switch strategy {
		case FallbackRetry:
			if attempt < e.config.MaxRetries {
				time.Sleep(time.Duration(e.config.RetryDelayMs) * time.Millisecond)
				continue
			}
			// 重试次数用尽，尝试备用工具
			if alt := e.getAlternate(call.Name); alt != "" {
				return e.executeAlternate(ctx, call, alt, start)
			}
			result = execResult

		case FallbackAlternate:
			if alt := e.getAlternate(call.Name); alt != "" {
				return e.executeAlternate(ctx, call, alt, start)
			}
			result = execResult

		case FallbackSkip:
			result.Result = e.buildSkipResponse(call.Name, execResult.Error)
			result.Duration = time.Since(start)
			return result

		case FallbackError:
			result = execResult
		}
		break
	}

	result.Duration = time.Since(start)
	return result
}

func (e *ResilientExecutor) tryExecute(ctx context.Context, call llmpkg.ToolCall) ToolResult {
	result := ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
	}

	// 获取工具
	fn, meta, err := e.registry.Get(call.Name)
	if err != nil {
		result.Error = fmt.Sprintf("tool not found: %s", err.Error())
		return result
	}

	// 检查速率限制
	if reg, ok := e.registry.(*DefaultRegistry); ok {
		if err := reg.checkRateLimit(call.Name); err != nil {
			result.Error = fmt.Sprintf("rate limit exceeded: %s", err.Error())
			return result
		}
	}

	// 执行（带超时）
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
	case err := <-errChan:
		result.Error = err.Error()
	case <-execCtx.Done():
		result.Error = fmt.Sprintf("timeout after %s", meta.Timeout)
	}

	return result
}

func (e *ResilientExecutor) determineStrategy(toolName, errMsg string) FallbackStrategy {
	// 检查是否应该跳过
	for _, skipErr := range e.config.SkipOnErrors {
		if contains(errMsg, skipErr) {
			return FallbackSkip
		}
	}

	// 检查是否有备用工具
	if _, ok := e.config.Alternates[toolName]; ok {
		return FallbackAlternate
	}

	return e.config.DefaultStrategy
}

func (e *ResilientExecutor) getAlternate(toolName string) string {
	if alt, ok := e.config.Alternates[toolName]; ok {
		if e.registry.Has(alt) {
			return alt
		}
	}
	return ""
}

func (e *ResilientExecutor) executeAlternate(ctx context.Context, original llmpkg.ToolCall, altName string, start time.Time) ToolResult {
	e.logger.Info("switching to alternate tool",
		zap.String("original", original.Name),
		zap.String("alternate", altName))

	altCall := llmpkg.ToolCall{
		ID:        original.ID,
		Name:      altName,
		Arguments: original.Arguments,
	}

	result := e.tryExecute(ctx, altCall)
	result.Name = original.Name // 保持原始工具名
	result.Duration = time.Since(start)

	if result.Error != "" {
		result.Error = fmt.Sprintf("alternate tool %s also failed: %s", altName, result.Error)
	}

	return result
}

func (e *ResilientExecutor) buildSkipResponse(toolName, errMsg string) json.RawMessage {
	resp := map[string]interface{}{
		"skipped": true,
		"tool":    toolName,
		"reason":  errMsg,
		"message": "Tool execution was skipped due to an error. Please provide a response without using this tool.",
	}
	data, _ := json.Marshal(resp)
	return data
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ToolCallChain 工具调用链（支持多步骤工具调用）
type ToolCallChain struct {
	executor *ResilientExecutor
	logger   *zap.Logger
}

// NewToolCallChain 创建工具调用链
func NewToolCallChain(executor *ResilientExecutor, logger *zap.Logger) *ToolCallChain {
	return &ToolCallChain{
		executor: executor,
		logger:   logger,
	}
}

// ExecuteChain 执行工具调用链
// 支持工具之间的依赖关系，前一个工具的输出可以作为后一个工具的输入
func (c *ToolCallChain) ExecuteChain(ctx context.Context, calls []llmpkg.ToolCall) ([]ToolResult, error) {
	results := make([]ToolResult, 0, len(calls))
	context := make(map[string]json.RawMessage) // 存储中间结果

	for _, call := range calls {
		// 替换参数中的引用
		args := c.resolveReferences(call.Arguments, context)
		call.Arguments = args

		result := c.executor.ExecuteOne(ctx, call)
		results = append(results, result)

		if result.Error != "" {
			c.logger.Warn("chain execution stopped due to error",
				zap.String("tool", call.Name),
				zap.String("error", result.Error))
			break
		}

		// 存储结果供后续工具使用
		context[call.ID] = result.Result
	}

	return results, nil
}

// resolveReferences 解析参数中的引用（如 ${tool_call_id.field}）
func (c *ToolCallChain) resolveReferences(args json.RawMessage, context map[string]json.RawMessage) json.RawMessage {
	if len(args) == 0 || len(context) == 0 {
		return args
	}

	var root any
	if err := json.Unmarshal(args, &root); err != nil {
		return args
	}

	// 缓存 tool_call_id -> 解析后的 JSON（避免重复 Unmarshal）
	parsed := make(map[string]any, len(context))
	resolveRef := func(expr string) (any, bool) {
		expr = strings.TrimSpace(expr)
		if expr == "" {
			return nil, false
		}
		callID := expr
		path := ""
		if idx := strings.IndexByte(expr, '.'); idx >= 0 {
			callID = strings.TrimSpace(expr[:idx])
			path = strings.TrimSpace(expr[idx+1:])
		}
		raw, ok := context[callID]
		if !ok {
			return nil, false
		}
		v, ok := parsed[callID]
		if !ok {
			var tmp any
			if err := json.Unmarshal(raw, &tmp); err != nil {
				return nil, false
			}
			parsed[callID] = tmp
			v = tmp
		}
		if path == "" {
			return v, true
		}
		val, found := resolvePath(v, path)
		return val, found
	}

	changed := false
	var replace func(v any) any
	replace = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			for k, vv := range t {
				nv := replace(vv)
				if nv != vv {
					t[k] = nv
					changed = true
				}
			}
			return t
		case []any:
			for i := range t {
				nv := replace(t[i])
				if nv != t[i] {
					t[i] = nv
					changed = true
				}
			}
			return t
		case string:
			nv, ok := replacePlaceholders(t, resolveRef)
			if ok {
				changed = true
				return nv
			}
			return t
		default:
			return v
		}
	}

	root = replace(root)
	if !changed {
		return args
	}
	out, err := json.Marshal(root)
	if err != nil {
		return args
	}
	return out
}

var rePlaceholder = regexp.MustCompile(`\$\{([^}]+)\}`)

func replacePlaceholders(s string, resolve func(expr string) (any, bool)) (any, bool) {
	matches := rePlaceholder.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s, false
	}

	// 纯占位符：允许替换为非字符串类型（对象/数组/数字等）
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(s) {
		expr := s[matches[0][2]:matches[0][3]]
		if v, ok := resolve(expr); ok {
			return v, true
		}
		return s, false
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, m := range matches {
		if m[0] < last {
			continue
		}
		b.WriteString(s[last:m[0]])
		expr := s[m[2]:m[3]]
		if v, ok := resolve(expr); ok {
			changed = true
			switch vv := v.(type) {
			case string:
				b.WriteString(vv)
			default:
				if data, err := json.Marshal(vv); err == nil {
					b.WriteString(string(data))
				} else {
					b.WriteString(fmt.Sprint(vv))
				}
			}
		} else {
			b.WriteString(s[m[0]:m[1]])
		}
		last = m[1]
	}
	b.WriteString(s[last:])
	if !changed {
		return s, false
	}
	return b.String(), true
}

func resolvePath(root any, path string) (any, bool) {
	cur := root
	parts := strings.Split(path, ".")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		switch v := cur.(type) {
		case map[string]any:
			nv, ok := v[part]
			if !ok {
				return nil, false
			}
			cur = nv
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}
