package handlers

import (
	"encoding/json"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)
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
