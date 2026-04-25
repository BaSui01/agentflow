package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/types"
)

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
	RuntimeStreamApproval     RuntimeStreamEventType = "approval"
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
	SDKApprovalRequested    SDKRunItemEventName = "approval_requested"
	SDKApprovalResponse     SDKRunItemEventName = "approval_response"
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
	SteeringContent string                 `json:"steering_content,omitempty"`
	CurrentStage    string                 `json:"current_stage,omitempty"`
	IterationCount  int                    `json:"iteration_count,omitempty"`
	SelectedMode    string                 `json:"selected_reasoning_mode,omitempty"`
	StopReason      string                 `json:"stop_reason,omitempty"`
	CheckpointID    string                 `json:"checkpoint_id,omitempty"`
	Resumable       bool                   `json:"resumable,omitempty"`
}

// RunEvent maps the runtime-specific stream event to the shared run event
// contract without changing the existing runtime stream wire format.
func (e RuntimeStreamEvent) RunEvent() types.RunEvent {
	event := types.RunEvent{
		Type:         runtimeRunEventType(e),
		Scope:        types.RunScopeAgent,
		CheckpointID: e.CheckpointID,
		ToolCallID:   e.ToolCallID,
		ToolName:     e.ToolName,
		Timestamp:    e.Timestamp,
		Data:         e.Data,
		Metadata:     runtimeRunEventMetadata(e),
	}
	if e.ToolCall != nil {
		event.ToolCall = &types.ToolCall{
			ID:        e.ToolCall.ID,
			Name:      e.ToolCall.Name,
			Arguments: e.ToolCall.Arguments,
		}
		if event.ToolCallID == "" {
			event.ToolCallID = e.ToolCall.ID
		}
		if event.ToolName == "" {
			event.ToolName = e.ToolCall.Name
		}
	}
	if e.ToolResult != nil {
		result := types.ToolResult{
			ToolCallID: e.ToolResult.ToolCallID,
			Name:       e.ToolResult.Name,
			Result:     e.ToolResult.Result,
			Error:      e.ToolResult.Error,
			Duration:   e.ToolResult.Duration,
		}
		event.ToolResult = &result
		if event.ToolCallID == "" {
			event.ToolCallID = e.ToolResult.ToolCallID
		}
		if event.ToolName == "" {
			event.ToolName = e.ToolResult.Name
		}
		if e.ToolResult.Error != "" {
			event.Error = e.ToolResult.Error
		}
	}
	if e.Token != "" || e.Delta != "" || e.Reasoning != "" || e.CurrentStage != "" ||
		e.IterationCount != 0 || e.SelectedMode != "" || e.StopReason != "" || e.SteeringContent != "" ||
		e.Resumable {
		event.Data = runtimeRunEventData(e)
	}
	return event
}

func runtimeRunEventType(e RuntimeStreamEvent) types.RunEventType {
	switch e.Type {
	case RuntimeStreamToken:
		return types.RunEventLLMChunk
	case RuntimeStreamReasoning:
		return types.RunEventReasoning
	case RuntimeStreamToolCall:
		return types.RunEventToolCall
	case RuntimeStreamToolResult:
		return types.RunEventToolResult
	case RuntimeStreamToolProgress:
		return types.RunEventToolProgress
	case RuntimeStreamApproval:
		return types.RunEventApproval
	case RuntimeStreamSession:
		return types.RunEventSession
	case RuntimeStreamSteering, RuntimeStreamStopAndSend:
		return types.RunEventSteering
	case RuntimeStreamStatus:
		fallthrough
	default:
		return types.RunEventStatus
	}
}

func runtimeRunEventMetadata(e RuntimeStreamEvent) map[string]string {
	metadata := map[string]string{}
	if e.SDKEventType != "" {
		metadata["sdk_event_type"] = string(e.SDKEventType)
	}
	if e.SDKEventName != "" {
		metadata["sdk_event_name"] = string(e.SDKEventName)
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func runtimeRunEventData(e RuntimeStreamEvent) map[string]any {
	data := map[string]any{}
	if e.Data != nil {
		data["payload"] = e.Data
	}
	if e.Token != "" {
		data["token"] = e.Token
	}
	if e.Delta != "" {
		data["delta"] = e.Delta
	}
	if e.Reasoning != "" {
		data["reasoning"] = e.Reasoning
	}
	if e.SteeringContent != "" {
		data["steering_content"] = e.SteeringContent
	}
	if e.CurrentStage != "" {
		data["current_stage"] = e.CurrentStage
	}
	if e.IterationCount != 0 {
		data["iteration_count"] = e.IterationCount
	}
	if e.SelectedMode != "" {
		data["selected_reasoning_mode"] = e.SelectedMode
	}
	if e.StopReason != "" {
		data["stop_reason"] = e.StopReason
	}
	if e.Resumable {
		data["resumable"] = e.Resumable
	}
	if len(data) == 0 {
		return nil
	}
	return data
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

func RuntimeStreamEmitterFromContext(ctx context.Context) (RuntimeStreamEmitter, bool) {
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

func EmitRuntimeStatus(emit RuntimeStreamEmitter, status string, event RuntimeStreamEvent) {
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
