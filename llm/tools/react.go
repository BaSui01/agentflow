package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// ReActConfig 定义 ReAct 循环的配置。
type ReActConfig struct {
	MaxIterations int  // 最大迭代次数（防止无限循环）
	StopOnError   bool // 遇到工具错误时是否停止
}

// ReActExecutor 实现 ReAct (Reasoning and Acting) 循环。
// 自动处理 "LLM -> Tool -> LLM" 的多轮对话，直到 LLM 不再调用工具或达到最大迭代次数。
type ReActExecutor struct {
	provider     llmpkg.Provider
	toolExecutor ToolExecutor
	logger       *zap.Logger
	config       ReActConfig
}

// NewReActExecutor 创建 ReAct 执行器。
func NewReActExecutor(provider llmpkg.Provider, toolExecutor ToolExecutor, config ReActConfig, logger *zap.Logger) *ReActExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	// 设置默认值
	if config.MaxIterations == 0 {
		config.MaxIterations = 10 // 默认最多 10 轮
	}

	return &ReActExecutor{
		provider:     provider,
		toolExecutor: toolExecutor,
		logger:       logger,
		config:       config,
	}
}

// Execute 执行 ReAct 循环，返回最终的 LLM 响应和所有中间步骤。
func (r *ReActExecutor) Execute(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, []ReActStep, error) {
	steps := make([]ReActStep, 0)
	messages := append([]llmpkg.Message{}, req.Messages...) // 复制消息历史

	for i := 0; i < r.config.MaxIterations; i++ {
		r.logger.Debug("ReAct iteration", zap.Int("iteration", i+1))

		// 1. 调用 LLM（Thought 阶段）
		callReq := *req
		callReq.Messages = messages
		resp, err := r.provider.Completion(ctx, &callReq)
		if err != nil {
			return nil, steps, fmt.Errorf("LLM call failed at iteration %d: %w", i+1, err)
		}

		// 2. 检查是否有工具调用
		if len(resp.Choices) == 0 {
			return resp, steps, fmt.Errorf("no choices in LLM response")
		}

		choice := resp.Choices[0]
		toolCalls := choice.Message.ToolCalls

		// 记录步骤（Thought）
		step := ReActStep{
			StepNumber: i + 1,
			Thought:    choice.Message.Content, // LLM 的思考过程
			Timestamp:  fmt.Sprintf("%d", i+1),
			TokensUsed: resp.Usage.TotalTokens,
		}

		if len(toolCalls) == 0 {
			// 没有工具调用，循环结束
			r.logger.Info("ReAct completed", zap.Int("iterations", i+1), zap.String("finish_reason", choice.FinishReason))
			steps = append(steps, step)
			return resp, steps, nil
		}

		// 3. 执行工具调用（Action 阶段）
		r.logger.Info("executing tools", zap.Int("count", len(toolCalls)))
		step.Actions = toolCalls
		toolResults := r.toolExecutor.Execute(ctx, toolCalls)
		step.Observations = toolResults // Observation 阶段

		// 检查是否有工具执行失败
		hasError := false
		for _, result := range toolResults {
			if result.Error != "" {
				hasError = true
				r.logger.Warn("tool execution failed",
					zap.String("tool", result.Name),
					zap.String("error", result.Error))
			}
		}

		if hasError && r.config.StopOnError {
			steps = append(steps, step)
			return resp, steps, fmt.Errorf("tool execution failed, stopping ReAct loop")
		}

		// 4. 将 Assistant 的 ToolCalls 和工具结果添加到消息历史
		// 先添加 Assistant 的消息
		messages = append(messages, choice.Message)

		// 再添加工具结果
		for _, result := range toolResults {
			messages = append(messages, result.ToMessage())
		}

		steps = append(steps, step)

		// 5. 继续下一轮循环
	}

	// 达到最大迭代次数
	r.logger.Warn("ReAct max iterations reached", zap.Int("max", r.config.MaxIterations))
	return nil, steps, fmt.Errorf("max iterations reached (%d)", r.config.MaxIterations)
}

// ExecuteWithTrace 执行 ReAct 循环并返回完整追踪
func (r *ReActExecutor) ExecuteWithTrace(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, *ReActTrace, error) {
	resp, steps, err := r.Execute(ctx, req)

	trace := &ReActTrace{
		TraceID:    fmt.Sprintf("react-%d", len(steps)),
		Steps:      steps,
		TotalSteps: len(steps),
		Success:    err == nil,
	}

	// 计算总 token 数
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

// ReActStep 表示 ReAct 循环的一个步骤（Thought → Action → Observation）
type ReActStep struct {
	StepNumber   int               `json:"step_number"`
	Thought      string            `json:"thought,omitempty"`      // 思考过程（从 LLM 响应中提取）
	Actions      []llmpkg.ToolCall `json:"actions,omitempty"`      // 行动（工具调用）
	Observations []ToolResult      `json:"observations,omitempty"` // 观察结果
	Timestamp    string            `json:"timestamp"`
	TokensUsed   int               `json:"tokens_used,omitempty"`
}

// ReActTrace 表示完整的 ReAct 执行追踪
type ReActTrace struct {
	TraceID      string      `json:"trace_id"`
	Steps        []ReActStep `json:"steps"`
	TotalTokens  int         `json:"total_tokens"`
	TotalSteps   int         `json:"total_steps"`
	Success      bool        `json:"success"`
	FinalAnswer  string      `json:"final_answer,omitempty"`
	ErrorMessage string      `json:"error_message,omitempty"`
}

// LLMCallInfo 记录 LLM 调用的详细信息（保留用于向后兼容）
type LLMCallInfo struct {
	Request  llmpkg.ChatRequest  `json:"request"`
	Response llmpkg.ChatResponse `json:"response"`
}

// ExecuteStream 执行流式 ReAct 循环。
// 注意：流式模式下的工具调用需要特殊处理，因为 ToolCalls 可能分散在多个 chunk 中。
func (r *ReActExecutor) ExecuteStream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan ReActStreamEvent, error) {
	eventCh := make(chan ReActStreamEvent)

	go func() {
		defer close(eventCh)

		messages := append([]llmpkg.Message{}, req.Messages...)

		for i := 0; i < r.config.MaxIterations; i++ {
			// 检查上下文取消
			select {
			case <-ctx.Done():
				eventCh <- ReActStreamEvent{
					Type:  "error",
					Error: fmt.Sprintf("context cancelled: %v", ctx.Err()),
				}
				return
			default:
			}

			// 发送迭代开始事件
			eventCh <- ReActStreamEvent{
				Type:      "iteration_start",
				Iteration: i + 1,
			}

			// 调用 LLM Stream
			callReq := *req
			callReq.Messages = messages

			streamCh, err := r.provider.Stream(ctx, &callReq)
			if err != nil {
				eventCh <- ReActStreamEvent{
					Type:  "error",
					Error: fmt.Sprintf("LLM stream failed: %s", err.Error()),
				}
				return
			}

			// 收集流式响应
			var (
				assembledMessage llmpkg.Message
				toolCallOrder    []string
				toolCallByID     map[string]*struct {
					id           string
					name         string
					argsFinal    json.RawMessage
					argsBuilding strings.Builder
				}
				lastChunkID      string
				lastProvider     string
				lastModel        string
				lastUsage        *llmpkg.ChatUsage
				lastFinishReason string
			)

			for chunk := range streamCh {
				// 检查上下文取消
				select {
				case <-ctx.Done():
					eventCh <- ReActStreamEvent{
						Type:  "error",
						Error: fmt.Sprintf("context cancelled: %v", ctx.Err()),
					}
					return
				default:
				}

				// 转发 chunk 给用户
				eventCh <- ReActStreamEvent{
					Type:  "llm_chunk",
					Chunk: &chunk,
				}

				if chunk.Err != nil {
					eventCh <- ReActStreamEvent{
						Type:  "error",
						Error: chunk.Err.Error(),
					}
					return
				}

				// 组装消息
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
						id := strings.TrimSpace(tc.ID)
						if id == "" {
							id = fmt.Sprintf("call_%d_%d", i+1, len(toolCallOrder)+1)
						}
						acc := toolCallByID[id]
						if acc == nil {
							acc = &struct {
								id           string
								name         string
								argsFinal    json.RawMessage
								argsBuilding strings.Builder
							}{id: id}
							toolCallByID[id] = acc
							toolCallOrder = append(toolCallOrder, id)
						}
						if strings.TrimSpace(tc.Name) != "" {
							acc.name = strings.TrimSpace(tc.Name)
						}
						if len(tc.Arguments) == 0 || len(acc.argsFinal) > 0 {
							continue
						}

						// OpenAI 等 provider 在 stream delta 中会把 arguments 作为“JSON 字符串片段”流式输出；
						// 这里统一先尝试反序列化为 string，再进行拼接。
						var argSegStr string
						if err := json.Unmarshal(tc.Arguments, &argSegStr); err == nil {
							acc.argsBuilding.WriteString(argSegStr)
							continue
						}

						// 其它 provider 可能直接返回 JSON 片段或完整 JSON（object/array）
						if json.Valid(tc.Arguments) {
							acc.argsFinal = append([]byte(nil), tc.Arguments...)
							continue
						}
						acc.argsBuilding.WriteString(string(tc.Arguments))
					}
				}
			}

			assembledMessage.Role = llmpkg.RoleAssistant
			nativeToolCalls := make([]llmpkg.ToolCall, 0, len(toolCallOrder))
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
							eventCh <- ReActStreamEvent{
								Type:  "error",
								Error: fmt.Sprintf("invalid tool call arguments (id=%s tool=%s): %s", acc.id, acc.name, raw),
							}
							return
						}
						args = json.RawMessage(raw)
					}
				}
				nativeToolCalls = append(nativeToolCalls, llmpkg.ToolCall{
					ID:        acc.id,
					Name:      acc.name,
					Arguments: args,
				})
			}
			assembledMessage.ToolCalls = nativeToolCalls

			// 检查是否有工具调用
			toolCalls := assembledMessage.ToolCalls

			if len(toolCalls) == 0 {
				// 没有工具调用，循环结束
				final := &llmpkg.ChatResponse{
					ID:       lastChunkID,
					Provider: lastProvider,
					Model:    lastModel,
					Choices: []llmpkg.ChatChoice{{
						Index:        0,
						FinishReason: lastFinishReason,
						Message:      assembledMessage,
					}},
				}
				if lastUsage != nil {
					final.Usage = *lastUsage
				}
				eventCh <- ReActStreamEvent{
					Type:          "completed",
					Iteration:     i + 1,
					FinalResponse: final,
				}
				return
			}

			// 执行工具调用
			eventCh <- ReActStreamEvent{
				Type:      "tools_start",
				ToolCalls: toolCalls,
			}

			toolResults := r.toolExecutor.Execute(ctx, toolCalls)

			eventCh <- ReActStreamEvent{
				Type:        "tools_end",
				ToolResults: toolResults,
			}

			// 更新消息历史
			messages = append(messages, assembledMessage)
			for _, result := range toolResults {
				toolMessage := result.ToMessage()
				if result.Error != "" && r.config.StopOnError {
					messages = append(messages, toolMessage)
					eventCh <- ReActStreamEvent{
						Type:  "error",
						Error: fmt.Sprintf("tool execution failed: %s", result.Error),
					}
					return
				}
				messages = append(messages, toolMessage)
			}
		}

		// 达到最大迭代次数
		eventCh <- ReActStreamEvent{
			Type:  "error",
			Error: fmt.Sprintf("max iterations reached (%d)", r.config.MaxIterations),
		}
	}()

	return eventCh, nil
}

// ReActStreamEvent 表示流式 ReAct 循环的事件。
type ReActStreamEvent struct {
	Type          string               `json:"type"` // iteration_start, llm_chunk, tools_start, tools_end, completed, error
	Iteration     int                  `json:"iteration,omitempty"`
	Chunk         *llmpkg.StreamChunk  `json:"chunk,omitempty"`
	ToolCalls     []llmpkg.ToolCall    `json:"tool_calls,omitempty"`
	ToolResults   []ToolResult         `json:"tool_results,omitempty"`
	FinalResponse *llmpkg.ChatResponse `json:"final_response,omitempty"`
	Error         string               `json:"error,omitempty"`
}

// ToMessage 将 ToolResult 转换为 LLM Message。
func (tr ToolResult) ToMessage() llmpkg.Message {
	msg := llmpkg.Message{
		Role:       llmpkg.RoleTool,
		ToolCallID: tr.ToolCallID,
		Name:       tr.Name,
	}

	if tr.Error != "" {
		msg.Content = fmt.Sprintf("Error: %s", tr.Error)
	} else {
		msg.Content = string(tr.Result)
	}

	return msg
}

// ToJSON 将 ToolResult 序列化为 JSON。
func (tr ToolResult) ToJSON() (json.RawMessage, error) {
	data, err := json.Marshal(tr)
	if err != nil {
		return nil, err
	}
	return data, nil
}
