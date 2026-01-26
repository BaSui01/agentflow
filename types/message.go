// Package types provides core types used across the agentflow framework.
// This package has ZERO dependencies on other agentflow packages to avoid circular imports.
// All other packages should import types from here.
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
)

// ToolCall represents a tool invocation request from the LLM.
type ToolCall struct {
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

// Message represents a conversation message.
type Message struct {
	Role       Role           `json:"role"`
	Content    string         `json:"content,omitempty"`
	Name       string         `json:"name,omitempty"`
	ToolCalls  []ToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Images     []ImageContent `json:"images,omitempty"`
	Metadata   any            `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp,omitempty"`
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
