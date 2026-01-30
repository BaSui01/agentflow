// Package api provides API types and documentation for AgentFlow.
package api

import (
	"encoding/json"
	"time"
)

// =============================================================================
// Chat Completion Types
// =============================================================================

// ChatRequest represents a chat completion request.
// @Description Chat completion request structure
type ChatRequest struct {
	// Trace ID for request tracking
	TraceID string `json:"trace_id,omitempty" example:"trace-123"`
	// Tenant ID for multi-tenancy
	TenantID string `json:"tenant_id,omitempty" example:"tenant-1"`
	// User ID
	UserID string `json:"user_id,omitempty" example:"user-1"`
	// Model name (e.g., gpt-4, claude-3-opus)
	Model string `json:"model" example:"gpt-4" binding:"required"`
	// Conversation messages
	Messages []Message `json:"messages" binding:"required"`
	// Maximum tokens to generate
	MaxTokens int `json:"max_tokens,omitempty" example:"4096"`
	// Sampling temperature (0-2)
	Temperature float32 `json:"temperature,omitempty" example:"0.7"`
	// Nucleus sampling parameter (0-1)
	TopP float32 `json:"top_p,omitempty" example:"1.0"`
	// Stop sequences
	Stop []string `json:"stop,omitempty"`
	// Available tools for function calling
	Tools []ToolSchema `json:"tools,omitempty"`
	// Tool choice mode (auto, none, or specific tool name)
	ToolChoice string `json:"tool_choice,omitempty" example:"auto"`
	// Request timeout duration
	Timeout string `json:"timeout,omitempty" example:"30s"`
	// Custom metadata
	Metadata map[string]string `json:"metadata,omitempty"`
	// Tags for routing
	Tags []string `json:"tags,omitempty"`
}

// ChatResponse represents a chat completion response.
// @Description Chat completion response structure
type ChatResponse struct {
	// Response ID
	ID string `json:"id,omitempty" example:"chatcmpl-123"`
	// Provider that handled the request
	Provider string `json:"provider,omitempty" example:"openai"`
	// Model used
	Model string `json:"model" example:"gpt-4"`
	// Response choices
	Choices []ChatChoice `json:"choices"`
	// Token usage statistics
	Usage ChatUsage `json:"usage"`
	// Response creation timestamp
	CreatedAt time.Time `json:"created_at"`
}

// ChatChoice represents a single choice in the response.
// @Description Chat choice structure
type ChatChoice struct {
	// Choice index
	Index int `json:"index" example:"0"`
	// Reason for completion (stop, length, tool_calls, content_filter)
	FinishReason string `json:"finish_reason,omitempty" example:"stop"`
	// Response message
	Message Message `json:"message"`
}

// ChatUsage represents token usage in a response.
// @Description Token usage statistics
type ChatUsage struct {
	// Tokens in the prompt
	PromptTokens int `json:"prompt_tokens" example:"100"`
	// Tokens in the completion
	CompletionTokens int `json:"completion_tokens" example:"50"`
	// Total tokens used
	TotalTokens int `json:"total_tokens" example:"150"`
}

// StreamChunk represents a streaming response chunk.
// @Description Streaming response chunk structure
type StreamChunk struct {
	// Chunk ID
	ID string `json:"id,omitempty" example:"chatcmpl-123"`
	// Provider name
	Provider string `json:"provider,omitempty" example:"openai"`
	// Model name
	Model string `json:"model,omitempty" example:"gpt-4"`
	// Choice index
	Index int `json:"index,omitempty" example:"0"`
	// Delta message content
	Delta Message `json:"delta"`
	// Finish reason (only in final chunk)
	FinishReason string `json:"finish_reason,omitempty" example:"stop"`
	// Usage statistics (only in final chunk)
	Usage *ChatUsage `json:"usage,omitempty"`
	// Error information
	Error *ErrorDetail `json:"error,omitempty"`
}

// =============================================================================
// Message Types
// =============================================================================

// Message represents a conversation message.
// @Description Conversation message structure
type Message struct {
	// Message role (system, user, assistant, tool)
	Role string `json:"role" example:"user" binding:"required"`
	// Message content
	Content string `json:"content,omitempty" example:"Hello, how are you?"`
	// Name (for tool messages)
	Name string `json:"name,omitempty"`
	// Tool calls (for assistant messages)
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// Tool call ID (for tool messages)
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Image content for multimodal messages
	Images []ImageContent `json:"images,omitempty"`
	// Custom metadata
	Metadata interface{} `json:"metadata,omitempty"`
	// Message timestamp
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// ToolCall represents a tool invocation request from the LLM.
// @Description Tool call structure
type ToolCall struct {
	// Tool call ID
	ID string `json:"id" example:"call_123"`
	// Tool name
	Name string `json:"name" example:"get_weather"`
	// Tool arguments as JSON
	Arguments json.RawMessage `json:"arguments"`
}

// ImageContent represents image data for multimodal messages.
// @Description Image content structure
type ImageContent struct {
	// Image content type (url or base64)
	Type string `json:"type" example:"url"`
	// Image URL (when type is url)
	URL string `json:"url,omitempty" example:"https://example.com/image.png"`
	// Base64 encoded image data (when type is base64)
	Data string `json:"data,omitempty"`
}

// =============================================================================
// Tool Types
// =============================================================================

// ToolSchema defines a tool's interface for LLM function calling.
// @Description Tool schema structure
type ToolSchema struct {
	// Tool name
	Name string `json:"name" example:"get_weather"`
	// Tool description
	Description string `json:"description,omitempty" example:"Get the current weather for a location"`
	// JSON Schema for tool parameters
	Parameters json.RawMessage `json:"parameters"`
	// Tool version
	Version string `json:"version,omitempty" example:"1.0.0"`
}

// ToolResult represents the result of a tool execution.
// @Description Tool result structure
type ToolResult struct {
	// Tool call ID
	ToolCallID string `json:"tool_call_id" example:"call_123"`
	// Tool name
	Name string `json:"name" example:"get_weather"`
	// Tool result as JSON
	Result json.RawMessage `json:"result"`
	// Error message if execution failed
	Error string `json:"error,omitempty"`
	// Execution duration
	Duration string `json:"duration,omitempty" example:"100ms"`
}

// ToolInvokeRequest represents a request to invoke a tool.
// @Description Tool invocation request
type ToolInvokeRequest struct {
	// Tool arguments
	Arguments json.RawMessage `json:"arguments" binding:"required"`
}

// =============================================================================
// Provider Types
// =============================================================================

// LLMProvider represents an LLM provider.
// @Description LLM provider structure
type LLMProvider struct {
	// Provider ID
	ID uint `json:"id" example:"1"`
	// Provider code (e.g., openai, anthropic)
	Code string `json:"code" example:"openai"`
	// Provider display name
	Name string `json:"name" example:"OpenAI"`
	// Provider description
	Description string `json:"description,omitempty" example:"OpenAI GPT models"`
	// Provider status (0: Inactive, 1: Active, 2: Disabled)
	Status int `json:"status" example:"1"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// LLMModel represents an LLM model.
// @Description LLM model structure
type LLMModel struct {
	// Model ID
	ID uint `json:"id" example:"1"`
	// Model identifier
	ModelName string `json:"model_name" example:"gpt-4"`
	// Display name
	DisplayName string `json:"display_name,omitempty" example:"GPT-4"`
	// Model description
	Description string `json:"description,omitempty"`
	// Whether the model is enabled
	Enabled bool `json:"enabled" example:"true"`
	// Creation timestamp
	CreatedAt time.Time `json:"created_at"`
	// Last update timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// LLMProviderModel represents a provider's model instance.
// @Description Provider model mapping structure
type LLMProviderModel struct {
	// Mapping ID
	ID uint `json:"id" example:"1"`
	// Model ID
	ModelID uint `json:"model_id" example:"1"`
	// Provider ID
	ProviderID uint `json:"provider_id" example:"1"`
	// Model name as known by the provider
	RemoteModelName string `json:"remote_model_name" example:"gpt-4-turbo"`
	// Provider base URL
	BaseURL string `json:"base_url,omitempty" example:"https://api.openai.com"`
	// Price per 1K input tokens
	PriceInput float64 `json:"price_input" example:"0.01"`
	// Price per 1K completion tokens
	PriceCompletion float64 `json:"price_completion" example:"0.03"`
	// Maximum context length
	MaxTokens int `json:"max_tokens" example:"128000"`
	// Priority for routing
	Priority int `json:"priority" example:"100"`
	// Whether the mapping is enabled
	Enabled bool `json:"enabled" example:"true"`
}

// HealthStatus represents provider health check result.
// @Description Provider health status
type HealthStatus struct {
	// Whether the provider is healthy
	Healthy bool `json:"healthy" example:"true"`
	// Response latency
	Latency string `json:"latency" example:"100ms"`
	// Error rate (0-1)
	ErrorRate float64 `json:"error_rate" example:"0.01"`
}

// =============================================================================
// Routing Types
// =============================================================================

// RoutingRequest represents a provider selection request.
// @Description Routing request structure
type RoutingRequest struct {
	// Model name to route
	Model string `json:"model" example:"gpt-4" binding:"required"`
	// Routing strategy (cost, health, qps, canary, tag)
	Strategy string `json:"strategy" example:"cost" binding:"required"`
	// Tags for tag-based routing
	Tags []string `json:"tags,omitempty"`
}

// ProviderSelection represents a selected provider.
// @Description Provider selection result
type ProviderSelection struct {
	// Provider ID
	ProviderID uint `json:"provider_id" example:"1"`
	// Provider code
	ProviderCode string `json:"provider_code" example:"openai"`
	// Model ID
	ModelID uint `json:"model_id" example:"1"`
	// Model name
	ModelName string `json:"model_name" example:"gpt-4"`
	// Whether this is a canary deployment
	IsCanary bool `json:"is_canary" example:"false"`
	// Strategy used for selection
	Strategy string `json:"strategy" example:"cost"`
}

// =============================================================================
// A2A Protocol Types
// =============================================================================

// AgentCard represents an A2A Agent Card.
// @Description A2A Agent Card structure
type AgentCard struct {
	// Agent name
	Name string `json:"name" example:"my-agent"`
	// Agent description
	Description string `json:"description" example:"A helpful AI assistant"`
	// Agent endpoint URL
	URL string `json:"url" example:"http://localhost:8080"`
	// Agent version
	Version string `json:"version" example:"1.0.0"`
	// Agent capabilities
	Capabilities []Capability `json:"capabilities"`
	// JSON Schema for agent input
	InputSchema interface{} `json:"input_schema,omitempty"`
	// JSON Schema for agent output
	OutputSchema interface{} `json:"output_schema,omitempty"`
	// Available tools
	Tools []ToolDefinition `json:"tools,omitempty"`
	// Additional metadata
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Capability defines an agent's capability.
// @Description Agent capability structure
type Capability struct {
	// Capability name
	Name string `json:"name" example:"chat"`
	// Capability description
	Description string `json:"description" example:"Chat with the agent"`
	// Capability type (task, query, stream)
	Type string `json:"type" example:"query"`
}

// ToolDefinition defines a tool that an agent can use.
// @Description Tool definition structure
type ToolDefinition struct {
	// Tool name
	Name string `json:"name" example:"search"`
	// Tool description
	Description string `json:"description" example:"Search the web"`
	// JSON Schema for tool parameters
	Parameters interface{} `json:"parameters,omitempty"`
}

// A2ARequest represents an A2A invocation request.
// @Description A2A request structure
type A2ARequest struct {
	// Task description or query
	Task string `json:"task" example:"What is the weather today?" binding:"required"`
	// Additional context
	Context interface{} `json:"context,omitempty"`
	// Whether to stream the response
	Stream bool `json:"stream,omitempty" example:"false"`
}

// A2AResponse represents an A2A invocation response.
// @Description A2A response structure
type A2AResponse struct {
	// Response status (success, error, pending)
	Status string `json:"status" example:"success"`
	// Task result
	Result interface{} `json:"result,omitempty"`
	// Error message if failed
	Error string `json:"error,omitempty"`
}

// =============================================================================
// Error Types
// =============================================================================

// ErrorResponse represents an error response.
// @Description Error response structure
type ErrorResponse struct {
	// Error details
	Error ErrorDetail `json:"error"`
}

// ErrorDetail represents error details.
// @Description Error detail structure
type ErrorDetail struct {
	// Error code
	Code string `json:"code" example:"INVALID_REQUEST"`
	// Human-readable error message
	Message string `json:"message" example:"Invalid request parameters"`
	// HTTP status code
	HTTPStatus int `json:"http_status,omitempty" example:"400"`
	// Whether the request can be retried
	Retryable bool `json:"retryable,omitempty" example:"false"`
	// Provider that returned the error
	Provider string `json:"provider,omitempty" example:"openai"`
}

// =============================================================================
// List Response Types
// =============================================================================

// ProviderListResponse represents a list of providers.
// @Description Provider list response
type ProviderListResponse struct {
	// List of providers
	Providers []LLMProvider `json:"providers"`
}

// ModelListResponse represents a list of models.
// @Description Model list response
type ModelListResponse struct {
	// List of models
	Models []LLMModel `json:"models"`
}

// ToolListResponse represents a list of tools.
// @Description Tool list response
type ToolListResponse struct {
	// List of tools
	Tools []ToolSchema `json:"tools"`
}
