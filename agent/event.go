package agent

import (
	"context"
	"time"
)

// EventType 事件类型
type EventType string

const (
	EventStateChange EventType = "agent.state_change" // 状态变更
	EventExecute     EventType = "agent.execute"      // 开始执行
	EventComplete    EventType = "agent.complete"     // 执行完成
	EventError       EventType = "agent.error"        // 执行错误
	EventToolCall    EventType = "agent.tool_call"    // 工具调用
	EventFeedback    EventType = "agent.feedback"     // 反馈事件
)

// Event 基础事件接口
type Event interface {
	EventType() EventType
	AgentID() string
	Timestamp() time.Time
}

// StateChangeEvent 状态变更事件
type StateChangeEvent struct {
	AgentID_   string    `json:"agent_id"`
	FromState  State     `json:"from_state"`
	ToState    State     `json:"to_state"`
	Timestamp_ time.Time `json:"timestamp"`
}

func (e *StateChangeEvent) EventType() EventType { return EventStateChange }
func (e *StateChangeEvent) AgentID() string      { return e.AgentID_ }
func (e *StateChangeEvent) Timestamp() time.Time { return e.Timestamp_ }

// ExecuteEvent 执行事件
type ExecuteEvent struct {
	AgentID_            string    `json:"agent_id"`
	RunID               string    `json:"run_id,omitempty"`
	TraceID             string    `json:"trace_id"`
	PromptBundleVersion string    `json:"prompt_bundle_version,omitempty"`
	Input               *Input    `json:"input,omitempty"`
	Timestamp_          time.Time `json:"timestamp"`
}

func (e *ExecuteEvent) EventType() EventType { return EventExecute }
func (e *ExecuteEvent) AgentID() string      { return e.AgentID_ }
func (e *ExecuteEvent) Timestamp() time.Time { return e.Timestamp_ }

// CompleteEvent 完成事件
type CompleteEvent struct {
	AgentID_            string        `json:"agent_id"`
	RunID               string        `json:"run_id,omitempty"`
	TraceID             string        `json:"trace_id"`
	PromptBundleVersion string        `json:"prompt_bundle_version,omitempty"`
	Output              *Output       `json:"output,omitempty"`
	Duration            time.Duration `json:"duration"`
	Timestamp_          time.Time     `json:"timestamp"`
}

func (e *CompleteEvent) EventType() EventType { return EventComplete }
func (e *CompleteEvent) AgentID() string      { return e.AgentID_ }
func (e *CompleteEvent) Timestamp() time.Time { return e.Timestamp_ }

// ErrorEvent 错误事件
type ErrorEvent struct {
	AgentID_            string    `json:"agent_id"`
	RunID               string    `json:"run_id,omitempty"`
	TraceID             string    `json:"trace_id"`
	PromptBundleVersion string    `json:"prompt_bundle_version,omitempty"`
	Error               string    `json:"error"`
	ErrorCategory       string    `json:"error_category,omitempty"`
	Timestamp_          time.Time `json:"timestamp"`
}

func (e *ErrorEvent) EventType() EventType { return EventError }
func (e *ErrorEvent) AgentID() string      { return e.AgentID_ }
func (e *ErrorEvent) Timestamp() time.Time { return e.Timestamp_ }

// ToolCallEvent 工具调用事件（start/end）。
type ToolCallEvent struct {
	AgentID_            string    `json:"agent_id"`
	RunID               string    `json:"run_id,omitempty"`
	TraceID             string    `json:"trace_id,omitempty"`
	PromptBundleVersion string    `json:"prompt_bundle_version,omitempty"`
	ToolCallID          string    `json:"tool_call_id,omitempty"`
	ToolName            string    `json:"tool_name"`
	Stage               string    `json:"stage"` // start/end
	Error               string    `json:"error,omitempty"`
	Timestamp_          time.Time `json:"timestamp"`
}

func (e *ToolCallEvent) EventType() EventType { return EventToolCall }
func (e *ToolCallEvent) AgentID() string      { return e.AgentID_ }
func (e *ToolCallEvent) Timestamp() time.Time { return e.Timestamp_ }

// EventHandler 事件处理函数
type EventHandler func(ctx context.Context, event Event) error

// EventBus 事件总线接口
type EventBus interface {
	// Publish 发布事件
	Publish(ctx context.Context, eventType EventType, event Event) error
	// Subscribe 订阅事件
	Subscribe(ctx context.Context, eventType EventType, handler EventHandler) error
	// Unsubscribe 取消订阅
	Unsubscribe(eventType EventType, handler EventHandler) error
}

// FeedbackEvent 反馈事件
type FeedbackEvent struct {
	AgentID_   string         `json:"agent_id"`
	Type       string         `json:"type"` // approval/rejection/correction
	Content    string         `json:"content"`
	Data       map[string]any `json:"data,omitempty"`
	Timestamp_ time.Time      `json:"timestamp"`
}

func (e *FeedbackEvent) EventType() EventType { return EventFeedback }
func (e *FeedbackEvent) AgentID() string      { return e.AgentID_ }
func (e *FeedbackEvent) Timestamp() time.Time { return e.Timestamp_ }
