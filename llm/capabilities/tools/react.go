package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// DefaultInactivityTimeout 是流式响应的默认空闲超时时间.
// 只要还在收到数据，就不会超时；只有在超过此时间没有新数据时才触发超时.
const DefaultInactivityTimeout = 5 * time.Minute

// ReActConfig 定义了 ReAct 循环配置.
type ReActConfig struct {
	MaxIterations     int           // Maximum iterations (prevents infinite loops)
	StopOnError       bool          // Stop on tool execution error
	InactivityTimeout time.Duration // 流式响应空闲超时（从上一次收到数据开始计算），0 表示使用默认值 5 分钟
}

// SteeringMessage 类型别名，统一使用 types 包定义
type SteeringMessage = types.SteeringMessage

// ReActExecutor 执行 ReAct（推理与行动）循环.
// 自动处理 LLM -> Tool -> LLM 多轮对话.
type ReActExecutor struct {
	provider     llm.Provider
	toolExecutor ToolExecutor
	logger       *zap.Logger
	config       ReActConfig
	steeringCh   <-chan SteeringMessage // 可选：steering 消息接收端
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

// SetSteeringChannel 设置 steering 消息接收通道（可选，用于实时引导/停止后发送）
func (r *ReActExecutor) SetSteeringChannel(ch <-chan SteeringMessage) {
	r.steeringCh = ch
}

// steerChOrNil 返回 steering channel 或 nil（select 中永远不触发）
func (r *ReActExecutor) steerChOrNil() <-chan SteeringMessage {
	return r.steeringCh
}

// Execute 运行 ReAct 循环，返回最终响应和所有步骤.
func (r *ReActExecutor) Execute(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, []ReActStep, error) {
	steps := make([]ReActStep, 0)
	messages := append([]types.Message{}, req.Messages...)
	var lastResp *llm.ChatResponse // 保留最后一次有效响应
	var totalUsage llm.ChatUsage   // 累计所有迭代的 token 用量
	var prevPromptTokens int       // 上一轮的 PromptTokens，用于计算增量

	for i := 0; i < r.config.MaxIterations; i++ {
		r.logger.Debug("ReAct iteration", zap.Int("iteration", i+1))

		callReq := *req
		callReq.Messages = messages
		resp, err := r.provider.Completion(ctx, &callReq)
		if err != nil {
			return lastResp, steps, fmt.Errorf("LLM call failed at iteration %d: %w", i+1, err)
		}
		lastResp = resp

		// 计算增量 token：PromptTokens 包含历史消息，只有增量部分是本轮新增的
		promptDelta := resp.Usage.PromptTokens - prevPromptTokens
		if promptDelta < 0 {
			promptDelta = resp.Usage.PromptTokens
		}
		stepTokens := promptDelta + resp.Usage.CompletionTokens
		prevPromptTokens = resp.Usage.PromptTokens

		totalUsage.PromptTokens += promptDelta
		totalUsage.CompletionTokens += resp.Usage.CompletionTokens
		totalUsage.TotalTokens += stepTokens

		if len(resp.Choices) == 0 {
			return resp, steps, fmt.Errorf("no choices in LLM response")
		}

		choice := resp.Choices[0]
		toolCalls := choice.Message.ToolCalls

		step := ReActStep{
			StepNumber: i + 1,
			Thought:    choice.Message.Content,
			Timestamp:  fmt.Sprintf("%d", i+1),
			TokensUsed: stepTokens,
		}

		if len(toolCalls) == 0 {
			r.logger.Info("ReAct completed", zap.Int("iterations", i+1), zap.String("finish_reason", choice.FinishReason))
			steps = append(steps, step)
			resp.Usage = totalUsage
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
		if handoffResp, ok := synthesizeHandoffFinalResponse(resp, toolResults, totalUsage); ok {
			steps = append(steps, step)
			handoffResp.Usage = totalUsage
			return handoffResp, steps, nil
		}

		if hasError && r.config.StopOnError {
			steps = append(steps, step)
			resp.Usage = totalUsage
			return resp, steps, fmt.Errorf("tool execution failed, stopping ReAct loop")
		}

		messages = append(messages, choice.Message)
		for _, result := range toolResults {
			messages = append(messages, result.ToMessage())
		}
		steps = append(steps, step)
	}

	r.logger.Warn("ReAct max iterations reached", zap.Int("max", r.config.MaxIterations))
	if lastResp != nil {
		lastResp.Usage = totalUsage
	}
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
	StepNumber   int                `json:"step_number"`
	Thought      string             `json:"thought,omitempty"`
	Actions      []types.ToolCall   `json:"actions,omitempty"`
	Observations []types.ToolResult `json:"observations,omitempty"`
	Timestamp    string             `json:"timestamp"`
	TokensUsed   int                `json:"tokens_used,omitempty"`
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
// 支持 Steering：通过 SetSteeringChannel 设置的通道接收实时引导/停止后发送指令。
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

			// 迭代开始前检查是否有 pending steering
			select {
			case steerMsg := <-r.steerChOrNil():
				if newMsgs, ok := r.applySteering(steerMsg, messages, "", "", eventCh); ok {
					messages = newMsgs
					continue // 用新 messages 重新迭代
				}
			default:
			}

			eventCh <- ReActStreamEvent{Type: ReActEventIterationStart, Iteration: i + 1}

			callReq := *req
			callReq.Messages = messages

			streamCtx, cancelStream := context.WithCancel(ctx)
			streamCh, err := r.provider.Stream(streamCtx, &callReq)
			if err != nil {
				cancelStream()
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
				steering                                               *SteeringMessage
			)

			// 空闲超时机制：只要还在收到数据，就重置计时器；只有在超过 InactivityTimeout 没有新数据时才触发超时
			inactivityTimeout := r.config.InactivityTimeout
			if inactivityTimeout <= 0 {
				inactivityTimeout = DefaultInactivityTimeout
			}
			inactivityTimer := time.NewTimer(inactivityTimeout)
			defer inactivityTimer.Stop()

		chunkLoop:
			for {
				select {
				case chunk, ok := <-streamCh:
					if !ok {
						break chunkLoop
					}

					// 收到数据，重置空闲超时计时器
					if !inactivityTimer.Stop() {
						select {
						case <-inactivityTimer.C:
						default:
						}
					}
					inactivityTimer.Reset(inactivityTimeout)

					eventCh <- ReActStreamEvent{Type: ReActEventLLMChunk, Chunk: &chunk}

					if chunk.Err != nil {
						cancelStream()
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
					if len(chunk.Delta.ReasoningSummaries) > 0 {
						assembledMessage.ReasoningSummaries = append(assembledMessage.ReasoningSummaries, chunk.Delta.ReasoningSummaries...)
					}
					if len(chunk.Delta.OpaqueReasoning) > 0 {
						assembledMessage.OpaqueReasoning = append(assembledMessage.OpaqueReasoning, chunk.Delta.OpaqueReasoning...)
					}
					if len(chunk.Delta.ThinkingBlocks) > 0 {
						assembledMessage.ThinkingBlocks = append(assembledMessage.ThinkingBlocks, chunk.Delta.ThinkingBlocks...)
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

				case steerMsg := <-r.steerChOrNil():
					steering = &steerMsg
					cancelStream()
					// drain 剩余 chunks 防止 goroutine 泄漏
					for range streamCh {
					}
					break chunkLoop

				case <-inactivityTimer.C:
					// 空闲超时：超过 InactivityTimeout 没有收到新数据
					cancelStream()
					r.logger.Warn("stream inactivity timeout",
						zap.Duration("timeout", inactivityTimeout),
						zap.Int("iteration", i+1),
					)
					eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("stream inactivity timeout after %v (no data received)", inactivityTimeout)}
					return

				case <-ctx.Done():
					// 用户主动取消或父 context 超时
					cancelStream()
					eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("context cancelled: %v", ctx.Err())}
					return
				}
			}
			cancelStream()

			// 如果收到 steering，处理后继续外层循环
			if steering != nil {
				rc := ""
				if assembledMessage.ReasoningContent != nil {
					rc = *assembledMessage.ReasoningContent
				}
				if newMsgs, ok := r.applySteering(*steering, messages, assembledMessage.Content, rc, eventCh); ok {
					messages = newMsgs
					continue
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
			// 获取工具执行结果（优先流式执行器）
			var toolResults []types.ToolResult
			if streamExec, ok := r.toolExecutor.(StreamableToolExecutor); ok {
				toolResults = r.executeToolsWithStreaming(ctx, streamExec, assembledMessage.ToolCalls, eventCh)
			} else {
				toolResults = r.toolExecutor.Execute(ctx, assembledMessage.ToolCalls)
			}
			eventCh <- ReActStreamEvent{Type: ReActEventToolsEnd, ToolResults: toolResults}
			if handoffResp, ok := synthesizeHandoffFinalResponse(&llm.ChatResponse{
				ID:       lastChunkID,
				Provider: lastProvider,
				Model:    lastModel,
				Usage:    usageOrZero(lastUsage),
			}, toolResults, usageOrZero(lastUsage)); ok {
				eventCh <- ReActStreamEvent{Type: ReActEventCompleted, Iteration: i + 1, FinalResponse: handoffResp}
				return
			}

			messages = append(messages, assembledMessage)
			for _, result := range toolResults {
				if result.Error != "" && r.config.StopOnError {
					eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("tool execution failed: %s", result.Error)}
					return
				}
				messages = append(messages, result.ToMessage())
			}

			// 工具执行完成后，检查是否有 pending steering
			select {
			case steerMsg := <-r.steerChOrNil():
				if newMsgs, ok := r.applySteering(steerMsg, messages, "", "", eventCh); ok {
					messages = newMsgs
				}
				// 不 return，继续外层 for 循环
			default:
			}
		}

		eventCh <- ReActStreamEvent{Type: ReActEventError, Error: fmt.Sprintf("max iterations reached (%d)", r.config.MaxIterations)}
		return
	}()

	return eventCh, nil
}

// applySteering 根据 steering 消息类型处理 messages 并发送确认事件。
// partialContent 和 reasoningContent 是被中断时已生成的部分内容。
// 零值消息（channel 关闭后产生）返回 false，调用方应忽略。
func (r *ReActExecutor) applySteering(msg SteeringMessage, messages []types.Message, partialContent string, reasoningContent string, eventCh chan<- ReActStreamEvent) ([]types.Message, bool) {
	if msg.IsZero() {
		r.logger.Debug("steering: ignoring zero-value message")
		return messages, false
	}
	switch msg.Type {
	case types.SteeringTypeGuide:
		eventCh <- ReActStreamEvent{Type: ReActEventSteering, SteeringContent: msg.Content}
		r.logger.Info("steering: guide received", zap.String("content", msg.Content))
	case types.SteeringTypeStopAndSend:
		eventCh <- ReActStreamEvent{Type: ReActEventStopAndSend}
		r.logger.Info("steering: stop_and_send received", zap.String("content", msg.Content))
	default:
		r.logger.Warn("steering: unknown type, ignoring", zap.String("type", string(msg.Type)))
		return messages, false
	}
	return types.ApplySteeringToMessages(msg, messages, partialContent, reasoningContent, llm.RoleAssistant), true
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
	ReActEventSteering       = "steering"
	ReActEventStopAndSend    = "stop_and_send"
)

// ReActStreamEvent 表示流式 ReAct 循环事件.
type ReActStreamEvent struct {
	Type            string             `json:"type"`
	Iteration       int                `json:"iteration,omitempty"`
	Chunk           *llm.StreamChunk   `json:"chunk,omitempty"`
	ToolCalls       []types.ToolCall   `json:"tool_calls,omitempty"`
	ToolResults     []types.ToolResult `json:"tool_results,omitempty"`
	ToolCallID      string             `json:"tool_call_id,omitempty"`
	ToolName        string             `json:"tool_name,omitempty"`
	ProgressData    any                `json:"progress_data,omitempty"`
	FinalResponse   *llm.ChatResponse  `json:"final_response,omitempty"`
	Error           string             `json:"error,omitempty"`
	SteeringContent string             `json:"steering_content,omitempty"` // steering 确认内容
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

func synthesizeHandoffFinalResponse(template *llm.ChatResponse, results []types.ToolResult, usage llm.ChatUsage) (*llm.ChatResponse, bool) {
	for _, result := range results {
		control := result.Control()
		if control == nil || control.Type != types.ToolResultControlTypeHandoff || control.Handoff == nil {
			continue
		}
		messageMetadata := map[string]any{
			"handoff_id":       control.Handoff.HandoffID,
			"from_agent_id":    control.Handoff.FromAgentID,
			"to_agent_id":      control.Handoff.ToAgentID,
			"to_agent_name":    control.Handoff.ToAgentName,
			"transfer_message": control.Handoff.TransferMessage,
		}
		for key, value := range control.Handoff.Metadata {
			messageMetadata[key] = value
		}
		response := &llm.ChatResponse{
			Usage: usage,
			Choices: []llm.ChatChoice{{
				Index:        0,
				FinishReason: "stop",
				Message: types.Message{
					Role:             llm.RoleAssistant,
					Content:          control.Handoff.Output,
					ReasoningContent: control.Handoff.ReasoningContent,
					Metadata:         messageMetadata,
				},
			}},
		}
		if template != nil {
			response.ID = template.ID
			response.Provider = template.Provider
			response.Model = template.Model
		}
		if strings.TrimSpace(control.Handoff.Provider) != "" {
			response.Provider = strings.TrimSpace(control.Handoff.Provider)
		}
		if strings.TrimSpace(control.Handoff.Model) != "" {
			response.Model = strings.TrimSpace(control.Handoff.Model)
		}
		if control.Handoff.TokensUsed > 0 {
			response.Usage = llm.ChatUsage{
				CompletionTokens: control.Handoff.TokensUsed,
				TotalTokens:      control.Handoff.TokensUsed,
			}
		}
		return response, true
	}
	return nil, false
}

func usageOrZero(usage *llm.ChatUsage) llm.ChatUsage {
	if usage == nil {
		return llm.ChatUsage{}
	}
	return *usage
}
