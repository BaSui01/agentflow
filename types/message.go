package types

import (
	"encoding/json"
	"time"
)

// Role represents the role of a message participant.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	RoleDeveloper Role = "developer" // OpenAI developer instructions role
)

// ToolCall represents a tool invocation request from the LLM.
type ToolCall struct {
	Index     int             `json:"index,omitempty"` // 流式 delta 中标识同一工具调用的位置索引
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ImageContent represents image data for multimodal messages.
type ImageContent struct {
	Type string `json:"type"` // "url" or "base64"
	URL  string `json:"url,omitempty"`
	Data string `json:"data,omitempty"` // base64 encoded
}

// VideoContent represents video data for multimodal messages.
type VideoContent struct {
	URL string   `json:"url"`
	FPS *float64 `json:"fps,omitempty"`
}

// Annotation represents a citation or reference annotation in a message.
type Annotation struct {
	Type       string `json:"type"`                  // "url_citation"
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
	URL        string `json:"url,omitempty"`
	Title      string `json:"title,omitempty"`
}

// ThinkingBlock represents a Claude extended thinking content block.
// Used for round-tripping thinking blocks in multi-turn tool use conversations.
type ThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature,omitempty"`
}

// Message represents a conversation message.
type Message struct {
	Role             Role            `json:"role"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent *string         `json:"reasoning_content,omitempty"` // 推理/思考内容
	ThinkingBlocks   []ThinkingBlock `json:"thinking_blocks,omitempty"`   // Claude thinking blocks（用于多轮 round-trip）
	Refusal          *string         `json:"refusal,omitempty"`           // 模型拒绝内容
	Name             string          `json:"name,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	IsToolError      bool            `json:"is_tool_error,omitempty"` // tool_result 是否为错误结果
	Images           []ImageContent  `json:"images,omitempty"`
	Videos           []VideoContent  `json:"videos,omitempty"`
	Annotations      []Annotation    `json:"annotations,omitempty"` // URL 引用注释
	Metadata         any             `json:"metadata,omitempty"`
	Timestamp        time.Time       `json:"timestamp,omitempty"`
}

// NewMessage creates a new message with the given role and content.
func NewMessage(role Role, content string) Message {
	return Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// NewSystemMessage creates a new system message.
func NewSystemMessage(content string) Message {
	return NewMessage(RoleSystem, content)
}

// NewUserMessage creates a new user message.
func NewUserMessage(content string) Message {
	return NewMessage(RoleUser, content)
}

// NewAssistantMessage creates a new assistant message.
func NewAssistantMessage(content string) Message {
	return NewMessage(RoleAssistant, content)
}

// NewToolMessage creates a new tool result message.
func NewToolMessage(toolCallID, name, content string) Message {
	return Message{
		Role:       RoleTool,
		Content:    content,
		Name:       name,
		ToolCallID: toolCallID,
		Timestamp:  time.Now(),
	}
}

// NewDeveloperMessage creates a new developer instructions message.
func NewDeveloperMessage(content string) Message {
	return NewMessage(RoleDeveloper, content)
}

// WithToolCalls adds tool calls to the message.
func (m Message) WithToolCalls(calls []ToolCall) Message {
	m.ToolCalls = calls
	return m
}

// WithImages adds images to the message.
func (m Message) WithImages(images []ImageContent) Message {
	m.Images = images
	return m
}

// WithMetadata adds metadata to the message.
func (m Message) WithMetadata(metadata any) Message {
	m.Metadata = metadata
	return m
}

