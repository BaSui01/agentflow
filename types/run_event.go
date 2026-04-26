package types

import "time"

// RunEventType is the shared event taxonomy for agent, team, workflow, tools,
// approval, checkpoint, and usage streams.
type RunEventType string

const (
	RunEventLLMChunk             RunEventType = "llm_chunk"
	RunEventReasoning            RunEventType = "reasoning"
	RunEventToolCall             RunEventType = "tool_call"
	RunEventToolResult           RunEventType = "tool_result"
	RunEventToolProgress         RunEventType = "tool_progress"

	// Deprecated: Use RunEventHandoffRequested / RunEventHandoffCompleted instead.
	RunEventHandoff RunEventType = "handoff"

	// Deprecated: Use RunEventApprovalRequested / RunEventApprovalResolved instead.
	RunEventApproval RunEventType = "approval"

	// Deprecated: Use RunEventCheckpointSaved instead.
	RunEventCheckpoint RunEventType = "checkpoint"

	// Deprecated: Use RunEventFailed instead.
	RunEventError RunEventType = "error"

	RunEventUsage                RunEventType = "usage"
	RunEventSession              RunEventType = "session"
	RunEventStatus               RunEventType = "status"
	RunEventSteering             RunEventType = "steering"
	RunEventWorkflowNodeStart    RunEventType = "workflow_node_start"
	RunEventWorkflowNodeComplete RunEventType = "workflow_node_complete"
	RunEventWorkflowNodeError    RunEventType = "workflow_node_error"

	RunEventHandoffRequested  RunEventType = "handoff_requested"
	RunEventHandoffCompleted  RunEventType = "handoff_completed"
	RunEventApprovalRequested RunEventType = "approval_requested"
	RunEventApprovalResolved  RunEventType = "approval_resolved"
	RunEventCheckpointSaved   RunEventType = "checkpoint_saved"
	RunEventStateChanged      RunEventType = "state_changed"
	RunEventCompleted         RunEventType = "completed"
	RunEventFailed            RunEventType = "failed"
)

// RunEvent carries a normalized execution event. Runtime-specific event
// structs may keep their wire format, but they should map to this contract
// before persistence, replay, audit, or cross-runtime stream consumption.
type RunEvent struct {
	Type         RunEventType      `json:"type"`
	Scope        RunScope          `json:"scope,omitempty"`
	RunID        string            `json:"run_id,omitempty"`
	ParentRunID  string            `json:"parent_run_id,omitempty"`
	TraceID      string            `json:"trace_id,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	AgentID      string            `json:"agent_id,omitempty"`
	TeamID       string            `json:"team_id,omitempty"`
	WorkflowID   string            `json:"workflow_id,omitempty"`
	NodeID       string            `json:"node_id,omitempty"`
	NodeName     string            `json:"node_name,omitempty"`
	CheckpointID string            `json:"checkpoint_id,omitempty"`
	ToolCallID   string            `json:"tool_call_id,omitempty"`
	ToolName     string            `json:"tool_name,omitempty"`
	ToolCall     *ToolCall         `json:"tool_call,omitempty"`
	ToolResult   *ToolResult       `json:"tool_result,omitempty"`
	Usage        *ChatUsage        `json:"usage,omitempty"`
	Error        string            `json:"error,omitempty"`
	Timestamp    time.Time         `json:"timestamp,omitempty"`
	Sequence     int64             `json:"sequence,omitempty"`
	Data         any               `json:"data,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}
