package types

// ExecutionStatus represents the unified execution status across agent and workflow layers.
// This replaces the cross-layer dependency on agent/persistence.TaskStatus.
type ExecutionStatus string

const (
	ExecutionStatusPending   ExecutionStatus = "pending"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusCompleted ExecutionStatus = "completed"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
	ExecutionStatusTimeout   ExecutionStatus = "timeout"
)

// IsTerminal returns true if the status represents a terminal state.
func (s ExecutionStatus) IsTerminal() bool {
	switch s {
	case ExecutionStatusCompleted, ExecutionStatusFailed, ExecutionStatusCancelled, ExecutionStatusTimeout:
		return true
	default:
		return false
	}
}
