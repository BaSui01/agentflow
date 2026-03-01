package handlers

import (
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// ChatConverter centralizes request/response conversion between API and LLM layers.
type ChatConverter interface {
	ToLLMRequest(req *api.ChatRequest) *llm.ChatRequest
	ToAPIResponse(resp *llm.ChatResponse) *api.ChatResponse
	ToAPIChoices(choices []llm.ChatChoice) []api.ChatChoice
	ToAPIUsage(usage llm.ChatUsage) api.ChatUsage
	ToAPIStreamChunk(chunk *llm.StreamChunk) *api.StreamChunk
}

// DefaultChatConverter is the default converter implementation used by ChatHandler.
type DefaultChatConverter struct {
	defaultTimeout time.Duration
}

// NewDefaultChatConverter creates a default converter with fallback timeout.
func NewDefaultChatConverter(defaultTimeout time.Duration) *DefaultChatConverter {
	return &DefaultChatConverter{defaultTimeout: defaultTimeout}
}

// ToLLMRequest converts api.ChatRequest to llm.ChatRequest.
func (c *DefaultChatConverter) ToLLMRequest(req *api.ChatRequest) *llm.ChatRequest {
	timeout := c.defaultTimeout
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = d
		}
	}

	messages := make([]types.Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = types.Message{
			Role:       types.Role(msg.Role),
			Content:    msg.Content,
			Name:       msg.Name,
			ToolCalls:  msg.ToolCalls,
			ToolCallID: msg.ToolCallID,
			Images:     convertAPIImagesToTypes(msg.Images),
			Metadata:   msg.Metadata,
			Timestamp:  msg.Timestamp,
		}
	}

	tools := make([]types.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = types.ToolSchema{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Version:     tool.Version,
		}
	}

	return &llm.ChatRequest{
		TraceID:     req.TraceID,
		TenantID:    req.TenantID,
		UserID:      req.UserID,
		Model:       req.Model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		Timeout:     timeout,
		Metadata:    req.Metadata,
		Tags:        req.Tags,
	}
}

// ToAPIResponse converts llm.ChatResponse to api.ChatResponse.
func (c *DefaultChatConverter) ToAPIResponse(resp *llm.ChatResponse) *api.ChatResponse {
	return &api.ChatResponse{
		ID:        resp.ID,
		Provider:  resp.Provider,
		Model:     resp.Model,
		Choices:   c.ToAPIChoices(resp.Choices),
		Usage:     c.ToAPIUsage(resp.Usage),
		CreatedAt: resp.CreatedAt,
	}
}

// ToAPIChoices converts llm choices to API choices.
func (c *DefaultChatConverter) ToAPIChoices(choices []llm.ChatChoice) []api.ChatChoice {
	result := make([]api.ChatChoice, len(choices))
	for i, choice := range choices {
		result[i] = api.ChatChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Message:      convertTypesMessageToAPI(choice.Message),
		}
	}
	return result
}

// ToAPIUsage converts llm usage to API usage.
func (c *DefaultChatConverter) ToAPIUsage(usage llm.ChatUsage) api.ChatUsage {
	return api.ChatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

// ToAPIStreamChunk converts llm stream chunk to API chunk.
func (c *DefaultChatConverter) ToAPIStreamChunk(chunk *llm.StreamChunk) *api.StreamChunk {
	return &api.StreamChunk{
		ID:           chunk.ID,
		Provider:     chunk.Provider,
		Model:        chunk.Model,
		Index:        chunk.Index,
		Delta:        convertTypesMessageToAPI(chunk.Delta),
		FinishReason: chunk.FinishReason,
		Usage:        convertStreamUsage(chunk.Usage),
	}
}
