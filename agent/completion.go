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
	reactIterationBudget := reactToolLoopBudget(pr)
	ctx = WithRuntimeConversationMessages(ctx, pr.req.Messages)

	if pr.hasTools {
		return b.chatCompletionStreamingWithTools(ctx, pr, emit, steerCh, reactIterationBudget)
	}
	return b.chatCompletionStreamingDirect(ctx, pr, emit, steerCh)
}

type reactStreamingState struct {
	final            *llm.ChatResponse
	currentIteration int
	selectedMode     string
}

func (b *BaseAgent) chatCompletionStreamingWithTools(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter, steerCh *SteeringChannel, reactIterationBudget int) (*llm.ChatResponse, error) {
	state, eventCh, err := b.startReactStreaming(ctx, pr, steerCh, reactIterationBudget, emit)
	if err != nil {
		return nil, err
	}
	for ev := range eventCh {
		if err := b.handleReactStreamEvent(emit, pr, state, ev); err != nil {
			return nil, err
		}
	}
	if state.final == nil {
		return nil, ErrNoResponse
	}
	return state.final, nil
}

func (b *BaseAgent) startReactStreaming(ctx context.Context, pr *preparedRequest, steerCh *SteeringChannel, reactIterationBudget int, emit RuntimeStreamEmitter) (*reactStreamingState, <-chan llmtools.ReActStreamEvent, error) {
	const selectedMode = ReasoningModeReact
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, b.config.Runtime.ToolModel)
	toolExec := newRuntimeHandoffExecutor(
		b,
		newToolManagerExecutor(b.toolManager, b.config.Core.ID, b.config.Runtime.Tools, b.bus),
		runtimeHandoffTargetsFromPreparedRequest(pr),
	)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		toolExec,
		llmtools.ReActConfig{MaxIterations: reactIterationBudget, StopOnError: false},
		b.logger,
	)
	if steerCh != nil {
		executor.SetSteeringChannel(steerCh.Receive())
	}
	eventCh, err := executor.ExecuteStream(ctx, &reactReq)
	if err != nil {
		return nil, nil, err
	}
	emitRuntimeStatus(emit, "reasoning_mode_selected", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "reasoning",
		IterationCount: 0,
		SelectedMode:   selectedMode,
		Data: map[string]any{
			"mode":                   selectedMode,
			"react_iteration_budget": reactIterationBudget,
		},
	})
	return &reactStreamingState{selectedMode: selectedMode}, eventCh, nil
}

func (b *BaseAgent) handleReactStreamEvent(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, ev llmtools.ReActStreamEvent) error {
	switch ev.Type {
	case llmtools.ReActEventIterationStart:
		state.currentIteration = ev.Iteration
	case llmtools.ReActEventLLMChunk:
		emitReactLLMChunk(emit, state, ev)
	case llmtools.ReActEventToolsStart:
		emitReactToolCalls(emit, pr, state, ev.ToolCalls)
	case llmtools.ReActEventToolsEnd:
		emitReactToolResults(emit, pr, state, ev.ToolResults)
	case llmtools.ReActEventToolProgress:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolProgress,
			Timestamp:      time.Now(),
			ToolCallID:     ev.ToolCallID,
			ToolName:       ev.ToolName,
			Data:           ev.ProgressData,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	case llmtools.ReActEventSteering:
		emit(RuntimeStreamEvent{
			Type:            RuntimeStreamSteering,
			Timestamp:       time.Now(),
			SteeringContent: ev.SteeringContent,
			CurrentStage:    "reasoning",
			IterationCount:  state.currentIteration,
			SelectedMode:    state.selectedMode,
		})
	case llmtools.ReActEventStopAndSend:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStopAndSend,
			Timestamp:      time.Now(),
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	case llmtools.ReActEventCompleted:
		state.final = ev.FinalResponse
		emitReactCompletion(emit, state)
	case llmtools.ReActEventError:
		stopReason := string(classifyStopReason(ev.Error))
		emitCompletionLoopStatus(emit, state.currentIteration, state.selectedMode, stopReason)
		return NewErrorWithCause(types.ErrAgentExecution, "streaming execution error", errors.New(ev.Error))
	}
	return nil
}

func emitReactLLMChunk(emit RuntimeStreamEmitter, state *reactStreamingState, ev llmtools.ReActStreamEvent) {
	if ev.Chunk == nil {
		return
	}
	if ev.Chunk.Delta.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToken,
			Timestamp:      time.Now(),
			Token:          ev.Chunk.Delta.Content,
			Delta:          ev.Chunk.Delta.Content,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	}
	if ev.Chunk.Delta.ReasoningContent != nil && *ev.Chunk.Delta.ReasoningContent != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamReasoning,
			Timestamp:      time.Now(),
			Reasoning:      *ev.Chunk.Delta.ReasoningContent,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	}
}

func emitReactToolCalls(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, calls []types.ToolCall) {
	for _, call := range calls {
		sdkEventName := SDKToolCalled
		if runtimeHandoffToolRequested(pr, call.Name) {
			sdkEventName = SDKHandoffRequested
		}
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolCall,
			Timestamp:      time.Now(),
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			ToolCall: &RuntimeToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), call.Arguments...),
			},
			SDKEventType: SDKRunItemEvent,
			SDKEventName: sdkEventName,
		})
	}
}

func emitReactToolResults(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, results []types.ToolResult) {
	for _, tr := range results {
		sdkEventName, resultPayload := reactToolResultPayload(pr, tr)
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolResult,
			Timestamp:      time.Now(),
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			ToolResult: &RuntimeToolResult{
				ToolCallID: tr.ToolCallID,
				Name:       tr.Name,
				Result:     resultPayload,
				Error:      tr.Error,
				Duration:   tr.Duration,
			},
			SDKEventType: SDKRunItemEvent,
			SDKEventName: sdkEventName,
		})
	}
}

func reactToolResultPayload(pr *preparedRequest, tr types.ToolResult) (SDKRunItemEventName, json.RawMessage) {
	sdkEventName := SDKToolOutput
	resultPayload := append(json.RawMessage(nil), tr.Result...)
	if runtimeHandoffToolRequested(pr, tr.Name) {
		sdkEventName = SDKHandoffOccured
		if control := tr.Control(); control != nil && control.Handoff != nil {
			if raw, err := json.Marshal(control.Handoff); err == nil {
				resultPayload = raw
			}
		}
	}
	return sdkEventName, resultPayload
}

func emitReactCompletion(emit RuntimeStreamEmitter, state *reactStreamingState) {
	final := state.final
	if emit != nil && final != nil && len(final.Choices) > 0 && final.Choices[0].Message.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStatus,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKMessageOutputCreated,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"content": final.Choices[0].Message.Content,
			},
		})
	}
	stopReason := normalizeRuntimeStopReasonFromResponse(final)
	emitCompletionLoopStatus(emit, state.currentIteration, state.selectedMode, stopReason)
}

type directStreamingAttemptResult struct {
	assembled        types.Message
	lastID           string
	lastProvider     string
	lastModel        string
	lastUsage        *llm.ChatUsage
	lastFinishReason string
	reasoning        string
	steering         *SteeringMessage
}

func (b *BaseAgent) chatCompletionStreamingDirect(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter, steerCh *SteeringChannel) (*llm.ChatResponse, error) {
	messages := append([]types.Message(nil), pr.req.Messages...)
	var cumulativeUsage llm.ChatUsage
	emitRuntimeStatus(emit, "reasoning_mode_selected", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "responding",
		IterationCount: 1,
	})

	for {
		attempt, err := b.runDirectStreamingAttempt(ctx, pr, messages, emit, steerCh)
		if err != nil {
			return nil, err
		}
		accumulateChatUsage(&cumulativeUsage, attempt.lastUsage)
		if attempt.steering == nil || attempt.steering.IsZero() {
			return finalizeDirectStreamingResponse(emit, attempt, cumulativeUsage), nil
		}
		emitDirectSteeringEvent(emit, attempt.steering)
		messages = types.ApplySteeringToMessages(*attempt.steering, messages, attempt.assembled.Content, attempt.reasoning, llm.RoleAssistant)
	}
}

func (b *BaseAgent) runDirectStreamingAttempt(ctx context.Context, pr *preparedRequest, messages []types.Message, emit RuntimeStreamEmitter, steerCh *SteeringChannel) (*directStreamingAttemptResult, error) {
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	pr.req.Messages = messages
	streamCh, err := pr.chatProvider.Stream(streamCtx, pr.req)
	if err != nil {
		return nil, err
	}
	result := &directStreamingAttemptResult{}
	var reasoningBuf strings.Builder

chunkLoop:
	for {
		select {
		case chunk, ok := <-streamCh:
			if !ok {
				break chunkLoop
			}
			if chunk.Err != nil {
				return nil, chunk.Err
			}
			consumeDirectStreamChunk(emit, result, &reasoningBuf, chunk)
		case msg := <-steerChOrNil(steerCh):
			result.steering = &msg
			cancelStream()
			for range streamCh {
			}
			break chunkLoop
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	result.reasoning = reasoningBuf.String()
	return result, nil
}

func consumeDirectStreamChunk(emit RuntimeStreamEmitter, result *directStreamingAttemptResult, reasoningBuf *strings.Builder, chunk llm.StreamChunk) {
	if chunk.ID != "" {
		result.lastID = chunk.ID
	}
	if chunk.Provider != "" {
		result.lastProvider = chunk.Provider
	}
	if chunk.Model != "" {
		result.lastModel = chunk.Model
	}
	if chunk.Usage != nil {
		result.lastUsage = chunk.Usage
	}
	if chunk.FinishReason != "" {
		result.lastFinishReason = chunk.FinishReason
	}
	if chunk.Delta.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToken,
			Timestamp:      time.Now(),
			Token:          chunk.Delta.Content,
			Delta:          chunk.Delta.Content,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "responding",
			IterationCount: 1,
		})
		result.assembled.Content += chunk.Delta.Content
	}
	if chunk.Delta.ReasoningContent != nil && *chunk.Delta.ReasoningContent != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamReasoning,
			Timestamp:      time.Now(),
			Reasoning:      *chunk.Delta.ReasoningContent,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "responding",
			IterationCount: 1,
		})
		reasoningBuf.WriteString(*chunk.Delta.ReasoningContent)
	}
	if len(chunk.Delta.ReasoningSummaries) > 0 {
		result.assembled.ReasoningSummaries = append(result.assembled.ReasoningSummaries, chunk.Delta.ReasoningSummaries...)
	}
	if len(chunk.Delta.OpaqueReasoning) > 0 {
		result.assembled.OpaqueReasoning = append(result.assembled.OpaqueReasoning, chunk.Delta.OpaqueReasoning...)
	}
	if len(chunk.Delta.ThinkingBlocks) > 0 {
		result.assembled.ThinkingBlocks = append(result.assembled.ThinkingBlocks, chunk.Delta.ThinkingBlocks...)
	}
}

func accumulateChatUsage(total, usage *llm.ChatUsage) {
	if usage == nil || total == nil {
		return
	}
	total.PromptTokens += usage.PromptTokens
	total.CompletionTokens += usage.CompletionTokens
	total.TotalTokens += usage.TotalTokens
}

func finalizeDirectStreamingResponse(emit RuntimeStreamEmitter, attempt *directStreamingAttemptResult, cumulativeUsage llm.ChatUsage) *llm.ChatResponse {
	if attempt.reasoning != "" {
		rc := attempt.reasoning
		attempt.assembled.ReasoningContent = &rc
	}
	attempt.assembled.Role = llm.RoleAssistant
	resp := &llm.ChatResponse{
		ID:       attempt.lastID,
		Provider: attempt.lastProvider,
		Model:    attempt.lastModel,
		Choices: []llm.ChatChoice{{
			Index:        0,
			FinishReason: attempt.lastFinishReason,
			Message:      attempt.assembled,
		}},
		Usage: cumulativeUsage,
	}
	if emit != nil && attempt.assembled.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStatus,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKMessageOutputCreated,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: 1,
			Data: map[string]any{
				"content": attempt.assembled.Content,
			},
		})
	}
	emitCompletionLoopStatus(emit, 1, "", normalizeRuntimeStopReason(attempt.lastFinishReason))
	return resp
}

func emitDirectSteeringEvent(emit RuntimeStreamEmitter, steering *SteeringMessage) {
	switch steering.Type {
	case SteeringTypeGuide:
		emit(RuntimeStreamEvent{
			Type:            RuntimeStreamSteering,
			Timestamp:       time.Now(),
			SteeringContent: steering.Content,
			CurrentStage:    "responding",
			IterationCount:  1,
		})
	case SteeringTypeStopAndSend:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStopAndSend,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: 1,
		})
	}
}

// chatCompletionWithTools executes a non-streaming ReAct loop with tools.
func (b *BaseAgent) chatCompletionWithTools(ctx context.Context, pr *preparedRequest) (*llm.ChatResponse, error) {
	ctx = WithRuntimeConversationMessages(ctx, pr.req.Messages)
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, b.config.Runtime.ToolModel)
	reactIterationBudget := reactToolLoopBudget(pr)
	toolExec := newRuntimeHandoffExecutor(
		b,
		newToolManagerExecutor(b.toolManager, b.config.Core.ID, b.config.Runtime.Tools, b.bus),
		runtimeHandoffTargetsFromPreparedRequest(pr),
	)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		toolExec,
		llmtools.ReActConfig{MaxIterations: reactIterationBudget, StopOnError: false},
		b.logger,
	)
	resp, _, err := executor.Execute(ctx, &reactReq)
	if err != nil {
		return resp, NewErrorWithCause(types.ErrAgentExecution, "ReAct execution failed", err)
	}
	return resp, nil
}

func reactToolLoopBudget(pr *preparedRequest) int {
	if pr != nil && pr.maxReActIter > 0 {
		return pr.maxReActIter
	}
	return 1
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
type SDKStreamEventType string
type SDKRunItemEventName string

const (
	RuntimeStreamToken        RuntimeStreamEventType = "token"
	RuntimeStreamReasoning    RuntimeStreamEventType = "reasoning"
	RuntimeStreamToolCall     RuntimeStreamEventType = "tool_call"
	RuntimeStreamToolResult   RuntimeStreamEventType = "tool_result"
	RuntimeStreamToolProgress RuntimeStreamEventType = "tool_progress"
	RuntimeStreamSession      RuntimeStreamEventType = "session"
	RuntimeStreamStatus       RuntimeStreamEventType = "status"
	RuntimeStreamSteering     RuntimeStreamEventType = "steering"
	RuntimeStreamStopAndSend  RuntimeStreamEventType = "stop_and_send"
)

const (
	SDKRawResponseEvent  SDKStreamEventType = "raw_response_event"
	SDKRunItemEvent      SDKStreamEventType = "run_item_stream_event"
	SDKAgentUpdatedEvent SDKStreamEventType = "agent_updated_stream_event"
)

const (
	SDKMessageOutputCreated SDKRunItemEventName = "message_output_created"
	SDKHandoffRequested     SDKRunItemEventName = "handoff_requested"
	SDKToolCalled           SDKRunItemEventName = "tool_called"
	SDKToolSearchCalled     SDKRunItemEventName = "tool_search_called"
	SDKToolSearchOutput     SDKRunItemEventName = "tool_search_output_created"
	SDKToolOutput           SDKRunItemEventName = "tool_output"
	SDKReasoningCreated     SDKRunItemEventName = "reasoning_item_created"
	SDKMCPApprovalRequested SDKRunItemEventName = "mcp_approval_requested"
	SDKMCPApprovalResponse  SDKRunItemEventName = "mcp_approval_response"
	SDKMCPListTools         SDKRunItemEventName = "mcp_list_tools"
)

var SDKHandoffOccured = SDKRunItemEventName(handoffOccuredEventName())

func handoffOccuredEventName() string {
	return string([]byte{'h', 'a', 'n', 'd', 'o', 'f', 'f', '_', 'o', 'c', 'c', 'u', 'r', 'e', 'd'})
}

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
	SDKEventType    SDKStreamEventType     `json:"sdk_event_type,omitempty"`
	SDKEventName    SDKRunItemEventName    `json:"sdk_event_name,omitempty"`
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
	CurrentStage    string                 `json:"current_stage,omitempty"`
	IterationCount  int                    `json:"iteration_count,omitempty"`
	SelectedMode    string                 `json:"selected_reasoning_mode,omitempty"`
	StopReason      string                 `json:"stop_reason,omitempty"`
	CheckpointID    string                 `json:"checkpoint_id,omitempty"`
	Resumable       bool                   `json:"resumable,omitempty"`
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

func emitRuntimeStatus(emit RuntimeStreamEmitter, status string, event RuntimeStreamEvent) {
	if emit == nil {
		return
	}
	event.Type = RuntimeStreamStatus
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Data == nil {
		event.Data = map[string]any{"status": status}
	} else if payload, ok := event.Data.(map[string]any); ok {
		if _, exists := payload["status"]; !exists {
			payload["status"] = status
		}
		event.Data = payload
	}
	emit(event)
}

func emitCompletionLoopStatus(emit RuntimeStreamEmitter, iteration int, selectedMode, stopReason string) {
	normalizedStopReason := normalizeTopLevelStopReason(stopReason, stopReason)
	emitRuntimeStatus(emit, "completion_judge_decision", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "evaluate",
		IterationCount: iteration,
		SelectedMode:   selectedMode,
		StopReason:     normalizedStopReason,
		Data: map[string]any{
			"decision":            "done",
			"solved":              normalizedStopReason == string(StopReasonSolved),
			"internal_stop_cause": stopReason,
		},
	})
	emitRuntimeStatus(emit, "loop_stopped", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "completed",
		IterationCount: iteration,
		SelectedMode:   selectedMode,
		StopReason:     normalizedStopReason,
		Data: map[string]any{
			"state":               "stopped",
			"internal_stop_cause": stopReason,
		},
	})
}

func normalizeRuntimeStopReasonFromResponse(resp *llm.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return normalizeRuntimeStopReason("")
	}
	return normalizeRuntimeStopReason(resp.Choices[0].FinishReason)
}

func normalizeRuntimeStopReason(finishReason string) string {
	normalized := strings.TrimSpace(finishReason)
	if normalized == "" {
		return string(StopReasonSolved)
	}
	return normalizeTopLevelStopReason(normalized, normalized)
}

func runtimeHandoffTargetsFromPreparedRequest(pr *preparedRequest) []RuntimeHandoffTarget {
	if pr == nil || len(pr.handoffTools) == 0 {
		return nil
	}
	out := make([]RuntimeHandoffTarget, 0, len(pr.handoffTools))
	for _, target := range pr.handoffTools {
		out = append(out, target)
	}
	return out
}

func runtimeHandoffToolRequested(pr *preparedRequest, toolName string) bool {
	if pr == nil || len(pr.handoffTools) == 0 {
		return false
	}
	_, ok := pr.handoffTools[toolName]
	return ok
}
