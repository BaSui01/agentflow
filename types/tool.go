package types

import (
	"encoding/json"
	"time"
)

// ToolSchema defines a tool's interface for LLM function calling.
type ToolSchema struct {
	Type         string          `json:"type,omitempty"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Parameters   json.RawMessage `json:"parameters"`
	ResultSchema json.RawMessage `json:"result_schema,omitempty"`
	RiskTier     RiskTier        `json:"risk_tier,omitempty"`
	Format       *ToolFormat     `json:"format,omitempty"`
	Strict       *bool           `json:"strict,omitempty"`
	Version      string          `json:"version,omitempty"`
}

type ToolRetryPolicy struct {
	MaxRetries     int           `json:"max_retries"`
	InitialBackoff time.Duration `json:"initial_backoff"`
	MaxBackoff     time.Duration `json:"max_backoff"`
	RetryableCodes []ErrorCode   `json:"retryable_codes,omitempty"`
}

// ToolFormat defines provider-native formatting constraints for custom tools.
type ToolFormat struct {
	Type       string `json:"type"`
	Syntax     string `json:"syntax,omitempty"`
	Definition string `json:"definition,omitempty"`
}

// Normalized tool types.
const (
	ToolTypeFunction = "function"
	ToolTypeCustom   = "custom"
)

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration"`
	FromCache  bool            `json:"from_cache,omitempty"`
}

// ToMessage converts ToolResult to a Message.
func (tr ToolResult) ToMessage() Message {
	content := string(tr.Result)
	isErr := tr.Error != ""
	if isErr {
		content = "Error: " + tr.Error
	}
	return Message{
		Role:        RoleTool,
		Content:     content,
		Name:        tr.Name,
		ToolCallID:  tr.ToolCallID,
		IsToolError: isErr,
	}
}

// IsError returns true if the tool execution failed.
func (tr ToolResult) IsError() bool {
	return tr.Error != ""
}

// ToJSON returns a JSON string representation of the ToolResult.
func (tr ToolResult) ToJSON() string {
	data, err := json.Marshal(tr)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// =============================================================================
// Steering Types（实时引导/停止后发送）
// =============================================================================

// SteeringMessageType 区分引导和停止后发送
type SteeringMessageType string

const (
	// SteeringTypeGuide 保留已生成内容，追加引导指令后重新发起流式调用
	SteeringTypeGuide SteeringMessageType = "guide"
	// SteeringTypeStopAndSend 丢弃已生成内容，用新消息替换后重新发起流式调用
	SteeringTypeStopAndSend SteeringMessageType = "stop_and_send"
)

// SteeringMessage 用户中途注入的消息
type SteeringMessage struct {
	Type      SteeringMessageType `json:"type"`
	Content   string              `json:"content"`
	Timestamp time.Time           `json:"timestamp"`
}

// IsZero 检查是否为零值消息（channel 关闭后产生）
func (m SteeringMessage) IsZero() bool {
	return m.Type == "" && m.Content == ""
}

// ApplySteeringToMessages 根据 steering 消息类型变换 messages 数组。
// partialContent 和 reasoningContent 是被中断时已生成的部分内容。
// assistantRole 应传入 "assistant" 常量（避免 types 包依赖 llm 包）。
func ApplySteeringToMessages(msg SteeringMessage, messages []Message, partialContent, reasoningContent string, assistantRole Role) []Message {
	switch msg.Type {
	case SteeringTypeGuide:
		if partialContent != "" {
			assistantMsg := Message{Role: assistantRole, Content: partialContent}
			if reasoningContent != "" {
				assistantMsg.ReasoningContent = &reasoningContent
			}
			messages = append(messages, assistantMsg)
		}
		messages = append(messages,
			Message{Role: RoleUser, Content: msg.Content},
		)
	case SteeringTypeStopAndSend:
		// 从末尾向前找到最后一条 user 消息并替换
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == RoleUser {
				messages[i] = Message{Role: RoleUser, Content: msg.Content}
				break
			}
		}
	}
	return messages
}
