package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

// ChatCompletion 调用 LLM 完成对话
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []types.Message) (*llm.ChatResponse, error) {
	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		return b.chatCompletionStreaming(ctx, pr, emit)
	}

	if pr.hasTools {
		return b.chatCompletionWithTools(ctx, pr)
	}

	return pr.chatProvider.Completion(ctx, pr.req)
}

// chatCompletionStreaming handles the streaming execution path of ChatCompletion.
// 支持 Steering：通过 context 中的 SteeringChannel 接收实时引导/停止后发送指令。
func (b *BaseAgent) chatCompletionStreaming(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter) (*llm.ChatResponse, error) {
	steerCh, _ := SteeringChannelFromContext(ctx)

	if pr.hasTools {
		reactReq := *pr.req
		reactReq.Model = effectiveToolModel(pr.req.Model, b.config.Runtime.ToolModel)
		executor := llmtools.NewReActExecutor(
			pr.toolProvider,
			newToolManagerExecutor(b.toolManager, b.config.Core.ID, b.config.Runtime.Tools, b.bus),
			llmtools.ReActConfig{MaxIterations: pr.maxReActIter, StopOnError: false},
			b.logger,
		)
		// 将 steering channel 直接传入 ReAct 执行器（类型统一，无需 adapter）
		if steerCh != nil {
			executor.SetSteeringChannel(steerCh.Receive())
		}

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
				if ev.Chunk != nil && ev.Chunk.Delta.ReasoningContent != nil && *ev.Chunk.Delta.ReasoningContent != "" {
					emit(RuntimeStreamEvent{
						Type:      RuntimeStreamReasoning,
						Timestamp: time.Now(),
						Reasoning: *ev.Chunk.Delta.ReasoningContent,
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
			case llmtools.ReActEventSteering:
				emit(RuntimeStreamEvent{
					Type:            RuntimeStreamSteering,
					Timestamp:       time.Now(),
					SteeringContent: ev.SteeringContent,
				})
			case llmtools.ReActEventStopAndSend:
				emit(RuntimeStreamEvent{
					Type:      RuntimeStreamStopAndSend,
					Timestamp: time.Now(),
				})
			case llmtools.ReActEventCompleted:
				final = ev.FinalResponse
			case llmtools.ReActEventError:
				return nil, NewErrorWithCause(types.ErrAgentExecution, "streaming execution error", errors.New(ev.Error))
			}
		}
		if final == nil {
			return nil, ErrNoResponse
		}
		return final, nil
	}

	// 无工具路径：外层 for 循环支持 steering 重试
	messages := make([]types.Message, len(pr.req.Messages))
	copy(messages, pr.req.Messages)
	var cumulativeUsage llm.ChatUsage

	for {
		streamCtx, cancelStream := context.WithCancel(ctx)
		pr.req.Messages = messages
		streamCh, err := pr.chatProvider.Stream(streamCtx, pr.req)
		if err != nil {
			cancelStream()
			return nil, err
		}

		var (
			assembled types.Message
			lastID    string
			lastProv  string
			lastModel string
			lastUsage *llm.ChatUsage
			lastFR    string
			steering  *SteeringMessage
		)
		var reasoningBuf strings.Builder

	chunkLoop:
		for {
			select {
			case chunk, ok := <-streamCh:
				if !ok {
					break chunkLoop
				}
				if chunk.Err != nil {
					cancelStream()
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
				if chunk.Delta.ReasoningContent != nil && *chunk.Delta.ReasoningContent != "" {
					emit(RuntimeStreamEvent{
						Type:      RuntimeStreamReasoning,
						Timestamp: time.Now(),
						Reasoning: *chunk.Delta.ReasoningContent,
					})
					reasoningBuf.WriteString(*chunk.Delta.ReasoningContent)
				}
			case msg := <-steerChOrNil(steerCh):
				steering = &msg
				cancelStream()
				// drain 剩余 chunks 防止 goroutine 泄漏
				for range streamCh {
				}
				break chunkLoop
			case <-ctx.Done():
				cancelStream()
				return nil, ctx.Err()
			}
		}
		cancelStream()

		// 累计 token 用量（包括被中断的调用）
		if lastUsage != nil {
			cumulativeUsage.PromptTokens += lastUsage.PromptTokens
			cumulativeUsage.CompletionTokens += lastUsage.CompletionTokens
			cumulativeUsage.TotalTokens += lastUsage.TotalTokens
		}

		if steering == nil {
			// 正常完成，组装 response 返回
			if reasoningBuf.Len() > 0 {
				rc := reasoningBuf.String()
				assembled.ReasoningContent = &rc
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
				Usage: cumulativeUsage,
			}
			return resp, nil
		}

		// 处理 steering 消息（零值消息忽略，视为正常完成）
		if steering.IsZero() {
			if reasoningBuf.Len() > 0 {
				rc := reasoningBuf.String()
				assembled.ReasoningContent = &rc
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
				Usage: cumulativeUsage,
			}
			return resp, nil
		}

		// emit 确认事件
		switch steering.Type {
		case SteeringTypeGuide:
			emit(RuntimeStreamEvent{
				Type:            RuntimeStreamSteering,
				Timestamp:       time.Now(),
				SteeringContent: steering.Content,
			})
		case SteeringTypeStopAndSend:
			emit(RuntimeStreamEvent{
				Type:      RuntimeStreamStopAndSend,
				Timestamp: time.Now(),
			})
		}

		// 统一的 messages 变换逻辑
		rc := ""
		if reasoningBuf.Len() > 0 {
			rc = reasoningBuf.String()
		}
		messages = types.ApplySteeringToMessages(*steering, messages, assembled.Content, rc, llm.RoleAssistant)
		// 继续外层 for 循环，重新发起流式调用
	}
}

// chatCompletionWithTools executes a non-streaming ReAct loop with tools.
func (b *BaseAgent) chatCompletionWithTools(ctx context.Context, pr *preparedRequest) (*llm.ChatResponse, error) {
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, b.config.Runtime.ToolModel)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		newToolManagerExecutor(b.toolManager, b.config.Core.ID, b.config.Runtime.Tools, b.bus),
		llmtools.ReActConfig{MaxIterations: pr.maxReActIter, StopOnError: false},
		b.logger,
	)
	resp, _, err := executor.Execute(ctx, &reactReq)
	if err != nil {
		return resp, NewErrorWithCause(types.ErrAgentExecution, "ReAct execution failed", err)
	}
	return resp, nil
}

// StreamCompletion 流式调用 LLM
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []types.Message) (<-chan llm.StreamChunk, error) {
	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}
	return pr.chatProvider.Stream(ctx, pr.req)
}

func applyContextRouteHints(req *llm.ChatRequest, ctx context.Context) {
	if req == nil {
		return
	}
	if provider, ok := types.LLMProvider(ctx); ok && strings.TrimSpace(provider) != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, 2)
		}
		req.Metadata[llmcore.MetadataKeyChatProvider] = strings.TrimSpace(provider)
	}
	if routePolicy, ok := types.LLMRoutePolicy(ctx); ok && strings.TrimSpace(routePolicy) != "" {
		if req.Metadata == nil {
			req.Metadata = make(map[string]string, 2)
		}
		req.Metadata["route_policy"] = strings.TrimSpace(routePolicy)
	}
}

// =============================================================================
// Runtime Stream Events
// =============================================================================

type runtimeStreamEmitterKey struct{}

// RuntimeStreamEventType identifies the kind of runtime stream event.
type RuntimeStreamEventType string

const (
	RuntimeStreamToken        RuntimeStreamEventType = "token"
	RuntimeStreamReasoning    RuntimeStreamEventType = "reasoning"
	RuntimeStreamToolCall     RuntimeStreamEventType = "tool_call"
	RuntimeStreamToolResult   RuntimeStreamEventType = "tool_result"
	RuntimeStreamToolProgress RuntimeStreamEventType = "tool_progress"
	RuntimeStreamSession      RuntimeStreamEventType = "session"
	RuntimeStreamSteering     RuntimeStreamEventType = "steering"
	RuntimeStreamStopAndSend  RuntimeStreamEventType = "stop_and_send"
)

// RuntimeToolCall carries tool invocation metadata in a stream event.
type RuntimeToolCall struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// RuntimeToolResult carries tool execution results in a stream event.
type RuntimeToolResult struct {
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration,omitempty"`
}

// RuntimeStreamEvent is a single event emitted during streamed Agent execution.
type RuntimeStreamEvent struct {
	Type            RuntimeStreamEventType `json:"type"`
	Timestamp       time.Time              `json:"timestamp"`
	Token           string                 `json:"token,omitempty"`
	Delta           string                 `json:"delta,omitempty"`
	Reasoning       string                 `json:"reasoning,omitempty"`
	ToolCall        *RuntimeToolCall       `json:"tool_call,omitempty"`
	ToolResult      *RuntimeToolResult     `json:"tool_result,omitempty"`
	ToolCallID      string                 `json:"tool_call_id,omitempty"`
	ToolName        string                 `json:"tool_name,omitempty"`
	Data            any                    `json:"data,omitempty"`
	SteeringContent string                 `json:"steering_content,omitempty"` // steering 确认内容
}

// RuntimeStreamEmitter is a callback that receives runtime stream events.
type RuntimeStreamEmitter func(RuntimeStreamEvent)

// WithRuntimeStreamEmitter stores an emitter in the context.
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
