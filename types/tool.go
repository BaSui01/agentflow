package types

import (
	"encoding/json"
	"time"
)

// ToolSchema defines a tool's interface for LLM function calling.
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Version     string          `json:"version,omitempty"`
}

// ToolResult represents the result of a tool execution.
type ToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Name       string          `json:"name"`
	Result     json.RawMessage `json:"result"`
	Error      string          `json:"error,omitempty"`
	Duration   time.Duration   `json:"duration"`
}

// ToMessage converts ToolResult to a Message.
func (tr ToolResult) ToMessage() Message {
	content := string(tr.Result)
	if tr.Error != "" {
		content = "Error: " + tr.Error
	}
	return Message{
		Role:       RoleTool,
		Content:    content,
		Name:       tr.Name,
		ToolCallID: tr.ToolCallID,
	}
}

// IsError returns true if the tool execution failed.
func (tr ToolResult) IsError() bool {
	return tr.Error != ""
}
