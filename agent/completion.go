package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

// ChatCompletion 调用 LLM 完成对话
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []types.Message) (*llm.ChatResponse, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}
	chatProvider := b.gatewayProvider()

	// 上下文工程：优化消息历史
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := ""
		// 提取用户查询（最后一条用户消息）
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			tokensBefore := b.contextManager.EstimateTokens(messages)
			tokensAfter := b.contextManager.EstimateTokens(optimized)
			if tokensBefore != tokensAfter {
				b.logger.Debug("context optimized",
					zap.Int("tokens_before", tokensBefore),
					zap.Int("tokens_after", tokensAfter))
			}
			messages = optimized
		}
	}

	model := b.config.LLM.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.LLM.MaxTokens,
		Temperature: b.config.LLM.Temperature,
	}

	// 运行时配置覆盖：从 context 获取 RunConfig 并应用到请求
	rc := GetRunConfig(ctx)
	if rc != nil {
		rc.ApplyToRequest(req, b.config)
	}

	// 按白名单过滤可用工具
	if b.toolManager != nil && len(b.config.Runtime.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(b.toolManager.GetAllowedTools(b.config.Core.ID), b.config.Runtime.Tools)
	}
	if len(req.Tools) > 0 {
		// 确定实际用于工具调用的 provider
		effectiveToolProvider := chatProvider
		if b.toolProvider != nil {
			effectiveToolProvider = b.gatewayToolProvider()
		}
		if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
			return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
		}
	}

	// 计算有效的 ReAct 迭代次数（RunConfig 可覆盖）
	effectiveReActIterations := rc.EffectiveMaxReActIterations(b.maxReActIterations())

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		// 若存在可用工具：使用流式 ReAct 循环，并将 token/tool 事件发射给上游（RunStream/Workflow）。
		if len(req.Tools) > 0 && b.toolManager != nil {
			// 双模型模式：ReAct 循环优先使用 toolProvider（便宜），未设置时退化为主 provider
			reactProvider := chatProvider
			if b.toolProvider != nil {
				reactProvider = b.gatewayToolProvider()
			}
			reactReq := *req
			reactReq.Model = effectiveToolModel(req.Model, b.config.Runtime.ToolModel)
			executor := llmtools.NewReActExecutor(reactProvider, newToolManagerExecutor(b.toolManager, b.config.Core.ID, b.config.Runtime.Tools, b.bus), llmtools.ReActConfig{
				MaxIterations: effectiveReActIterations,
				StopOnError:   false,
			}, b.logger)

			evCh, err := executor.ExecuteStream(ctx, &reactReq)
			if err != nil {
				return nil, err
			}
			var final *llm.ChatResponse
			for ev := range evCh {
				switch ev.Type {
				case llmtools.ReActEventLLMChunk:
					if ev.Chunk != nil && ev.Chunk.Delta.Content != "" {
						emit(RuntimeStreamEvent{
							Type:      RuntimeStreamToken,
							Timestamp: time.Now(),
							Token:     ev.Chunk.Delta.Content,
							Delta:     ev.Chunk.Delta.Content,
						})
					}
				case llmtools.ReActEventToolsStart:
					for _, call := range ev.ToolCalls {
						emit(RuntimeStreamEvent{
							Type:      RuntimeStreamToolCall,
							Timestamp: time.Now(),
							ToolCall: &RuntimeToolCall{
								ID:        call.ID,
								Name:      call.Name,
								Arguments: append(json.RawMessage(nil), call.Arguments...),
							},
						})
					}
				case llmtools.ReActEventToolsEnd:
					for _, tr := range ev.ToolResults {
						emit(RuntimeStreamEvent{
							Type:      RuntimeStreamToolResult,
							Timestamp: time.Now(),
							ToolResult: &RuntimeToolResult{
								ToolCallID: tr.ToolCallID,
								Name:       tr.Name,
								Result:     append(json.RawMessage(nil), tr.Result...),
								Error:      tr.Error,
								Duration:   tr.Duration,
							},
						})
					}
				case llmtools.ReActEventToolProgress:
					emit(RuntimeStreamEvent{
						Type:       RuntimeStreamToolProgress,
						Timestamp:  time.Now(),
						ToolCallID: ev.ToolCallID,
						ToolName:   ev.ToolName,
						Data:       ev.ProgressData,
					})
				case llmtools.ReActEventCompleted:
					final = ev.FinalResponse
				case llmtools.ReActEventError:
					return nil, fmt.Errorf("%s", ev.Error)
				}
			}
			if final == nil {
				return nil, fmt.Errorf("no final response")
			}
			return final, nil
		}

		// 无工具：直接走 provider stream，并将 token 发射给上游，同时组装最终响应。
		streamCh, err := chatProvider.Stream(ctx, req)
		if err != nil {
			return nil, err
		}
		var (
			assembled types.Message
			lastID    string
			lastProv  string
			lastModel string
			lastUsage *llm.ChatUsage
			lastFR    string
		)
		for chunk := range streamCh {
			if chunk.Err != nil {
				return nil, chunk.Err
			}
			if chunk.ID != "" {
				lastID = chunk.ID
			}
			if chunk.Provider != "" {
				lastProv = chunk.Provider
			}
			if chunk.Model != "" {
				lastModel = chunk.Model
			}
			if chunk.Usage != nil {
				lastUsage = chunk.Usage
			}
			if chunk.FinishReason != "" {
				lastFR = chunk.FinishReason
			}
			if chunk.Delta.Content != "" {
				emit(RuntimeStreamEvent{
					Type:      RuntimeStreamToken,
					Timestamp: time.Now(),
					Token:     chunk.Delta.Content,
					Delta:     chunk.Delta.Content,
				})
				assembled.Content += chunk.Delta.Content
			}
		}
		assembled.Role = llm.RoleAssistant
		resp := &llm.ChatResponse{
			ID:       lastID,
			Provider: lastProv,
			Model:    lastModel,
			Choices: []llm.ChatChoice{{
				Index:        0,
				FinishReason: lastFR,
				Message:      assembled,
			}},
		}
		if lastUsage != nil {
			resp.Usage = *lastUsage
		}
		return resp, nil
	}

	// 若存在可用工具：执行 ReAct 循环（LLM -> Tool -> LLM），直到模型停止调用工具或达到最大迭代次数。
	if len(req.Tools) > 0 && b.toolManager != nil {
		// 双模型模式：ReAct 循环优先使用 toolProvider（便宜），未设置时退化为主 provider
		reactProvider := chatProvider
		if b.toolProvider != nil {
			reactProvider = b.gatewayToolProvider()
		}
		reactReq := *req
		reactReq.Model = effectiveToolModel(req.Model, b.config.Runtime.ToolModel)
		reactExecutor := llmtools.NewReActExecutor(reactProvider, newToolManagerExecutor(b.toolManager, b.config.Core.ID, b.config.Runtime.Tools, b.bus), llmtools.ReActConfig{
			MaxIterations: effectiveReActIterations,
			StopOnError:   false,
		}, b.logger)

		resp, _, err := reactExecutor.Execute(ctx, &reactReq)
		return resp, err
	}

	return chatProvider.Completion(ctx, req)
}

// StreamCompletion 流式调用 LLM
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []types.Message) (<-chan llm.StreamChunk, error) {
	if b.provider == nil {
		return nil, ErrProviderNotSet
	}
	chatProvider := b.gatewayProvider()

	// 上下文工程：优化消息历史
	if b.contextEngineEnabled && b.contextManager != nil && len(messages) > 1 {
		query := ""
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				query = messages[i].Content
				break
			}
		}
		optimized, err := b.contextManager.PrepareMessages(ctx, messages, query)
		if err != nil {
			b.logger.Warn("context optimization failed, using original messages", zap.Error(err))
		} else {
			messages = optimized
		}
	}

	model := b.config.LLM.Model
	if v, ok := types.LLMModel(ctx); ok && strings.TrimSpace(v) != "" {
		model = strings.TrimSpace(v)
	}

	req := &llm.ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   b.config.LLM.MaxTokens,
		Temperature: b.config.LLM.Temperature,
	}

	// 运行时配置覆盖：从 context 获取 RunConfig 并应用到请求
	if rc := GetRunConfig(ctx); rc != nil {
		rc.ApplyToRequest(req, b.config)
	}

	if b.toolManager != nil && len(b.config.Runtime.Tools) > 0 {
		req.Tools = filterToolSchemasByWhitelist(b.toolManager.GetAllowedTools(b.config.Core.ID), b.config.Runtime.Tools)
	}
	if len(req.Tools) > 0 {
		// 确定实际用于工具调用的 provider（与 ChatCompletion 保持一致）
		effectiveToolProvider := chatProvider
		if b.toolProvider != nil {
			effectiveToolProvider = b.gatewayToolProvider()
		}
		if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
			return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
		}
	}

	return chatProvider.Stream(ctx, req)
}

func effectiveToolModel(mainModel string, configuredToolModel string) string {
	if v := strings.TrimSpace(configuredToolModel); v != "" {
		return v
	}
	return mainModel
}

// =============================================================================
// RunConfig (merged from run_config.go)
// =============================================================================

// runConfigKey is the unexported context key for RunConfig.
type runConfigKey struct{}

// RunConfig provides runtime overrides for Agent execution.
// All pointer fields use nil to indicate "no override" — only non-nil values
// are applied, leaving the base Config defaults intact.
type RunConfig struct {
	Model              *string           `json:"model,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	MaxTokens          *int              `json:"max_tokens,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	Stop               []string          `json:"stop,omitempty"`
	ToolChoice         *string           `json:"tool_choice,omitempty"`
	Timeout            *time.Duration    `json:"timeout,omitempty"`
	MaxReActIterations *int              `json:"max_react_iterations,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
}

// WithRunConfig stores a RunConfig in the context.
func WithRunConfig(ctx context.Context, rc *RunConfig) context.Context {
	return context.WithValue(ctx, runConfigKey{}, rc)
}

// GetRunConfig retrieves the RunConfig from the context.
// Returns nil if no RunConfig is present.
func GetRunConfig(ctx context.Context) *RunConfig {
	rc, _ := ctx.Value(runConfigKey{}).(*RunConfig)
	return rc
}

// ApplyToRequest applies RunConfig overrides to a ChatRequest.
// Fields in baseCfg are used as defaults; only non-nil RunConfig fields override them.
// If rc is nil, this is a no-op.
func (rc *RunConfig) ApplyToRequest(req *llm.ChatRequest, baseCfg types.AgentConfig) {
	if rc == nil || req == nil {
		return
	}

	if rc.Model != nil {
		req.Model = *rc.Model
	}
	if rc.Temperature != nil {
		req.Temperature = *rc.Temperature
	}
	if rc.MaxTokens != nil {
		req.MaxTokens = *rc.MaxTokens
	}
	if rc.TopP != nil {
		req.TopP = *rc.TopP
	}
	if len(rc.Stop) > 0 {
		req.Stop = rc.Stop
	}
	if rc.ToolChoice != nil {
		req.ToolChoice = *rc.ToolChoice
	}
	if rc.Timeout != nil {
		req.Timeout = *rc.Timeout
	}
	if len(rc.Metadata) > 0 {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, len(rc.Metadata))
		}
		for k, v := range rc.Metadata {
			req.Metadata[k] = v
		}
	}
	if len(rc.Tags) > 0 {
		req.Tags = rc.Tags
	}
}

// EffectiveMaxReActIterations returns the RunConfig override if set,
// otherwise falls back to defaultVal.
func (rc *RunConfig) EffectiveMaxReActIterations(defaultVal int) int {
	if rc != nil && rc.MaxReActIterations != nil {
		return *rc.MaxReActIterations
	}
	return defaultVal
}

// --- Pointer helper functions ---

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string { return &s }

// Float32Ptr returns a pointer to the given float32.
func Float32Ptr(f float32) *float32 { return &f }

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int { return &i }

// DurationPtr returns a pointer to the given time.Duration.
func DurationPtr(d time.Duration) *time.Duration { return &d }

// =============================================================================
// RuntimeStream (merged from runtime_stream.go)
// =============================================================================

type runtimeStreamEmitterKey struct{}

type RuntimeStreamEventType string

const (
	RuntimeStreamToken        RuntimeStreamEventType = "token"
	RuntimeStreamToolCall     RuntimeStreamEventType = "tool_call"
	RuntimeStreamToolResult   RuntimeStreamEventType = "tool_result"
	RuntimeStreamToolProgress RuntimeStreamEventType = "tool_progress"
)

type RuntimeToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type RuntimeToolResult struct {
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration,omitempty"`
}

type RuntimeStreamEvent struct {
	Type       RuntimeStreamEventType `json:"type"`
	Timestamp  time.Time              `json:"timestamp"`
	Token      string                 `json:"token,omitempty"`
	Delta      string                 `json:"delta,omitempty"`
	ToolCall   *RuntimeToolCall       `json:"tool_call,omitempty"`
	ToolResult *RuntimeToolResult     `json:"tool_result,omitempty"`
	ToolCallID string                 `json:"tool_call_id,omitempty"`
	ToolName   string                 `json:"tool_name,omitempty"`
	Data       any                    `json:"data,omitempty"`
}

type RuntimeStreamEmitter func(RuntimeStreamEvent)

func WithRuntimeStreamEmitter(ctx context.Context, emit RuntimeStreamEmitter) context.Context {
	if emit == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeStreamEmitterKey{}, emit)
}

func runtimeStreamEmitterFromContext(ctx context.Context) (RuntimeStreamEmitter, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(runtimeStreamEmitterKey{})
	if v == nil {
		return nil, false
	}
	emit, ok := v.(RuntimeStreamEmitter)
	return emit, ok && emit != nil
}
