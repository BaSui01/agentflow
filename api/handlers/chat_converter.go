package handlers

import (
	"encoding/json"
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
		BlockedDomains:    append([]string(nil), in.BlockedDomains...),
		MaxUses:           in.MaxUses,
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

func convertAPICacheControl(in *api.CacheControl) *llm.CacheControl {
	if in == nil {
		return nil
	}
	return &llm.CacheControl{
		Type: in.Type,
		TTL:  in.TTL,
	}
}

func convertAPIToolFormatToUsecase(in *api.ToolFormat) *usecase.ToolFormat {
	if in == nil {
		return nil
	}
	return &usecase.ToolFormat{
		Type:       in.Type,
		Syntax:     in.Syntax,
		Definition: in.Definition,
	}
}

func convertAPIResponseFormatToUsecase(in *api.ResponseFormat) *usecase.ResponseFormat {
	if in == nil {
		return nil
	}
	out := &usecase.ResponseFormat{Type: in.Type}
	if in.JSONSchema != nil {
		out.JSONSchema = &usecase.ResponseFormatJSONSchema{
			Name:        in.JSONSchema.Name,
			Description: in.JSONSchema.Description,
			Schema:      in.JSONSchema.Schema,
			Strict:      in.JSONSchema.Strict,
		}
	}
	return out
}

func convertAPIStreamOptionsToUsecase(in *api.StreamOptions) *usecase.StreamOptions {
	if in == nil {
		return nil
	}
	return &usecase.StreamOptions{
		IncludeUsage:      in.IncludeUsage,
		ChunkIncludeUsage: in.ChunkIncludeUsage,
	}
}

func convertAPIWebSearchOptionsToUsecase(in *api.WebSearchOptions) *usecase.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &usecase.WebSearchOptions{
		SearchContextSize: in.SearchContextSize,
		AllowedDomains:    append([]string(nil), in.AllowedDomains...),
		BlockedDomains:    append([]string(nil), in.BlockedDomains...),
		MaxUses:           in.MaxUses,
	}
	if in.UserLocation != nil {
		out.UserLocation = &usecase.WebSearchLocation{
			Type:     in.UserLocation.Type,
			Country:  in.UserLocation.Country,
			Region:   in.UserLocation.Region,
			City:     in.UserLocation.City,
			Timezone: in.UserLocation.Timezone,
		}
	}
	return out
}

func convertAPICacheControlToUsecase(in *api.CacheControl) *usecase.CacheControl {
	if in == nil {
		return nil
	}
	return &usecase.CacheControl{
		Type: in.Type,
		TTL:  in.TTL,
	}
}

func convertAPIImagesToUsecase(in []api.ImageContent) []usecase.ImageContent {
	if len(in) == 0 {
		return nil
	}
	out := make([]usecase.ImageContent, len(in))
	for i, img := range in {
		out[i] = usecase.ImageContent{
			Type: img.Type,
			URL:  img.URL,
			Data: img.Data,
		}
	}
	return out
}

func convertUsecaseRequestToAPI(req *usecase.ChatRequest) *api.ChatRequest {
	if req == nil {
		return nil
	}
	messages := make([]api.Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = convertUsecaseMessageToAPI(msg)
	}
	tools := make([]api.ToolSchema, len(req.Tools))
	for i, tool := range req.Tools {
		tools[i] = api.ToolSchema{
			Type:        tool.Type,
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  cloneJSON(tool.Parameters),
			Format:      convertUsecaseToolFormatToAPI(tool.Format),
			Strict:      tool.Strict,
			Version:     tool.Version,
		}
	}
	out := &api.ChatRequest{
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
		ResponseFormat:                   convertUsecaseResponseFormatToAPI(req.ResponseFormat),
		StreamOptions:                    convertUsecaseStreamOptionsToAPI(req.StreamOptions),
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
		WebSearchOptions:                 convertUsecaseWebSearchOptionsToAPI(req.WebSearchOptions),
		PromptCacheKey:                   req.PromptCacheKey,
		PromptCacheRetention:             req.PromptCacheRetention,
		CacheControl:                     convertUsecaseCacheControlToAPI(req.CacheControl),
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
	return out
}

func convertAPIResponseToUsecase(resp *api.ChatResponse) *usecase.ChatResponse {
	if resp == nil {
		return nil
	}
	choices := make([]usecase.ChatChoice, len(resp.Choices))
	for i, choice := range resp.Choices {
		choices[i] = usecase.ChatChoice{
			Index:        choice.Index,
			FinishReason: choice.FinishReason,
			Message:      convertAPIMessageToUsecase(choice.Message),
		}
	}
	out := &usecase.ChatResponse{
		ID:        resp.ID,
		Provider:  resp.Provider,
		Model:     resp.Model,
		Choices:   choices,
		Usage:     convertAPIUsageToUsecase(resp.Usage),
		CreatedAt: resp.CreatedAt,
	}
	return out
}

func convertAPIUsageToUsecase(in api.ChatUsage) usecase.ChatUsage {
	out := usecase.ChatUsage{
		PromptTokens:     in.PromptTokens,
		CompletionTokens: in.CompletionTokens,
		TotalTokens:      in.TotalTokens,
	}
	if in.PromptTokensDetails != nil {
		out.PromptTokensDetails = &usecase.PromptTokensDetails{
			CachedTokens:        in.PromptTokensDetails.CachedTokens,
			CacheCreationTokens: in.PromptTokensDetails.CacheCreationTokens,
			AudioTokens:         in.PromptTokensDetails.AudioTokens,
		}
	}
	if in.CompletionTokensDetails != nil {
		out.CompletionTokensDetails = &usecase.CompletionTokensDetails{
			ReasoningTokens:          in.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:              in.CompletionTokensDetails.AudioTokens,
			AcceptedPredictionTokens: in.CompletionTokensDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: in.CompletionTokensDetails.RejectedPredictionTokens,
		}
	}
	return out
}

func convertUsecaseUsageToAPI(in usecase.ChatUsage) api.ChatUsage {
	out := api.ChatUsage{
		PromptTokens:     in.PromptTokens,
		CompletionTokens: in.CompletionTokens,
		TotalTokens:      in.TotalTokens,
	}
	if in.PromptTokensDetails != nil {
		out.PromptTokensDetails = &api.PromptTokensDetails{
			CachedTokens:        in.PromptTokensDetails.CachedTokens,
			CacheCreationTokens: in.PromptTokensDetails.CacheCreationTokens,
			AudioTokens:         in.PromptTokensDetails.AudioTokens,
		}
	}
	if in.CompletionTokensDetails != nil {
		out.CompletionTokensDetails = &api.CompletionTokensDetails{
			ReasoningTokens:          in.CompletionTokensDetails.ReasoningTokens,
			AudioTokens:              in.CompletionTokensDetails.AudioTokens,
			AcceptedPredictionTokens: in.CompletionTokensDetails.AcceptedPredictionTokens,
			RejectedPredictionTokens: in.CompletionTokensDetails.RejectedPredictionTokens,
		}
	}
	return out
}

func convertAPIMessageToUsecase(in api.Message) usecase.Message {
	return usecase.Message{
		Role:               string(in.Role),
		Content:            in.Content,
		ReasoningContent:   in.ReasoningContent,
		ReasoningSummaries: in.ReasoningSummaries,
		OpaqueReasoning:    in.OpaqueReasoning,
		ThinkingBlocks:     in.ThinkingBlocks,
		Refusal:            in.Refusal,
		Name:               in.Name,
		ToolCalls:          in.ToolCalls,
		ToolCallID:         in.ToolCallID,
		IsToolError:        in.IsToolError,
		Images:             convertAPIImagesToUsecase(in.Images),
		Videos:             in.Videos,
		Annotations:        in.Annotations,
		Metadata:           in.Metadata,
		Timestamp:          in.Timestamp,
	}
}

func convertUsecaseMessageToAPI(in usecase.Message) api.Message {
	return api.Message{
		Role:               in.Role,
		Content:            in.Content,
		ReasoningContent:   in.ReasoningContent,
		ReasoningSummaries: in.ReasoningSummaries,
		OpaqueReasoning:    in.OpaqueReasoning,
		ThinkingBlocks:     in.ThinkingBlocks,
		Refusal:            in.Refusal,
		Name:               in.Name,
		ToolCalls:          in.ToolCalls,
		ToolCallID:         in.ToolCallID,
		IsToolError:        in.IsToolError,
		Images:             convertUsecaseImagesToAPI(in.Images),
		Videos:             in.Videos,
		Annotations:        in.Annotations,
		Metadata:           in.Metadata,
		Timestamp:          in.Timestamp,
	}
}

func convertUsecaseImagesToAPI(in []usecase.ImageContent) []api.ImageContent {
	if len(in) == 0 {
		return nil
	}
	out := make([]api.ImageContent, len(in))
	for i, img := range in {
		out[i] = api.ImageContent{
			Type: img.Type,
			URL:  img.URL,
			Data: img.Data,
		}
	}
	return out
}

func convertUsecaseToolFormatToAPI(in *usecase.ToolFormat) *api.ToolFormat {
	if in == nil {
		return nil
	}
	return &api.ToolFormat{
		Type:       in.Type,
		Syntax:     in.Syntax,
		Definition: in.Definition,
	}
}

func convertUsecaseResponseFormatToAPI(in *usecase.ResponseFormat) *api.ResponseFormat {
	if in == nil {
		return nil
	}
	out := &api.ResponseFormat{Type: in.Type}
	if in.JSONSchema != nil {
		out.JSONSchema = &api.ResponseFormatJSONSchema{
			Name:        in.JSONSchema.Name,
			Description: in.JSONSchema.Description,
			Schema:      in.JSONSchema.Schema,
			Strict:      in.JSONSchema.Strict,
		}
	}
	return out
}

func convertUsecaseStreamOptionsToAPI(in *usecase.StreamOptions) *api.StreamOptions {
	if in == nil {
		return nil
	}
	return &api.StreamOptions{
		IncludeUsage:      in.IncludeUsage,
		ChunkIncludeUsage: in.ChunkIncludeUsage,
	}
}

func convertUsecaseWebSearchOptionsToAPI(in *usecase.WebSearchOptions) *api.WebSearchOptions {
	if in == nil {
		return nil
	}
	out := &api.WebSearchOptions{
		SearchContextSize: in.SearchContextSize,
		AllowedDomains:    append([]string(nil), in.AllowedDomains...),
		BlockedDomains:    append([]string(nil), in.BlockedDomains...),
		MaxUses:           in.MaxUses,
	}
	if in.UserLocation != nil {
		out.UserLocation = &api.WebSearchLocation{
			Type:     in.UserLocation.Type,
			Country:  in.UserLocation.Country,
			Region:   in.UserLocation.Region,
			City:     in.UserLocation.City,
			Timezone: in.UserLocation.Timezone,
		}
	}
	return out
}

func convertUsecaseCacheControlToAPI(in *usecase.CacheControl) *api.CacheControl {
	if in == nil {
		return nil
	}
	return &api.CacheControl{
		Type: in.Type,
		TTL:  in.TTL,
	}
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func convertAPIToolFormat(in *api.ToolFormat) *types.ToolFormat {
	if in == nil {
		return nil
	}
	return &types.ToolFormat{
		Type:       in.Type,
		Syntax:     in.Syntax,
		Definition: in.Definition,
	}
}
