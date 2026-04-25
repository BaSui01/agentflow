package types

import "time"

// RunScope identifies the runtime surface that owns a run.
type RunScope string

const (
	RunScopeAgent    RunScope = "agent"
	RunScopeTeam     RunScope = "team"
	RunScopeWorkflow RunScope = "workflow"
)

// RunStatus is the stable lifecycle state for a run.
type RunStatus string

const (
	RunStatusPending         RunStatus = "pending"
	RunStatusRunning         RunStatus = "running"
	RunStatusPendingApproval RunStatus = "pending_approval"
	RunStatusPaused          RunStatus = "paused"
	RunStatusCompleted       RunStatus = "completed"
	RunStatusFailed          RunStatus = "failed"
	RunStatusCanceled        RunStatus = "canceled"
)

// RunApprovalState captures a pending approval that can pause or resume a run.
type RunApprovalState struct {
	ApprovalID  string            `json:"approval_id"`
	ToolCallID  string            `json:"tool_call_id,omitempty"`
	ToolName    string            `json:"tool_name,omitempty"`
	Resource    string            `json:"resource,omitempty"`
	Risk        string            `json:"risk,omitempty"`
	RequestedAt time.Time         `json:"requested_at,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// RunState is the cross-runtime checkpointable state envelope. It intentionally
// contains provider-neutral fields so agent, team, and workflow code can share
// state without depending on each other's packages.
type RunState struct {
	RunID           string              `json:"run_id"`
	ParentRunID     string              `json:"parent_run_id,omitempty"`
	TraceID         string              `json:"trace_id,omitempty"`
	Scope           RunScope            `json:"scope,omitempty"`
	Status          RunStatus           `json:"status,omitempty"`
	SessionID       string              `json:"session_id,omitempty"`
	ConversationID  string              `json:"conversation_id,omitempty"`
	AgentID         string              `json:"agent_id,omitempty"`
	TeamID          string              `json:"team_id,omitempty"`
	WorkflowID      string              `json:"workflow_id,omitempty"`
	CheckpointID    string              `json:"checkpoint_id,omitempty"`
	MemorySnapshot  []MemoryRecord      `json:"memory_snapshot,omitempty"`
	ToolState       []ToolStateSnapshot `json:"tool_state,omitempty"`
	PendingApproval *RunApprovalState   `json:"pending_approval,omitempty"`
	Metadata        map[string]string   `json:"metadata,omitempty"`
	Tags            []string            `json:"tags,omitempty"`
	StartedAt       time.Time           `json:"started_at,omitempty"`
	UpdatedAt       time.Time           `json:"updated_at,omitempty"`
	CompletedAt     *time.Time          `json:"completed_at,omitempty"`
}
