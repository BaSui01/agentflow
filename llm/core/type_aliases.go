package core

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

type ChatRequest = types.ChatRequest
type ChatResponse = types.ChatResponse
type ChatChoice = types.ChatChoice
type ChatUsage = types.ChatUsage
type StreamChunk = types.StreamChunk
type PromptTokensDetails = types.PromptTokensDetails
type CompletionTokensDetails = types.CompletionTokensDetails
type ResponseFormat = types.ResponseFormat
type ResponseFormatType = types.ResponseFormatType
type JSONSchemaParam = types.JSONSchemaParam
type StreamOptions = types.StreamOptions
type CacheControl = types.CacheControl
type WebSearchOptions = types.WebSearchOptions
type WebSearchLocation = types.WebSearchLocation
type ToolCallMode = types.ToolCallMode

const (
	RoleSystem    = types.RoleSystem
	RoleUser      = types.RoleUser
	RoleAssistant = types.RoleAssistant
	RoleTool      = types.RoleTool
	RoleDeveloper = types.RoleDeveloper
)

const (
	ResponseFormatText       = types.ResponseFormatText
	ResponseFormatJSONObject = types.ResponseFormatJSONObject
	ResponseFormatJSONSchema = types.ResponseFormatJSONSchema
)

const (
	ToolCallModeNative = types.ToolCallModeNative
	ToolCallModeXML    = types.ToolCallModeXML
)

const (
	ErrInvalidRequest      = types.ErrInvalidRequest
	ErrAuthentication      = types.ErrAuthentication
	ErrUnauthorized        = types.ErrUnauthorized
	ErrForbidden           = types.ErrForbidden
	ErrRateLimit           = types.ErrRateLimit
	ErrQuotaExceeded       = types.ErrQuotaExceeded
	ErrModelNotFound       = types.ErrModelNotFound
	ErrModelOverloaded     = types.ErrModelOverloaded
	ErrContextTooLong      = types.ErrContextTooLong
	ErrContentFiltered     = types.ErrContentFiltered
	ErrUpstreamError       = types.ErrUpstreamError
	ErrUpstreamTimeout     = types.ErrUpstreamTimeout
	ErrTimeout             = types.ErrTimeout
	ErrInternalError       = types.ErrInternalError
	ErrServiceUnavailable  = types.ErrServiceUnavailable
	ErrProviderUnavailable = types.ErrProviderUnavailable
)
