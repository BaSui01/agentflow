package tools

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ReActConfig 定义了 ReAct 循环配置.
type ReActConfig struct {
	MaxIterations int  // Maximum iterations (prevents infinite loops)
	StopOnError   bool // Stop on tool execution error
}

// ReActExecutor 执行 ReAct（推理与行动）循环.
// 自动处理 LLM -> Tool -> LLM 多轮对话.
type ReActExecutor struct {
	provider     llm.Provider
	toolExecutor ToolExecutor
	logger       *zap.Logger
	config       ReActConfig
}

// NewReActExecutor 创建 ReAct 执行器.
func NewReActExecutor(provider llm.Provider, toolExecutor ToolExecutor, config ReActConfig, logger *zap.Logger) *ReActExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.MaxIterations == 0 {
		config.MaxIterations = 10
	}
	return &ReActExecutor{
		provider:     provider,
		toolExecutor: toolExecutor,
		logger:       logger,
		config:       config,
	}
}

// Execute 运行 ReAct 循环，返回最终响应和所有步骤.
func (r *ReActExecutor) Execute(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, []ReActStep, error) {
	steps := make([]ReActStep, 0)
	messages := append([]types.Message{}, req.Messages...)
	var lastResp *llm.ChatResponse // 保留最后一次有效响应

	for i := 0; i < r.config.MaxIterations; i++ {
		r.logger.Debug("ReAct iteration", zap.Int("iteration", i+1))

		callReq := *req
		callReq.Messages = messages
		resp, err := r.provider.Completion(ctx, &callReq)
		if err != nil {
			return lastResp, steps, fmt.Errorf("LLM call failed at iteration %d: %w", i+1, err)
		}
		lastResp = resp

		if len(resp.Choices) == 0 {
			return resp, steps, fmt.Errorf("no choices in LLM response")
		}

		choice := resp.Choices[0]
		toolCalls := choice.Message.ToolCalls

		step := ReActStep{
			StepNumber: i + 1,
			Thought:    choice.Message.Content,
			Timestamp:  fmt.Sprintf("%d", i+1),
			TokensUsed: resp.Usage.TotalTokens,
		}

		if len(toolCalls) == 0 {
			r.logger.Info("ReAct completed", zap.Int("iterations", i+1), zap.String("finish_reason", choice.FinishReason))
			steps = append(steps, step)
			return resp, steps, nil
		}

		r.logger.Info("executing tools", zap.Int("count", len(toolCalls)))
		step.Actions = toolCalls
		toolResults := r.toolExecutor.Execute(ctx, toolCalls)
		step.Observations = toolResults

		hasError := false
		for _, result := range toolResults {
			if result.IsError() {
				hasError = true
				r.logger.Warn("tool execution failed", zap.String("tool", result.Name), zap.String("error", result.Error))
			}
		}

		if hasError && r.config.StopOnError {
			steps = append(steps, step)
			return resp, steps, fmt.Errorf("tool execution failed, stopping ReAct loop")
		}

		messages = append(messages, choice.Message)
		for _, result := range toolResults {
			messages = append(messages, result.ToMessage())
		}
		steps = append(steps, step)
	}

	r.logger.Warn("ReAct max iterations reached", zap.Int("max", r.config.MaxIterations))
	return lastResp, steps, fmt.Errorf("max iterations reached (%d)", r.config.MaxIterations)
}

// ExecuteWithTrace 执行 ReAct 循环并返回完整跟踪信息.
func (r *ReActExecutor) ExecuteWithTrace(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, *ReActTrace, error) {
	resp, steps, err := r.Execute(ctx, req)

	trace := &ReActTrace{
		TraceID:    fmt.Sprintf("react-%d", len(steps)),
		Steps:      steps,
		TotalSteps: len(steps),
		Success:    err == nil,
	}

	for _, step := range steps {
		trace.TotalTokens += step.TokensUsed
	}

	if resp != nil && len(resp.Choices) > 0 {
		trace.FinalAnswer = resp.Choices[0].Message.Content
	}

	if err != nil {
		trace.ErrorMessage = err.Error()
	}

	return resp, trace, err
}

// ReActStep 表示 ReAct 循环（Thought -> Action -> Observation）的一步.
type ReActStep struct {
	StepNumber   int              `json:"step_number"`
	Thought      string           `json:"thought,omitempty"`
	Actions      []types.ToolCall   `json:"actions,omitempty"`
	Observations []types.ToolResult `json:"observations,omitempty"`
	Timestamp    string           `json:"timestamp"`
	TokensUsed   int              `json:"tokens_used,omitempty"`
}

// ReActTrace 表示完整的 ReAct 执行追踪.
type ReActTrace struct {
	TraceID      string      `json:"trace_id"`
	Steps        []ReActStep `json:"steps"`
	TotalTokens  int         `json:"total_tokens"`
	TotalSteps   int         `json:"total_steps"`
	Success      bool        `json:"success"`
	FinalAnswer  string      `json:"final_answer,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
}

// LLMCallInfo 记录 LLM 调用详情.
type LLMCallInfo struct {
	Request  llm.ChatRequest  `json:"request"`
	Response llm.ChatResponse `json:"response"`
}

// ExecuteStream 执行流式 ReAct 循环.
func (r *ReActExecutor) ExecuteStream(ctx context.Context, req *llm.ChatRequest) (<-chan ReActStreamEvent, error) {
	eventCh := make(chan ReActStreamEvent, 64) // 带缓冲防止消费者慢导致发送者阻塞

	go func() {
		defer close(eventCh)
		messages := append([]types.Message{}, req.Messages...)

		for i := 0; i < r.config.MaxIterations; i++ {
			select {
			case <-ctx.Done():
				eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("context cancelled: %v", ctx.Err())}
				return
			default:
			}

			eventCh <- ReActStreamEvent{Type: ReActEventIterationStart, Iteration: i + 1}

			callReq := *req
			callReq.Messages = messages

			streamCh, err := r.provider.Stream(ctx, &callReq)
			if err != nil {
				eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("LLM stream failed: %s", err.Error())}
				return
			}

			var (
				assembledMessage types.Message
				toolCallOrder    []string
				toolCallByID     map[string]*struct {
					id           string
					name         string
					argsFinal    json.RawMessage
					argsBuilding strings.Builder
				}
				lastChunkID, lastProvider, lastModel, lastFinishReason string
				lastUsage                                              *llm.ChatUsage
			)

			for chunk := range streamCh {
				select {
				case <-ctx.Done():
					eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("context cancelled: %v", ctx.Err())}
					return
				default:
				}

				eventCh <- ReActStreamEvent{Type: ReActEventLLMChunk, Chunk: &chunk}

				if chunk.Err != nil {
					eventCh <- ReActStreamEvent{Type: ReActEventError, Error: chunk.Err.Error()}
					return
				}

				if chunk.ID != "" {
					lastChunkID = chunk.ID
				}
				if chunk.Provider != "" {
					lastProvider = chunk.Provider
				}
				if chunk.Model != "" {
					lastModel = chunk.Model
				}
				if chunk.Usage != nil {
					lastUsage = chunk.Usage
				}
				if chunk.FinishReason != "" {
					lastFinishReason = chunk.FinishReason
				}

				if chunk.Delta.Content != "" {
					assembledMessage.Content += chunk.Delta.Content
				}
				if chunk.Delta.ReasoningContent != nil && *chunk.Delta.ReasoningContent != "" {
					if assembledMessage.ReasoningContent == nil {
						s := ""
						assembledMessage.ReasoningContent = &s
					}
					*assembledMessage.ReasoningContent += *chunk.Delta.ReasoningContent
				}
				if len(chunk.Delta.ToolCalls) > 0 {
					if toolCallByID == nil {
						toolCallByID = make(map[string]*struct {
							id           string
							name         string
							argsFinal    json.RawMessage
							argsBuilding strings.Builder
						})
					}
					for _, tc := range chunk.Delta.ToolCalls {
						// 用 index 作为聚合 key（OpenAI 流式协议：首 chunk 含 id/name，
						// 后续 chunk 同一 index 的 id/name 为空，只有 arguments 增量）
						key := fmt.Sprintf("idx_%d", tc.Index)
						acc := toolCallByID[key]
						if acc == nil {
							acc = &struct {
								id           string
								name         string
								argsFinal    json.RawMessage
								argsBuilding strings.Builder
							}{}
							toolCallByID[key] = acc
							toolCallOrder = append(toolCallOrder, key)
						}
						// 首 chunk 带 id，后续为空 — 只在非空时更新
						if strings.TrimSpace(tc.ID) != "" {
							acc.id = strings.TrimSpace(tc.ID)
						}
						if strings.TrimSpace(tc.Name) != "" {
							acc.name = strings.TrimSpace(tc.Name)
						}
						// 兜底：如果最后仍无 id，生成一个
						if acc.id == "" {
							acc.id = fmt.Sprintf("call_%d_%d", i+1, tc.Index+1)
						}
						if len(tc.Arguments) == 0 || len(acc.argsFinal) > 0 {
							continue
						}
						var argSegStr string
						if err := json.Unmarshal(tc.Arguments, &argSegStr); err == nil {
							acc.argsBuilding.WriteString(argSegStr)
							continue
						}
						if json.Valid(tc.Arguments) {
							acc.argsFinal = append([]byte(nil), tc.Arguments...)
							continue
						}
						acc.argsBuilding.WriteString(string(tc.Arguments))
					}
				}
			}

			assembledMessage.Role = llm.RoleAssistant
			nativeToolCalls := make([]types.ToolCall, 0, len(toolCallOrder))
			for _, id := range toolCallOrder {
				acc := toolCallByID[id]
				if acc == nil {
					continue
				}
				args := json.RawMessage(nil)
				if len(acc.argsFinal) > 0 {
					args = acc.argsFinal
				} else {
					raw := strings.TrimSpace(acc.argsBuilding.String())
					if raw != "" {
						if !json.Valid([]byte(raw)) {
							eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("invalid tool call arguments (id=%s tool=%s): %s", acc.id, acc.name, raw)}
							return
						}
						args = json.RawMessage(raw)
					}
				}
				nativeToolCalls = append(nativeToolCalls, types.ToolCall{ID: acc.id, Name: acc.name, Arguments: args})
			}
			assembledMessage.ToolCalls = nativeToolCalls

			if len(assembledMessage.ToolCalls) == 0 {
				final := &llm.ChatResponse{
					ID: lastChunkID, Provider: lastProvider, Model: lastModel,
					Choices: []llm.ChatChoice{{Index: 0, FinishReason: lastFinishReason, Message: assembledMessage}},
				}
				if lastUsage != nil {
					final.Usage = *lastUsage
				}
				eventCh <- ReActStreamEvent{Type: ReActEventCompleted, Iteration: i + 1, FinalResponse: final}
				return
			}

			eventCh <- ReActStreamEvent{Type: ReActEventToolsStart, ToolCalls: assembledMessage.ToolCalls}
			// 检查 toolExecutor 是否支持流式执行
			if streamExec, ok := r.toolExecutor.(StreamableToolExecutor); ok {
				toolResults := r.executeToolsWithStreaming(ctx, streamExec, assembledMessage.ToolCalls, eventCh)
				eventCh <- ReActStreamEvent{Type: ReActEventToolsEnd, ToolResults: toolResults}

				messages = append(messages, assembledMessage)
				for _, result := range toolResults {
					toolMessage := result.ToMessage()
					if result.Error != "" && r.config.StopOnError {
						eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("tool execution failed: %s", result.Error)}
						return
					}
					messages = append(messages, toolMessage)
				}
			} else {
				toolResults := r.toolExecutor.Execute(ctx, assembledMessage.ToolCalls)
				eventCh <- ReActStreamEvent{Type: ReActEventToolsEnd, ToolResults: toolResults}

				messages = append(messages, assembledMessage)
				for _, result := range toolResults {
					toolMessage := result.ToMessage()
					if result.Error != "" && r.config.StopOnError {
						eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("tool execution failed: %s", result.Error)}
						return
					}
					messages = append(messages, toolMessage)
				}
			}
		}

		eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("max iterations reached (%d)", r.config.MaxIterations)}
		return
	}()

	return eventCh, nil
}

// ReActStreamEvent type constants.
const (
	ReActEventIterationStart = "iteration_start"
	ReActEventLLMChunk       = "llm_chunk"
	ReActEventToolsStart     = "tools_start"
	ReActEventToolsEnd       = "tools_end"
	ReActEventToolProgress   = "tool_progress"
	ReActEventCompleted      = "completed"
	ReActEventError          = "error"
)

// ReActStreamEvent 表示流式 ReAct 循环事件.
type ReActStreamEvent struct {
	Type          string            `json:"type"`
	Iteration     int               `json:"iteration,omitempty"`
	Chunk         *llm.StreamChunk  `json:"chunk,omitempty"`
	ToolCalls     []types.ToolCall    `json:"tool_calls,omitempty"`
	ToolResults   []types.ToolResult  `json:"tool_results,omitempty"`
	ToolCallID    string            `json:"tool_call_id,omitempty"`
	ToolName      string            `json:"tool_name,omitempty"`
	ProgressData  any               `json:"progress_data,omitempty"`
	FinalResponse *llm.ChatResponse `json:"final_response,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// executeToolsWithStreaming 使用流式执行器逐个执行工具调用，
// 将中间 progress 事件作为 "tool_progress" 转发到 eventCh.
func (r *ReActExecutor) executeToolsWithStreaming(
	ctx context.Context,
	streamExec StreamableToolExecutor,
	calls []types.ToolCall,
	eventCh chan<- ReActStreamEvent,
) []types.ToolResult {
	results := make([]types.ToolResult, len(calls))

	for i, call := range calls {
		streamCh := streamExec.ExecuteOneStream(ctx, call)
		result := types.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
		}

		for event := range streamCh {
			switch event.Type {
			case ToolStreamProgress:
				// 转发中间进度事件
				select {
				case eventCh <- ReActStreamEvent{
					Type:         ReActEventToolProgress,
					ToolCallID:   call.ID,
					ToolName:     call.Name,
					ProgressData: event.Data,
				}:
				case <-ctx.Done():
					return results
				}
			case ToolStreamOutput:
				// output 事件的 Data 是 json.RawMessage
				if raw, ok := event.Data.(json.RawMessage); ok {
					result.Result = raw
				}
			case ToolStreamComplete:
				// complete 事件的 Data 是 types.ToolResult
				if tr, ok := event.Data.(types.ToolResult); ok {
					result = tr
				}
			case ToolStreamError:
				if event.Error != nil {
					result.Error = event.Error.Error()
				}
			}
		}

		results[i] = result
	}

	return results
}


