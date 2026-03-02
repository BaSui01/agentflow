package types

// AgentEventType defines unified agent event names.
type AgentEventType string

const (
	AgentEventStateChange       AgentEventType = "state_change"
	AgentEventToolCall          AgentEventType = "tool_call"
	AgentEventFeedback          AgentEventType = "feedback"
	AgentEventApprovalRequested AgentEventType = "approval_requested"
	AgentEventApprovalResponded AgentEventType = "approval_responded"
	AgentEventSubagentCompleted AgentEventType = "subagent_completed"
	AgentEventRunStart          AgentEventType = "agent_run_start"
	AgentEventRunComplete       AgentEventType = "agent_run_complete"
	AgentEventRunError          AgentEventType = "agent_run_error"
)

// WorkflowNodeEventType defines node-level workflow observability events.
type WorkflowNodeEventType string

const (
	WorkflowNodeEventStart    WorkflowNodeEventType = "node_start"
	WorkflowNodeEventComplete WorkflowNodeEventType = "node_complete"
	WorkflowNodeEventError    WorkflowNodeEventType = "node_error"
)

