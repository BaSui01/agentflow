package handlers

import (
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	llm "github.com/BaSui01/agentflow/llm/core"
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
	ToAPIStreamChunkFromUsecase(chunk *usecase.ChatStreamChunk) *api.StreamChunk
	ToUsecaseRequest(req *api.ChatRequest) *usecase.ChatRequest
	ToAPIResponseFromUsecase(resp *usecase.ChatResponse) *api.ChatResponse
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
			Role:               types.Role(msg.Role),
			Content:            msg.Content,
			ReasoningContent:   msg.ReasoningContent,
			ReasoningSummaries: msg.ReasoningSummaries,
			OpaqueReasoning:    msg.OpaqueReasoning,
			ThinkingBlocks:     msg.ThinkingBlocks,
			Refusal:            msg.Refusal,
			Name:               msg.Name,
			ToolCalls:          msg.ToolCalls,
			ToolCallID:         msg.ToolCallID,
			IsToolError:        msg.IsToolError,
			Images:             convertAPIImagesToTypes(msg.Images),
			Videos:             msg.Videos,
			Annotations:        msg.Annotations,
			Metadata:           msg.Metadata,
			Timestamp:          msg.Timestamp,
		}
	}

	tools := make([]types.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = types.ToolSchema{
			Type:        tool.Type,
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
			Format:      convertAPIToolFormat(tool.Format),
			Strict:      tool.Strict,
			Version:     tool.Version,
		}
	}

	return &llm.ChatRequest{
		TraceID:                          req.TraceID,
		TenantID:                         req.TenantID,
		UserID:                           req.UserID,
		Model:                            req.Model,
		Messages:                         messages,
		MaxTokens:                        req.MaxTokens,
		Temperature:                      req.Temperature,
		TopP:                             req.TopP,
		FrequencyPenalty:                 req.FrequencyPenalty,
		PresencePenalty:                  req.PresencePenalty,
		RepetitionPenalty:                req.RepetitionPenalty,
		N:                                req.N,
		LogProbs:                         req.LogProbs,
		TopLogProbs:                      req.TopLogProbs,
		Stop:                             req.Stop,
		Tools:                            tools,
		ToolChoice:                       api.ParseToolChoice(req.ToolChoice),
		ResponseFormat:                   convertAPIResponseFormat(req.ResponseFormat),
		ParallelToolCalls:                req.ParallelToolCalls,
		ServiceTier:                      req.ServiceTier,
		User:                             req.User,
		StreamOptions:                    convertAPIStreamOptions(req.StreamOptions),
		MaxCompletionTokens:              req.MaxCompletionTokens,
		ReasoningEffort:                  req.ReasoningEffort,
		ReasoningSummary:                 req.ReasoningSummary,
		ReasoningDisplay:                 req.ReasoningDisplay,
		InferenceSpeed:                   req.InferenceSpeed,
		Store:                            req.Store,
		Modalities:                       req.Modalities,
		WebSearchOptions:                 convertAPIWebSearchOptions(req.WebSearchOptions),
		PromptCacheKey:                   req.PromptCacheKey,
		PromptCacheRetention:             req.PromptCacheRetention,
		CacheControl:                     convertAPICacheControl(req.CacheControl),
		CachedContent:                    req.CachedContent,
		IncludeServerSideToolInvocations: req.IncludeServerSideToolInvocations,
		ReasoningMode:                    req.Metadata["reasoning_mode"],
		PreviousResponseID:               req.PreviousResponseID,
		ConversationID:                   req.ConversationID,
		Timeout:                          timeout,
		Metadata:                         req.Metadata,
		Tags:                             req.Tags,
		Include:                          req.Include,
		Truncation:                       req.Truncation,
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
	out := api.ChatUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if usage.PromptTokensDetails != nil {
		out.PromptTokensDetails = &api.PromptTokensDetails{
			CachedTokens:        usage.PromptTokensDetails.CachedTokens,
			CacheCreationTokens: usage.PromptTokensDetails.CacheCreationTokens,
			AudioTokens:         usage.PromptTokensDetails.AudioTokens,
		}
	}
	if usage.CompletionTokensDetails != nil {
		out.CompletionTokensDetails = &api.CompletionTokensDetails{
			ReasoningTokens:          usage.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:              usage.CompletionTokensDetails.AudioTokens,
			AcceptedPredictionTokens: usage.CompletionTokensDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: usage.CompletionTokensDetails.RejectedPredictionTokens,
		}
	}
	return out
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

func (c *DefaultChatConverter) ToAPIStreamChunkFromUsecase(chunk *usecase.ChatStreamChunk) *api.StreamChunk {
	if chunk == nil {
		return nil
	}
	out := &api.StreamChunk{
		ID:           chunk.ID,
		Provider:     chunk.Provider,
		Model:        chunk.Model,
		Index:        chunk.Index,
		Delta:        convertUsecaseMessageToAPI(chunk.Delta),
		FinishReason: chunk.FinishReason,
	}
	if chunk.Usage != nil {
		usage := convertUsecaseUsageToAPI(*chunk.Usage)
		out.Usage = &usage
	}
	return out
}

// convertToLLMRequest 转换为 LLM 请求
func (h *ChatHandler) convertToLLMRequest(req *api.ChatRequest) *llm.ChatRequest {
	return h.converter.ToLLMRequest(req)
}

// convertStreamUsage safely converts *llm.ChatUsage to *api.ChatUsage
// without relying on unsafe pointer casts between distinct types.
func convertStreamUsage(u *llm.ChatUsage) *api.ChatUsage {
	if u == nil {
		return nil
	}
	out := &api.ChatUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		out.PromptTokensDetails = &api.PromptTokensDetails{
			CachedTokens:        u.PromptTokensDetails.CachedTokens,
			CacheCreationTokens: u.PromptTokensDetails.CacheCreationTokens,
			AudioTokens:         u.PromptTokensDetails.AudioTokens,
		}
	}
	if u.CompletionTokensDetails != nil {
		out.CompletionTokensDetails = &api.CompletionTokensDetails{
			ReasoningTokens:          u.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:              u.CompletionTokensDetails.AudioTokens,
			AcceptedPredictionTokens: u.CompletionTokensDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: u.CompletionTokensDetails.RejectedPredictionTokens,
		}
	}
	return out
}

func (c *DefaultChatConverter) ToUsecaseRequest(req *api.ChatRequest) *usecase.ChatRequest {
	if req == nil {
		return nil
	}

	messages := make([]usecase.Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = usecase.Message{
			Role:               string(msg.Role),
			Content:            msg.Content,
			ReasoningContent:   msg.ReasoningContent,
			ReasoningSummaries: msg.ReasoningSummaries,
			OpaqueReasoning:    msg.OpaqueReasoning,
			ThinkingBlocks:     msg.ThinkingBlocks,
			Refusal:            msg.Refusal,
			Name:               msg.Name,
			ToolCalls:          msg.ToolCalls,
			ToolCallID:         msg.ToolCallID,
			IsToolError:        msg.IsToolError,
			Images:             convertAPIImagesToUsecase(msg.Images),
			Videos:             msg.Videos,
			Annotations:        msg.Annotations,
			Metadata:           msg.Metadata,
			Timestamp:          msg.Timestamp,
		}
	}

	tools := make([]usecase.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = usecase.ToolSchema{
			Type:        tool.Type,
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  cloneJSON(tool.Parameters),
			Format:      convertAPIToolFormatToUsecase(tool.Format),
			Strict:      tool.Strict,
			Version:     tool.Version,
		}
	}

	return &usecase.ChatRequest{
		TraceID:                          req.TraceID,
		TenantID:                         req.TenantID,
		UserID:                           req.UserID,
		Model:                            req.Model,
		Provider:                         req.Provider,
		RoutePolicy:                      req.RoutePolicy,
		EndpointMode:                     req.EndpointMode,
		Messages:                         messages,
		MaxTokens:                        req.MaxTokens,
		Temperature:                      req.Temperature,
		TopP:                             req.TopP,
		FrequencyPenalty:                 req.FrequencyPenalty,
		PresencePenalty:                  req.PresencePenalty,
		RepetitionPenalty:                req.RepetitionPenalty,
		N:                                req.N,
		LogProbs:                         req.LogProbs,
		TopLogProbs:                      req.TopLogProbs,
		Stop:                             append([]string(nil), req.Stop...),
		Tools:                            tools,
		ToolChoice:                       req.ToolChoice,
		ResponseFormat:                   convertAPIResponseFormatToUsecase(req.ResponseFormat),
		StreamOptions:                    convertAPIStreamOptionsToUsecase(req.StreamOptions),
		ParallelToolCalls:                req.ParallelToolCalls,
		ServiceTier:                      req.ServiceTier,
		User:                             req.User,
		MaxCompletionTokens:              req.MaxCompletionTokens,
		ReasoningEffort:                  req.ReasoningEffort,
		ReasoningSummary:                 req.ReasoningSummary,
		ReasoningDisplay:                 req.ReasoningDisplay,
		InferenceSpeed:                   req.InferenceSpeed,
		Store:                            req.Store,
		Modalities:                       append([]string(nil), req.Modalities...),
		WebSearchOptions:                 convertAPIWebSearchOptionsToUsecase(req.WebSearchOptions),
		PromptCacheKey:                   req.PromptCacheKey,
		PromptCacheRetention:             req.PromptCacheRetention,
		CacheControl:                     convertAPICacheControlToUsecase(req.CacheControl),
		CachedContent:                    req.CachedContent,
		IncludeServerSideToolInvocations: req.IncludeServerSideToolInvocations,
		PreviousResponseID:               req.PreviousResponseID,
		ConversationID:                   req.ConversationID,
		Include:                          append([]string(nil), req.Include...),
		Truncation:                       req.Truncation,
		Timeout:                          req.Timeout,
		Metadata:                         cloneStringMap(req.Metadata),
		Tags:                             append([]string(nil), req.Tags...),
	}
}

func (c *DefaultChatConverter) ToAPIResponseFromUsecase(resp *usecase.ChatResponse) *api.ChatResponse {
	if resp == nil {
		return nil
	}

	choices := make([]api.ChatChoice, len(resp.Choices))
	for i, choice := range resp.Choices {
		choices[i] = api.ChatChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Message:      convertUsecaseMessageToAPI(choice.Message),
		}
	}

	out := &api.ChatResponse{
		ID:        resp.ID,
		Provider:  resp.Provider,
		Model:     resp.Model,
		Choices:   choices,
		Usage:     convertUsecaseUsageToAPI(resp.Usage),
		CreatedAt: resp.CreatedAt,
	}
	return out
}

type usecaseChatConverter struct {
	base ChatConverter
}

func NewUsecaseChatConverter(base ChatConverter) usecase.ChatConverter {
	return &usecaseChatConverter{base: base}
}

func newUsecaseChatConverter(base ChatConverter) usecase.ChatConverter {
	return NewUsecaseChatConverter(base)
}

func (c *usecaseChatConverter) ToLLMRequest(req *usecase.ChatRequest) *llm.ChatRequest {
	return c.base.ToLLMRequest(convertUsecaseRequestToAPI(req))
}

func (c *usecaseChatConverter) ToChatResponse(resp *llm.ChatResponse) *usecase.ChatResponse {
	return convertAPIResponseToUsecase(c.base.ToAPIResponse(resp))
}
