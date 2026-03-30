package llm

import "github.com/BaSui01/agentflow/types"

// Re-export canonical shared types used across llm package internals.
type Message = types.Message
type Error = types.Error
type ToolCall = types.ToolCall
type ToolResult = types.ToolResult
type ToolSchema = types.ToolSchema
type ThinkingBlock = types.ThinkingBlock
type ReasoningSummary = types.ReasoningSummary
type OpaqueReasoning = types.OpaqueReasoning
