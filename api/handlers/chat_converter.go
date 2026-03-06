package handlers

import (
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/types"
)

// ChatConverter centralizes request/response conversion between API and LLM layers.
// Implements usecase.ChatConverter (ToLLMRequest, ToAPIResponse) and extends with stream/choices/usage.
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
			Role:             types.Role(msg.Role),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			ThinkingBlocks:   msg.ThinkingBlocks,
			Refusal:          msg.Refusal,
			Name:             msg.Name,
			ToolCalls:        msg.ToolCalls,
			ToolCallID:       msg.ToolCallID,
			IsToolError:      msg.IsToolError,
			Images:           convertAPIImagesToTypes(msg.Images),
			Videos:           msg.Videos,
			Annotations:      msg.Annotations,
			Metadata:         msg.Metadata,
			Timestamp:        msg.Timestamp,
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
		TraceID:             req.TraceID,
		TenantID:            req.TenantID,
		UserID:              req.UserID,
		Model:               req.Model,
		Messages:            messages,
		MaxTokens:           req.MaxTokens,
		Temperature:         req.Temperature,
		TopP:                req.TopP,
		FrequencyPenalty:    req.FrequencyPenalty,
		PresencePenalty:     req.PresencePenalty,
		RepetitionPenalty:   req.RepetitionPenalty,
		N:                   req.N,
		LogProbs:            req.LogProbs,
		TopLogProbs:         req.TopLogProbs,
		Stop:                req.Stop,
		Tools:               tools,
		ToolChoice:          req.ToolChoice,
		ResponseFormat:      convertAPIResponseFormat(req.ResponseFormat),
		ParallelToolCalls:   req.ParallelToolCalls,
		ServiceTier:         req.ServiceTier,
		User:                req.User,
		StreamOptions:       convertAPIStreamOptions(req.StreamOptions),
		MaxCompletionTokens: req.MaxCompletionTokens,
		ReasoningEffort:     req.ReasoningEffort,
		Store:               req.Store,
		Modalities:          req.Modalities,
		WebSearchOptions:    convertAPIWebSearchOptions(req.WebSearchOptions),
		ReasoningMode:       req.Metadata["reasoning_mode"],
		PreviousResponseID:  req.PreviousResponseID,
		Timeout:             timeout,
		Metadata:            req.Metadata,
		Tags:                req.Tags,
		Include:             req.Include,
		Truncation:          req.Truncation,
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

func convertAPIResponseFormat(in *api.ResponseFormat) *llm.ResponseFormat {
	if in == nil {
		return nil
	}
	out := &llm.ResponseFormat{
		Type: llm.ResponseFormatType(in.Type),
	}
	if in.JSONSchema != nil {
		out.JSONSchema = &llm.JSONSchemaParam{
			Name:        in.JSONSchema.Name,
			Description: in.JSONSchema.Description,
			Schema:      in.JSONSchema.Schema,
			Strict:      in.JSONSchema.Strict,
		}
	}
	return out
}

func convertAPIStreamOptions(in *api.StreamOptions) *llm.StreamOptions {
	if in == nil {
		return nil
	}
	return &llm.StreamOptions{
		IncludeUsage:      in.IncludeUsage,
		ChunkIncludeUsage: in.ChunkIncludeUsage,
	}
}

func convertAPIWebSearchOptions(in *api.WebSearchOptions) *llm.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &llm.WebSearchOptions{
		SearchContextSize: in.SearchContextSize,
		AllowedDomains:    append([]string(nil), in.AllowedDomains...),
	}
	if in.UserLocation != nil {
		out.UserLocation = &llm.WebSearchLocation{
			Type:     in.UserLocation.Type,
			Country:  in.UserLocation.Country,
			Region:   in.UserLocation.Region,
			City:     in.UserLocation.City,
			Timezone: in.UserLocation.Timezone,
		}
	}
	return out
}
